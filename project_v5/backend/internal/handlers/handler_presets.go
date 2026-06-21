package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine"
	"keepstar_v5/internal/engine/presets"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/usecases"
)

// PresetHandler owns the internal preset-control endpoints:
//
//   - cache-invalidate: when a tenant publishes a preset (via the v9-canvas
//     write side / curator cockpit), the caller pokes this endpoint so V5
//     rebuilds the <tenant_design_context> block on the next Agent2 turn
//     instead of serving a stale cached system prompt.
//   - preview: deterministic zero-LLM render of a preset (published by name,
//     or a raw doc_json draft) against real tenant products, so the admin
//     can show exactly what the engine will produce before publishing.
type PresetHandler struct {
	promptCache *usecases.PromptCache
	catalog     ports.CatalogPort
	presets     ports.PresetPort
	components  ports.ComponentPort
	themes      ports.ThemePort // optional; nil → default theme attached to preview
	log         *slog.Logger
}

// NewPresetHandler constructs the handler over the shared Agent2 prompt
// cache (the one whose GetOrBuild assembles <tenant_design_context>) plus
// the read ports the preview render needs (catalog for tenant + products,
// preset/component for the published-preset path).
func NewPresetHandler(
	promptCache *usecases.PromptCache,
	catalog ports.CatalogPort,
	presetPort ports.PresetPort,
	componentPort ports.ComponentPort,
	themePort ports.ThemePort,
	log *slog.Logger,
) *PresetHandler {
	return &PresetHandler{
		promptCache: promptCache,
		catalog:     catalog,
		presets:     presetPort,
		components:  componentPort,
		themes:      themePort,
		log:         log,
	}
}

// CacheInvalidate handles POST /api/v1/internal/presets/cache-invalidate?tenant=<slug>.
// Drops the cached Agent2 system prompt for the tenant so the next turn
// re-reads the tenant's published presets. Gated by X-Internal-Key (same
// guard as the kill switch); exempt from WithTenant (middleware_tenant.go).
func (h *PresetHandler) CacheInvalidate(w http.ResponseWriter, r *http.Request) {
	if !checkInternalKey(w, r) {
		return
	}
	tenant := r.URL.Query().Get("tenant")
	if tenant == "" {
		http.Error(w, "tenant query param missing", http.StatusBadRequest)
		return
	}
	h.promptCache.Invalidate(tenant)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tenant":      tenant,
		"invalidated": true,
	})
}

// presetPreviewRequest is the body of POST /api/v1/internal/presets/preview.
// Exactly one of DocJSON / PresetName must be provided. DocJSON is a raw v5
// preset document (same shape as v5_preset_versions.doc_json) — this is how
// the admin previews DRAFTS without V5 knowing about draft status.
type presetPreviewRequest struct {
	DocJSON    json.RawMessage `json:"docJson,omitempty"`
	PresetName string          `json:"presetName,omitempty"`
	Count      int             `json:"count,omitempty"`
}

const (
	previewDefaultCount = 3
	previewMaxCount     = 12
)

