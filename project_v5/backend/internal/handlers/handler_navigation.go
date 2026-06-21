package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine"
	"keepstar_v5/internal/engine/presets"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/usecases"
)

// NavigationHandler owns POST /api/v1/navigation/expand and
// /api/v1/navigation/back. The two endpoints are state-mutating
// shortcuts that don't go through the LLM — they realise the
// adjacency graph (preset → drill-target preset, hardcoded today;
// future v5_presets.metadata) directly.
//
// expand: snapshot current view → materialise drill-target preset
// against ONE entity → write template + view zones, push snapshot.
//
// back:   pop snapshot → restore its template + view zones.
//
// Both endpoints are best-effort sync writes; failure modes surface
// as 4xx (entity not found, can't go back) or 5xx (DB write failure).
type NavigationHandler struct {
	state      ports.StatePort
	presets    ports.PresetPort
	components ports.ComponentPort
	themes     ports.ThemePort
	log        *slog.Logger
}

// NewNavigationHandler constructs the handler with its deps. themes is
// optional (nil → default theme attached) and exists so the deterministic
// drill/back/filter documents carry the same doc.theme the chat/preview
// paths attach — otherwise a custom tenant theme would be stripped by the
// widget on the first navigation (the frontend drops --kw-* on a theme-less
// document).
func NewNavigationHandler(state ports.StatePort, presetPort ports.PresetPort, componentPort ports.ComponentPort, themes ports.ThemePort, log *slog.Logger) *NavigationHandler {
	return &NavigationHandler{
		state:      state,
		presets:    presetPort,
		components: componentPort,
		themes:     themes,
		log:        log,
	}
}

type expandRequest struct {
	SessionID  string            `json:"sessionId"`
	EntityType domain.EntityType `json:"entityType"`
	EntityID   string            `json:"entityId"`
	TurnID     string            `json:"turnId,omitempty"`
}

type backRequest struct {
	SessionID string `json:"sessionId"`
	TurnID    string `json:"turnId,omitempty"`
}

type filterRequest struct {
	SessionID string                   `json:"sessionId"`
	Filters   []usecases.AppliedFilter `json:"filters"`
	SortField string                   `json:"sortField,omitempty"` // price | rating | name
	SortOrder string                   `json:"sortOrder,omitempty"` // asc | desc
	TurnID    string                   `json:"turnId,omitempty"`
}

type navResponse struct {
	Success     bool                   `json:"success"`
	Document    map[string]interface{} `json:"document,omitempty"`
	ViewMode    domain.ViewMode        `json:"viewMode,omitempty"`
	Focused     *domain.EntityRef      `json:"focused,omitempty"`
	StackSize   int                    `json:"stackSize"`
	CanGoBack   bool                   `json:"canGoBack"`
	PresetInUse string                 `json:"presetInUse,omitempty"`
	// Facets is set by Filter — the guided, data-derived filter dimensions
	// recomputed for the active filter set. Omitted by expand/back.
	Facets []usecases.Facet `json:"facets,omitempty"`
}