// Preview handles POST /api/v1/internal/presets/preview?tenant=<slug>.
//
// Deterministic zero-LLM render: resolve tenant → load the document
// (published preset by name + published components via Materialise, OR the
// raw docJson draft as-is) → fetch up to `count` real products (first N by
// the catalog's stable created_at DESC order) → run the engine chain
// ExpandReplicates → ResolveAndInline → BindData → InjectDefaultActions —
// the same pipeline the navigation/filter and visual_assembly paths use.
//
// Response: {"document": <rendered doc map>, "productCount": <n>}.
// Errors: 400 bad body / both-or-neither doc sources / negative count,
// 404 unknown tenant or preset, 500 (logged) on infrastructure failures.
// Gated by X-Internal-Key (503 when V5_INTERNAL_KEY unset, 403 mismatch);
// exempt from WithTenant like every /api/v1/internal/ route.
func (h *PresetHandler) Preview(w http.ResponseWriter, r *http.Request) {
	if !checkInternalKey(w, r) {
		return
	}
	tenantSlug := r.URL.Query().Get("tenant")
	if tenantSlug == "" {
		http.Error(w, "tenant query param missing", http.StatusBadRequest)
		return
	}

	var req presetPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	hasDoc := len(req.DocJSON) > 0 && string(req.DocJSON) != "null"
	if hasDoc == (req.PresetName != "") {
		http.Error(w, "exactly one of docJson or presetName must be provided", http.StatusBadRequest)
		return
	}
	if req.Count < 0 {
		http.Error(w, "count must be >= 0", http.StatusBadRequest)
		return
	}
	count := req.Count
	if count == 0 {
		count = previewDefaultCount
	}
	if count > previewMaxCount {
		count = previewMaxCount
	}

	tenant, err := h.catalog.GetTenantBySlug(r.Context(), tenantSlug)
	if err != nil {
		if errors.Is(err, domain.ErrTenantNotFound) {
			http.Error(w, "tenant not found", http.StatusNotFound)
			return
		}
		h.log.Error("preview: tenant lookup failed", "tenant", tenantSlug, "err", err)
		http.Error(w, "tenant lookup failed", http.StatusInternalServerError)
		return
	}

	// Source document: published preset (+ components, via Materialise) or
	// the raw draft docJson as-is — drafts from the canvas translator carry
	// no component refs, so no Materialise merge on that path.
	var doc *engine.Document
	if req.PresetName != "" {
		preset, err := h.presets.GetPublishedPreset(r.Context(), tenant.Slug, req.PresetName)
		if err != nil {
			if errors.Is(err, domain.ErrPresetNotFound) {
				http.Error(w, "preset not found", http.StatusNotFound)
				return
			}
			h.log.Error("preview: load preset failed", "tenant", tenantSlug, "preset", req.PresetName, "err", err)
			http.Error(w, "load preset failed", http.StatusInternalServerError)
			return
		}
		presetDoc, err := unmarshalPresetDoc(preset.DocumentJSON)
		if err != nil {
			h.log.Error("preview: published preset doc unparsable", "tenant", tenantSlug, "preset", req.PresetName, "err", err)
			http.Error(w, "published preset document unparsable", http.StatusInternalServerError)
			return
		}
		componentList, err := h.components.ListPublishedComponents(r.Context(), tenant.Slug)
		if err != nil {
			h.log.Error("preview: list components failed", "tenant", tenantSlug, "err", err)
			http.Error(w, "list components failed", http.StatusInternalServerError)
			return
		}
		componentDocs := make([]*engine.Document, 0, len(componentList))
		for _, c := range componentList {
			cd, err := unmarshalPresetDoc(c.DocumentJSON)
			if err != nil {
				h.log.Debug("preview: component doc unmarshal failed; skipping", "component", c.Name, "err", err)
				continue
			}
			componentDocs = append(componentDocs, cd)
		}
		doc = engine.Materialise(presetDoc, componentDocs)
	} else {
		doc, err = unmarshalPresetDoc(req.DocJSON)
		if err != nil {
			http.Error(w, "invalid docJson: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Deterministic product fetch over a wider window, preferring
	// presentation-rich products (image/brand/rating/description). The
	// newest rows are often ingest-test junk with no media — a preview
	// full of bare cards reads as a broken preset rather than missing
	// data (owner hit exactly this on 2026-06-12).
	window := count * 8
	if window < 24 {
		window = 24
	}
	products, _, err := h.catalog.ListProducts(r.Context(), tenant.ID, ports.ProductFilter{Limit: window})
	if err != nil {
		h.log.Error("preview: list products failed", "tenant", tenantSlug, "err", err)
		http.Error(w, "list products failed", http.StatusInternalServerError)
		return
	}
	products = pickRichestSamples(products, count)
	data := make([]map[string]any, len(products))
	for i, p := range products {
		m := engine.ProductToMap(p)
		// InjectDefaultActions resolves the action entity via data["id"];
		// ProductToMap only carries it when the vendor extra has one, so
		// backfill from the catalog row (extra's id wins if present).
		if _, ok := m["id"]; !ok {
			m["id"] = p.ID
		}
		data[i] = m
	}

	// Zero-LLM render chain — mirrors materialiseGrid (handler_navigation.go).
	engine.ExpandReplicates(doc, len(data))
	engine.ResolveAndInline(doc)
	engine.BindData(doc, data)
	engine.InjectDefaultActions(doc, domain.EntityTypeProduct, data)

	templateMap, err := engineDocToHandlerMap(doc)
	if err != nil {
		h.log.Error("preview: marshal document failed", "tenant", tenantSlug, "err", err)
		http.Error(w, "marshal document failed", http.StatusInternalServerError)
		return
	}

	// Attach doc.theme so the preview render matches what chat will paint.
	// tenant resolved above; tenant.ID keys the same v5_themes lookup.
	templateMap["theme"] = resolveThemeMap(r.Context(), h.themes, tenant.ID, h.log)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"document":     templateMap,
		"productCount": len(data),
	})
}

// systemPresetEntry is one row of GET /api/v1/internal/presets/system.
// Field names are a frozen cross-repo contract with the admin canvas —
// do not rename without bumping both sides.
type systemPresetEntry struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	Category         string `json:"category"`
	DefaultReplicate bool   `json:"defaultReplicate"`
	Kind             string `json:"kind"`
}

// ListSystem handles GET /api/v1/internal/presets/system.
//
// Returns the full system preset library (the 12 registry presets plus
// the 2 reusable components) with the Agent2-visible description, the
// family category, and the default replicate flag — everything the admin
// canvas needs to render a registration picker without duplicating the
// taxonomy. Not tenant-scoped: system presets are by definition shared.
//
// Components are advertised under their public names (presets.
// SystemComponentPublicNames) with kind="component", category=
// "components", defaultReplicate=false. Sorted by name ascending.
// Gated by X-Internal-Key (503 when V5_INTERNAL_KEY unset, 403
// mismatch); exempt from WithTenant like every /api/v1/internal/ route.
func (h *PresetHandler) ListSystem(w http.ResponseWriter, r *http.Request) {
	if !checkInternalKey(w, r) {
		return
	}
	entries := make([]systemPresetEntry, 0, len(presets.SystemPresetSeeds)+len(presets.SystemComponentSeeds))
	for name := range presets.SystemPresetSeeds {
		entries = append(entries, systemPresetEntry{
			Name:             name,
			Description:      presets.SystemPresetDescriptions[name],
			Category:         presets.SystemPresetCategory[name],
			DefaultReplicate: presets.SystemPresetDefaultReplicate[name],
			Kind:             "preset",
		})
	}
	for key := range presets.SystemComponentSeeds {
		entries = append(entries, systemPresetEntry{
			Name:             presets.SystemComponentPublicNames[key],
			Description:      presets.SystemComponentDescriptions[key],
			Category:         "components",
			DefaultReplicate: false,
			Kind:             "component",
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"presets": entries,
	})
}

// pickRichestSamples keeps `count` products, preferring ones that can
// actually fill a card's atoms — an image above all, then brand, rating,
// description. Stable on ties and re-sorted back to the catalog's default
// order, so previews stay deterministic run to run.
func pickRichestSamples(products []domain.Product, count int) []domain.Product {
	if len(products) <= count {
		return products
	}
	type scored struct {
		idx   int
		score int
	}
	all := make([]scored, len(products))
	for i, p := range products {
		s := 0
		if len(p.Images) > 0 && p.Images[0] != "" {
			s += 4 // the hero image carries most of a card's look
		}
		if p.Brand != "" {
			s++
		}
		if p.Rating > 0 {
			s++
		}
		if p.Description != "" {
			s++
		}
		all[i] = scored{idx: i, score: s}
	}
	sort.SliceStable(all, func(a, b int) bool { return all[a].score > all[b].score })
	picked := all[:count]
	sort.SliceStable(picked, func(a, b int) bool { return picked[a].idx < picked[b].idx })
	out := make([]domain.Product, count)
	for i, s := range picked {
		out[i] = products[s.idx]
	}
	return out
}