// Expand handles POST /api/v1/navigation/expand.
func (h *NavigationHandler) Expand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req expandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SessionID == "" || req.EntityID == "" {
		http.Error(w, "sessionId and entityId are required", http.StatusBadRequest)
		return
	}
	if req.EntityType == "" {
		req.EntityType = domain.EntityTypeProduct
	}

	tenant := TenantFromContext(r.Context())
	if tenant == nil {
		http.Error(w, "tenant unresolved", http.StatusInternalServerError)
		return
	}

	state, err := h.state.GetState(r.Context(), req.SessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Locate the entity inside the data zone.
	dataMap, ok := findEntityMap(state, req.EntityType, req.EntityID)
	if !ok {
		http.Error(w, "entity not found in current data zone", http.StatusBadRequest)
		return
	}

	// Look up adjacency: source preset → drill target. Source comes from
	// the synthetic top-level marker stamped by visual_assembly.
	sourcePreset := readPresetInUseMap(state.Current.Template)
	target := presets.AdjacentPreset(sourcePreset)
	if target == "" {
		http.Error(w, "no drill target registered for current preset", http.StatusBadRequest)
		return
	}

	// Materialise the drill-target preset against the single entity.
	doc, err := h.materialise(r.Context(), tenant.Slug, target, dataMap)
	if err != nil {
		http.Error(w, "drill render failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	templateMap, err := engineDocToHandlerMap(doc)
	if err != nil {
		http.Error(w, "marshal document: "+err.Error(), http.StatusInternalServerError)
		return
	}
	templateMap[domain.TemplatePresetInUseKey] = target

	// Push the prior view (mode + focused + step + template snapshot)
	// onto the stack so /back can restore it without re-rendering.
	snapshot := &domain.ViewSnapshot{
		Mode:        state.View.Mode,
		Focused:     state.View.Focused,
		Refs:        buildEntityRefs(state.Current.Data),
		Step:        state.Step,
		Template:    state.Current.Template,
		PresetInUse: sourcePreset,
		CreatedAt:   time.Now(),
	}
	if err := h.state.PushView(r.Context(), req.SessionID, snapshot); err != nil {
		http.Error(w, "push view: "+err.Error(), http.StatusInternalServerError)
		return
	}

	stack, _ := h.state.GetViewStack(r.Context(), req.SessionID)
	newView := domain.ViewState{
		Mode:    domain.ViewModeDetail,
		Focused: &domain.EntityRef{Type: req.EntityType, ID: req.EntityID},
	}
	viewInfo := domain.DeltaInfo{
		TurnID:    req.TurnID,
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_expand",
		DeltaType: domain.DeltaTypePush,
		Path:      "view",
	}
	if _, err := h.state.UpdateView(r.Context(), req.SessionID, newView, stack, viewInfo); err != nil {
		http.Error(w, "update view: "+err.Error(), http.StatusInternalServerError)
		return
	}

	templateInfo := domain.DeltaInfo{
		TurnID:    req.TurnID,
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_expand",
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "current.template",
	}
	if _, err := h.state.UpdateTemplate(r.Context(), req.SessionID, templateMap, templateInfo); err != nil {
		http.Error(w, "update template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	attachThemeToDocument(r.Context(), templateMap, h.themes, tenant, h.log)
	writeJSON(w, http.StatusOK, navResponse{
		Success:     true,
		Document:    templateMap,
		ViewMode:    newView.Mode,
		Focused:     newView.Focused,
		StackSize:   len(stack),
		CanGoBack:   len(stack) > 0,
		PresetInUse: target,
	})
}

// Back handles POST /api/v1/navigation/back.
func (h *NavigationHandler) Back(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req backRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	snap, err := h.state.PopView(r.Context(), req.SessionID)
	if err != nil {
		http.Error(w, "pop view: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if snap == nil {
		http.Error(w, "cannot go back: stack is empty", http.StatusBadRequest)
		return
	}

	stack, _ := h.state.GetViewStack(r.Context(), req.SessionID)
	restoredView := domain.ViewState{
		Mode:    snap.Mode,
		Focused: snap.Focused,
	}
	viewInfo := domain.DeltaInfo{
		TurnID:    req.TurnID,
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_back",
		DeltaType: domain.DeltaTypePop,
		Path:      "view",
	}
	if _, err := h.state.UpdateView(r.Context(), req.SessionID, restoredView, stack, viewInfo); err != nil {
		http.Error(w, "update view: "+err.Error(), http.StatusInternalServerError)
		return
	}

	templateMap := snap.Template
	if templateMap == nil {
		templateMap = map[string]interface{}{}
	}
	templateInfo := domain.DeltaInfo{
		TurnID:    req.TurnID,
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_back",
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "current.template",
	}
	if _, err := h.state.UpdateTemplate(r.Context(), req.SessionID, templateMap, templateInfo); err != nil {
		http.Error(w, "update template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Restored snapshots carry the pre-drill template (no theme stored in
	// state); re-attach the resolved theme so /back keeps a custom tenant
	// theme. TenantFromContext is fail-open in attachThemeToDocument (nil →
	// defaults), so an unresolved tenant degrades to the static look.
	attachThemeToDocument(r.Context(), templateMap, h.themes, TenantFromContext(r.Context()), h.log)
	writeJSON(w, http.StatusOK, navResponse{
		Success:     true,
		Document:    templateMap,
		ViewMode:    restoredView.Mode,
		Focused:     restoredView.Focused,
		StackSize:   len(stack),
		CanGoBack:   len(stack) > 0,
		PresetInUse: snap.PresetInUse,
	})
}

// Filter handles POST /api/v1/navigation/filter — a deterministic
// (no-LLM) re-render of the CURRENT grid preset against the products in
// the data zone, narrowed by brand. Empty brand resets to the full set.
// current_data is NOT mutated (the full set is kept so a reset re-renders
// everything); only current.template is updated. This is the cheap
// interaction path: a brand chip click re-filters with zero LLM cost.
func (h *NavigationHandler) Filter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req filterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	tenant := TenantFromContext(r.Context())
	if tenant == nil {
		http.Error(w, "tenant unresolved", http.StatusInternalServerError)
		return
	}

	state, err := h.state.GetState(r.Context(), req.SessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// The grid can only be re-rendered deterministically when a preset is
	// stamped on the current template (freestyle / detail views have none).
	sourcePreset := readPresetInUseMap(state.Current.Template)
	if sourcePreset == "" {
		http.Error(w, "no grid preset to filter on the current view", http.StatusBadRequest)
		return
	}

	// Apply the active filter set over the FULL current data set (empty set
	// = reset). current_data is left untouched so a reset shows everything
	// again and stacked filters always recompute from the same base.
	filtered := usecases.FilterProducts(state.Current.Data.Products, req.Filters)

	// Optional deterministic sort over the filtered slice (in-memory, no LLM).
	if req.SortField != "" {
		filtered = sortProducts(filtered, req.SortField, req.SortOrder)
	}

	doc, err := h.materialiseGrid(r.Context(), tenant.Slug, sourcePreset, filtered)
	if err != nil {
		http.Error(w, "filter render failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	templateMap, err := engineDocToHandlerMap(doc)
	if err != nil {
		http.Error(w, "marshal document: "+err.Error(), http.StatusInternalServerError)
		return
	}
	templateMap[domain.TemplatePresetInUseKey] = sourcePreset

	templateInfo := domain.DeltaInfo{
		TurnID:    req.TurnID,
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_filter",
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "current.template",
	}
	if _, err := h.state.UpdateTemplate(r.Context(), req.SessionID, templateMap, templateInfo); err != nil {
		http.Error(w, "update template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	stack, _ := h.state.GetViewStack(r.Context(), req.SessionID)
	// Guided facets — recomputed over the base set with the active filters
	// so the panel reflects what's still available after filtering.
	facets := usecases.BuildGuidedFacets(state.Current.Data.Products, req.Filters)
	attachThemeToDocument(r.Context(), templateMap, h.themes, tenant, h.log)
	writeJSON(w, http.StatusOK, navResponse{
		Success:     true,
		Document:    templateMap,
		ViewMode:    state.View.Mode,
		Focused:     state.View.Focused,
		StackSize:   len(stack),
		CanGoBack:   len(stack) > 0,
		PresetInUse: sourcePreset,
		Facets:      facets,
	})
}

// materialise loads target preset + components and runs the V5 engine
// pipeline (Materialise → ResolveAndInline → BindData → InjectDefaultActions)
// against ONE entity. No replicate fan-out — detail views render a
// single instance.
func (h *NavigationHandler) materialise(ctx context.Context, tenantSlug string, target string, entity map[string]interface{}) (*engine.Document, error) {
	preset, err := h.presets.GetPublishedPreset(ctx, tenantSlug, target)
	if err != nil {
		return nil, fmt.Errorf("load preset %q: %w", target, err)
	}
	presetDoc, err := unmarshalPresetDoc(preset.DocumentJSON)
	if err != nil {
		return nil, fmt.Errorf("parse preset doc: %w", err)
	}

	componentList, err := h.components.ListPublishedComponents(ctx, tenantSlug)
	if err != nil {
		return nil, fmt.Errorf("list components: %w", err)
	}
	componentDocs := make([]*engine.Document, 0, len(componentList))
	for _, c := range componentList {
		cd, err := unmarshalPresetDoc(c.DocumentJSON)
		if err != nil {
			h.log.Debug("nav: component doc unmarshal failed; skipping", "component", c.Name, "err", err)
			continue
		}
		componentDocs = append(componentDocs, cd)
	}
	merged := engine.Materialise(presetDoc, componentDocs)
	engine.ResolveAndInline(merged)
	engine.BindData(merged, []map[string]any{entity})
	engine.InjectDefaultActions(merged, domain.EntityTypeProduct, []map[string]any{entity})
	return merged, nil
}

// materialiseGrid renders a GRID preset against a LIST of products
// (replicate fan-out), mirroring the rebuild+preset path of the
// visual_assembly tool but without the LLM. Used by Filter to re-render
// the current grid against a filtered product slice deterministically.
// Note: ops the LLM may have layered on the original grid are not
// replayed — this renders the clean preset against the filtered data.
func (h *NavigationHandler) materialiseGrid(ctx context.Context, tenantSlug, presetName string, products []domain.Product) (*engine.Document, error) {
	preset, err := h.presets.GetPublishedPreset(ctx, tenantSlug, presetName)
	if err != nil {
		return nil, fmt.Errorf("load preset %q: %w", presetName, err)
	}
	presetDoc, err := unmarshalPresetDoc(preset.DocumentJSON)
	if err != nil {
		return nil, fmt.Errorf("parse preset doc: %w", err)
	}

	componentList, err := h.components.ListPublishedComponents(ctx, tenantSlug)
	if err != nil {
		return nil, fmt.Errorf("list components: %w", err)
	}
	componentDocs := make([]*engine.Document, 0, len(componentList))
	for _, c := range componentList {
		cd, err := unmarshalPresetDoc(c.DocumentJSON)
		if err != nil {
			h.log.Debug("filter: component doc unmarshal failed; skipping", "component", c.Name, "err", err)
			continue
		}
		componentDocs = append(componentDocs, cd)
	}

	data := make([]map[string]any, len(products))
	for i, p := range products {
		data[i] = engine.ProductToMap(p)
	}

	merged := engine.Materialise(presetDoc, componentDocs)
	engine.ExpandReplicates(merged, len(products))
	engine.ResolveAndInline(merged)
	engine.BindData(merged, data)
	engine.InjectDefaultActions(merged, domain.EntityTypeProduct, data)
	return merged, nil
}

// sortProducts returns a sorted COPY of products (never mutates the input,
// which may alias state.Current.Data). field: price|rating|name; order desc
// reverses. Unknown field → unchanged copy. Deterministic, no LLM.
func sortProducts(products []domain.Product, field, order string) []domain.Product {
	out := make([]domain.Product, len(products))
	copy(out, products)
	desc := order == "desc"
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch field {
		case "price":
			if desc {
				return a.Price > b.Price
			}
			return a.Price < b.Price
		case "rating":
			if desc {
				return a.Rating > b.Rating
			}
			return a.Rating < b.Rating
		case "name":
			if desc {
				return a.Name > b.Name
			}
			return a.Name < b.Name
		}
		return false
	})
	return out
}

// unmarshalPresetDoc parses raw doc bytes into engine.Document.
func unmarshalPresetDoc(raw json.RawMessage) (*engine.Document, error) {
	var doc engine.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// engineDocToHandlerMap round-trips engine.Document → JSON →
// map[string]interface{}.
func engineDocToHandlerMap(doc *engine.Document) (map[string]interface{}, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// findEntityMap returns the data-map shape (engine.ProductToMap) of
// the named entity from the current data zone, or false if not found.
func findEntityMap(state *domain.SessionState, et domain.EntityType, id string) (map[string]any, bool) {
	switch et {
	case domain.EntityTypeProduct:
		for _, p := range state.Current.Data.Products {
			if p.ID == id {
				return engine.ProductToMap(p), true
			}
		}
	}
	return nil, false
}

// buildEntityRefs flattens the data zone into EntityRef list (for
// snapshots).
func buildEntityRefs(data domain.StateData) []domain.EntityRef {
	out := make([]domain.EntityRef, 0, len(data.Products)+len(data.Services))
	for _, p := range data.Products {
		out = append(out, domain.EntityRef{Type: domain.EntityTypeProduct, ID: p.ID})
	}
	for _, s := range data.Services {
		out = append(out, domain.EntityRef{Type: domain.EntityTypeService, ID: s.ID})
	}
	return out
}

// readPresetInUseMap reads the stamped __presetInUse marker from a
// template map. Empty when freestyle / modify (no preset to drill into).
func readPresetInUseMap(tpl map[string]interface{}) string {
	if tpl == nil {
		return ""
	}
	v, _ := tpl[domain.TemplatePresetInUseKey].(string)
	return v
}
