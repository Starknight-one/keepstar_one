// Package usecases — harvester-lite (curator-driven pivot 2026-04-27, Этап 2.2).
//
// Replaces legacy `runInitialSync` from the deleted ShopifyUseCase. Reads
// staged Shopify product payloads from `catalog.shopify_raw_imports` and
// writes to `catalog.products` ONLY — no master_products / master_variants /
// candidates. The merge into master is curator-driven (Этап 5).
//
// Tier-1 deterministic mapping: title→original_name, descriptionHtml→raw_attributes.description,
// vendor/product_type/tags→raw_attributes, featuredImage+images→media, variants[]
// (sku/barcode/price/inventory/weight/options)→raw_attributes.variants. Aggregate
// price (min across variants) and stock (sum) into the listing-level columns.
//
// Two entry points implementing usecases.HarvesterLite:
//   - RunForTenant(tenantID) — bulk: streams shopify_raw_imports, upserts each.
//   - UpsertOne(tenantID, body) — webhook: parses one Shopify REST product.
//   - SoftDeleteOne(tenantID, sourceID) — webhook: marks listing deleted.
package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// HarvesterLiteUseCase implements usecases.HarvesterLite.
//
// candidates and categories are optional dependencies. When present (production
// wiring, see cmd/server/main.go), every imported product feeds the candidates
// staging tables (master_attribute_candidates, master_category_candidates) and
// upserts tenant_categories rows from the product's collection links. When nil
// (e.g. tests, small CLI tools that only need basic listing upsert), those side
// effects are silently skipped.
type HarvesterLiteUseCase struct {
	staging    ports.ShopifyStagingPort
	catalog    ports.AdminCatalogPort
	candidates ports.CandidatesPort
	categories ports.CategoriesPort
	log        *logger.Logger
}

func NewHarvesterLite(
	staging ports.ShopifyStagingPort,
	catalog ports.AdminCatalogPort,
	log *logger.Logger,
) *HarvesterLiteUseCase {
	return &HarvesterLiteUseCase{staging: staging, catalog: catalog, log: log}
}

// SetSignals wires the optional candidates + categories dependencies. Called
// from main.go after construction so the existing two-arg NewHarvesterLite
// signature stays backwards-compatible with tests / CLI tools that don't care.
func (uc *HarvesterLiteUseCase) SetSignals(candidates ports.CandidatesPort, categories ports.CategoriesPort) {
	uc.candidates = candidates
	uc.categories = categories
}

// RunForTenant streams every staged product row and upserts a listing per row.
// Returns the count of products written. Errors on individual products are
// logged but don't abort the run (single bad payload shouldn't block a 1000-product
// catalog import). The caller is responsible for tenant authorization.
func (uc *HarvesterLiteUseCase) RunForTenant(ctx context.Context, tenantID string) (int, error) {
	start := time.Now()
	count := 0
	failures := 0

	signals := newSignalCounters()
	err := uc.staging.IterateProducts(ctx, tenantID, func(sourceID string, payload json.RawMessage, fetchedAt time.Time) error {
		view, err := parseBulkProduct(payload)
		if err != nil {
			uc.log.Warn("harvester_lite_parse_failed", "tenant_id", tenantID, "source_id", sourceID, "error", err.Error())
			failures++
			return nil // continue iteration
		}
		listing := view.toListing(tenantID)
		if _, err := uc.catalog.UpsertListingFromSource(ctx, listing); err != nil {
			uc.log.Warn("harvester_lite_upsert_failed", "tenant_id", tenantID, "source_id", sourceID, "error", err.Error())
			failures++
			return nil
		}
		uc.recordSignals(ctx, tenantID, view, signals)
		count++
		return nil
	})
	if err != nil {
		return count, fmt.Errorf("iterate staging: %w", err)
	}
	uc.log.Info("harvester_lite_run_completed",
		"tenant_id", tenantID,
		"products_written", count,
		"failures", failures,
		"attribute_candidates_upserted", signals.attrUpserts,
		"category_candidates_upserted", signals.catUpserts,
		"tenant_categories_upserted", signals.tenantCats,
		"duration_ms", time.Since(start).Milliseconds())
	return count, nil
}

// UpsertOne handles one Shopify webhook payload (products/create or products/update).
// Webhook payload shape differs from bulk JSONL: snake_case keys, body_html instead
// of descriptionHtml, tags as comma-separated string, numeric ids instead of GIDs.
func (uc *HarvesterLiteUseCase) UpsertOne(ctx context.Context, tenantID string, body []byte) error {
	view, err := parseWebhookProduct(body)
	if err != nil {
		return fmt.Errorf("parse webhook product: %w", err)
	}
	listing := view.toListing(tenantID)
	if _, err := uc.catalog.UpsertListingFromSource(ctx, listing); err != nil {
		return fmt.Errorf("upsert: %w", err)
	}
	uc.recordSignals(ctx, tenantID, view, newSignalCounters())
	return nil
}

// --- Signals (candidates + tenant_categories) wiring ---

// signalCounters is the per-run aggregate the harvester logs at the end so
// operators can see how much candidate signal one import generated.
type signalCounters struct {
	attrUpserts int
	catUpserts  int
	tenantCats  int
}

func newSignalCounters() *signalCounters { return &signalCounters{} }

// tier1Reserved is the set of metafield-style keys that the Tier-1 deterministic
// mapping already handles. We don't want them re-staged as attribute candidates.
var tier1Reserved = map[string]struct{}{
	"vendor": {}, "product_type": {}, "tags": {}, "handle": {},
	"title": {}, "name": {}, "description": {}, "body_html": {},
	"sku": {}, "barcode": {}, "price": {}, "compare_at_price": {},
	"images": {}, "image": {}, "id": {},
}

// recordSignals stamps candidate / tenant_category rows from one parsed product
// view. Best-effort — failures log a warning but don't abort the parent run
// (signal collection is bookkeeping, not primary data flow).
//
// What it does:
//
//   1. Classifies the product's vertical from productType/vendor/tags.
//   2. For every metafield not in Tier-1 reserved keys, upserts an
//      attribute candidate keyed by (key, vertical).
//   3. For every collection, upserts a category candidate (master-side
//      staging) AND a tenant_category row (per-tenant copy) with kind
//      classified by ClassifyKind. The merge agent later maps tenant_category
//      to master_category.
func (uc *HarvesterLiteUseCase) recordSignals(ctx context.Context, tenantID string, view *shopifyListingView, counters *signalCounters) {
	if view == nil {
		return
	}
	vertical := ClassifyVertical(view.productType, view.vendor, view.tags)

	// Attribute candidates — every metafield key, deduped by (key, vertical)
	// at the adapter level via ON CONFLICT.
	if uc.candidates != nil {
		for _, mf := range view.metafields {
			key, _ := mf["key"].(string)
			if key == "" {
				continue
			}
			if _, reserved := tier1Reserved[strings.ToLower(key)]; reserved {
				continue
			}
			value, _ := mf["value"].(string)
			ns, _ := mf["namespace"].(string)
			agentMeta := ""
			if ns != "" {
				agentMeta = "namespace=" + ns
			}
			if err := uc.candidates.UpsertAttributeCandidate(ctx, key, vertical, value, agentMeta); err != nil {
				uc.log.Warn("harvester_lite_attr_candidate_failed",
					"tenant_id", tenantID, "key", key, "error", err.Error())
				continue
			}
			counters.attrUpserts++
		}

		// Category candidates — every collection title, deduped by (name, vertical).
		for _, c := range view.collections {
			title, _ := c["title"].(string)
			if title == "" {
				continue
			}
			if err := uc.candidates.UpsertCategoryCandidate(ctx, title, "", vertical); err != nil {
				uc.log.Warn("harvester_lite_cat_candidate_failed",
					"tenant_id", tenantID, "name", title, "error", err.Error())
				continue
			}
			counters.catUpserts++
		}
	}

	// Tenant categories — one row per (tenant, collection). Reuses categories
	// adapter's UNIQUE(tenant_id, external_id) idempotency. Kind comes from
	// the deterministic classifier (Phase A5).
	if uc.categories != nil {
		for _, c := range view.collections {
			title, _ := c["title"].(string)
			handle, _ := c["handle"].(string)
			extID, _ := c["id"].(string)
			if title == "" {
				continue
			}
			if extID != "" {
				// Adapter expects full GID for cross-tenant uniqueness. Re-add
				// prefix if classifyChild stripped it (we keep numeric id in
				// raw_attributes for human readability).
				if !strings.HasPrefix(extID, "gid://") {
					extID = "gid://shopify/Collection/" + extID
				}
			}
			tc := &domain.TenantCategory{
				TenantID:   tenantID,
				ExternalID: extID,
				Slug:       slugifyOrFallback(handle, title),
				Name:       title,
				Kind:       ClassifyKind(title),
			}
			if _, err := uc.categories.UpsertTenantCategory(ctx, tc); err != nil {
				uc.log.Warn("harvester_lite_tenant_cat_failed",
					"tenant_id", tenantID, "name", title, "error", err.Error())
				continue
			}
			counters.tenantCats++
		}
	}
}

// slugifyOrFallback prefers Shopify's handle (already lowercase + hyphenated);
// when missing falls back to a defensive slug derived from the title. We don't
// over-engineer this — collisions inside one tenant are acceptable since the
// adapter dedupes by (tenant_id, external_id) which always exists for Shopify.
func slugifyOrFallback(handle, title string) string {
	if handle != "" {
		return handle
	}
	s := strings.ToLower(title)
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r == ' ', r == '-', r == '_':
			return '-'
		default:
			return -1
		}
	}, s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// SoftDeleteOne marks a listing deleted by source id (products/delete webhook).
func (uc *HarvesterLiteUseCase) SoftDeleteOne(ctx context.Context, tenantID, sourceID string) error {
	return uc.catalog.SoftDeleteProductBySource(ctx, tenantID, "shopify", sourceID)
}

// --- Intermediate view (parser output, upserter input) ---

// shopifyListingView is the canonical, parser-agnostic shape we feed into the
// upsert path. Both parseBulkProduct (JSONL merged from M4a) and parseWebhookProduct
// (REST webhook body) produce this shape.
type shopifyListingView struct {
	sourceID    string
	title       string
	descHTML    string
	vendor      string
	productType string
	tags        []string
	handle      string
	media       []map[string]interface{} // [{url, alt}]
	variants    []map[string]interface{} // canonical variant shape (see canonicalizeVariant)
	metafields  []map[string]interface{} // [{namespace, key, type, value}]
	collections []map[string]interface{} // [{id, handle, title}]
	currency    string                   // dev-store and most shops use shop default; webhook payload doesn't carry it directly
	rawPayload  []byte                   // for hash-diff
}

// toListing maps the view into the upsert payload. Aggregates price (minimum
// across variants in cents) and stock (sum across variants).
func (v *shopifyListingView) toListing(tenantID string) *domain.ListingFromSource {
	priceCents, stockTotal := aggregateVariants(v.variants)
	rawAttrs := map[string]interface{}{}
	if v.vendor != "" {
		rawAttrs["vendor"] = v.vendor
	}
	if v.productType != "" {
		rawAttrs["product_type"] = v.productType
	}
	if len(v.tags) > 0 {
		rawAttrs["tags"] = v.tags
	}
	if v.handle != "" {
		rawAttrs["handle"] = v.handle
	}
	if len(v.variants) > 0 {
		rawAttrs["variants"] = v.variants
	}
	if len(v.metafields) > 0 {
		rawAttrs["metafields"] = v.metafields
	}
	if len(v.collections) > 0 {
		rawAttrs["collections"] = v.collections
	}
	// Use first variant's SKU for top-level convenience (curator UI search hits this).
	if len(v.variants) > 0 {
		if sku, _ := v.variants[0]["sku"].(string); sku != "" {
			rawAttrs["sku"] = sku
		}
	}

	flatImages := make([]string, 0, len(v.media))
	for _, m := range v.media {
		if u, _ := m["url"].(string); u != "" {
			flatImages = append(flatImages, u)
		}
	}

	hash := ""
	if len(v.rawPayload) > 0 {
		sum := sha256.Sum256(v.rawPayload)
		hash = hex.EncodeToString(sum[:])
	}

	return &domain.ListingFromSource{
		TenantID:      tenantID,
		SourceSystem:  "shopify",
		SourceID:      v.sourceID,
		Name:          v.title,
		OriginalName:  v.title,
		Description:   stripHTML(v.descHTML),
		PriceCents:    priceCents,
		Currency:      v.currency,
		StockQuantity: stockTotal,
		Images:        flatImages,
		Media:         v.media,
		RawAttributes: rawAttrs,
		PayloadHash:   hash,
	}
}

// --- Bulk JSONL parser ---

// parseBulkProduct reads a row written by ShopifyV2UseCase.streamJSONLToStaging
// (curator/backend M4a). Top-level keys come from the bulk GraphQL query;
// children (variants/images/metafields/collections) live under _v2_* keys.
func parseBulkProduct(payload json.RawMessage) (*shopifyListingView, error) {
	var p struct {
		ID              string          `json:"id"`
		Title           string          `json:"title"`
		Handle          string          `json:"handle"`
		DescriptionHTML string          `json:"descriptionHtml"`
		ProductType     string          `json:"productType"`
		Vendor          string          `json:"vendor"`
		Tags            json.RawMessage `json:"tags"` // can be []string OR string
		FeaturedImage   *struct {
			URL    string `json:"url"`
			AltTxt string `json:"altText"`
		} `json:"featuredImage"`
		V2Variants    []json.RawMessage `json:"_v2_variants"`
		V2Images      []json.RawMessage `json:"_v2_images"`
		V2Metafields  []json.RawMessage `json:"_v2_metafields"`
		V2Collections []json.RawMessage `json:"_v2_collections"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("unmarshal bulk product: %w", err)
	}

	view := &shopifyListingView{
		sourceID:    extractNumericID(p.ID),
		title:       p.Title,
		handle:      p.Handle,
		descHTML:    p.DescriptionHTML,
		vendor:      p.Vendor,
		productType: p.ProductType,
		tags:        parseTagsField(p.Tags),
		rawPayload:  payload,
	}

	if p.FeaturedImage != nil && p.FeaturedImage.URL != "" {
		view.media = append(view.media, map[string]interface{}{
			"url": p.FeaturedImage.URL, "alt": p.FeaturedImage.AltTxt,
		})
	}
	for _, raw := range p.V2Images {
		var img struct {
			URL    string `json:"url"`
			AltTxt string `json:"altText"`
		}
		if json.Unmarshal(raw, &img) == nil && img.URL != "" {
			// Skip if already in featuredImage.
			if len(view.media) > 0 && view.media[0]["url"] == img.URL {
				continue
			}
			view.media = append(view.media, map[string]interface{}{
				"url": img.URL, "alt": img.AltTxt,
			})
		}
	}

	for _, raw := range p.V2Variants {
		view.variants = append(view.variants, canonicalizeBulkVariant(raw))
	}

	for _, raw := range p.V2Metafields {
		var mf struct {
			Namespace string `json:"namespace"`
			Key       string `json:"key"`
			Type      string `json:"type"`
			Value     string `json:"value"`
		}
		if json.Unmarshal(raw, &mf) == nil && mf.Key != "" {
			view.metafields = append(view.metafields, map[string]interface{}{
				"namespace": mf.Namespace, "key": mf.Key, "type": mf.Type, "value": mf.Value,
			})
		}
	}

	for _, raw := range p.V2Collections {
		var c struct {
			ID     string `json:"id"`
			Handle string `json:"handle"`
			Title  string `json:"title"`
		}
		if json.Unmarshal(raw, &c) == nil && c.ID != "" {
			view.collections = append(view.collections, map[string]interface{}{
				"id": extractNumericID(c.ID), "handle": c.Handle, "title": c.Title,
			})
		}
	}

	return view, nil
}

// canonicalizeBulkVariant flattens a bulk-variant JSON line into the canonical
// shape we store under raw_attributes.variants[]. Matches what the merge agent
// will read in Этап 5.
func canonicalizeBulkVariant(raw json.RawMessage) map[string]interface{} {
	var bv struct {
		ID              string `json:"id"`
		SKU             string `json:"sku"`
		Title           string `json:"title"`
		Price           string `json:"price"`           // string per Shopify GraphQL Money scalar
		CompareAtPrice  string `json:"compareAtPrice"`
		Barcode         string `json:"barcode"`
		InventoryQty    int    `json:"inventoryQuantity"`
		SelectedOptions []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"selectedOptions"`
		Image *struct {
			URL string `json:"url"`
		} `json:"image"`
		InventoryItem *struct {
			Measurement *struct {
				Weight *struct {
					Value float64 `json:"value"`
					Unit  string  `json:"unit"`
				} `json:"weight"`
			} `json:"measurement"`
		} `json:"inventoryItem"`
	}
	_ = json.Unmarshal(raw, &bv)
	out := map[string]interface{}{
		"id":             extractNumericID(bv.ID),
		"sku":            bv.SKU,
		"title":          bv.Title,
		"price_cents":    parsePriceCents(bv.Price),
		"compare_cents":  parsePriceCents(bv.CompareAtPrice),
		"barcode":        bv.Barcode,
		"inventory_qty":  bv.InventoryQty,
	}
	if len(bv.SelectedOptions) > 0 {
		opts := make(map[string]string, len(bv.SelectedOptions))
		for _, o := range bv.SelectedOptions {
			opts[strings.ToLower(o.Name)] = o.Value
		}
		out["options"] = opts
	}
	if bv.Image != nil && bv.Image.URL != "" {
		out["image"] = bv.Image.URL
	}
	if bv.InventoryItem != nil && bv.InventoryItem.Measurement != nil && bv.InventoryItem.Measurement.Weight != nil {
		out["weight_value"] = bv.InventoryItem.Measurement.Weight.Value
		out["weight_unit"] = bv.InventoryItem.Measurement.Weight.Unit
	}
	return out
}

// --- Webhook (REST) parser ---

// parseWebhookProduct handles the Shopify REST webhook body for products/create
// and products/update. Different shape from bulk JSONL: snake_case, body_html
// instead of descriptionHtml, tags as a CSV string.
func parseWebhookProduct(body []byte) (*shopifyListingView, error) {
	var p struct {
		ID          int64  `json:"id"`
		Title       string `json:"title"`
		Handle      string `json:"handle"`
		BodyHTML    string `json:"body_html"`
		Vendor      string `json:"vendor"`
		ProductType string `json:"product_type"`
		Tags        string `json:"tags"` // comma-separated string
		Images      []struct {
			Src string `json:"src"`
			Alt string `json:"alt"`
		} `json:"images"`
		Image *struct {
			Src string `json:"src"`
			Alt string `json:"alt"`
		} `json:"image"`
		Variants []struct {
			ID              int64  `json:"id"`
			SKU             string `json:"sku"`
			Title           string `json:"title"`
			Price           string `json:"price"`
			CompareAtPrice  string `json:"compare_at_price"`
			Barcode         string `json:"barcode"`
			InventoryQty    int    `json:"inventory_quantity"`
			Grams           int    `json:"grams"`
			Option1         string `json:"option1"`
			Option2         string `json:"option2"`
			Option3         string `json:"option3"`
		} `json:"variants"`
		Options []struct {
			Name string `json:"name"`
		} `json:"options"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("unmarshal webhook product: %w", err)
	}
	if p.ID == 0 {
		return nil, fmt.Errorf("webhook payload missing product id")
	}

	view := &shopifyListingView{
		sourceID:    strconv.FormatInt(p.ID, 10),
		title:       p.Title,
		handle:      p.Handle,
		descHTML:    p.BodyHTML,
		vendor:      p.Vendor,
		productType: p.ProductType,
		tags:        parseCSVTags(p.Tags),
		rawPayload:  body,
	}

	// Image preference: featured first, then images[].
	if p.Image != nil && p.Image.Src != "" {
		view.media = append(view.media, map[string]interface{}{
			"url": p.Image.Src, "alt": p.Image.Alt,
		})
	}
	for _, im := range p.Images {
		if im.Src == "" {
			continue
		}
		if len(view.media) > 0 && view.media[0]["url"] == im.Src {
			continue
		}
		view.media = append(view.media, map[string]interface{}{"url": im.Src, "alt": im.Alt})
	}

	optNames := make([]string, len(p.Options))
	for i, o := range p.Options {
		optNames[i] = strings.ToLower(o.Name)
	}
	for _, v := range p.Variants {
		variant := map[string]interface{}{
			"id":            strconv.FormatInt(v.ID, 10),
			"sku":           v.SKU,
			"title":         v.Title,
			"price_cents":   parsePriceCents(v.Price),
			"compare_cents": parsePriceCents(v.CompareAtPrice),
			"barcode":       v.Barcode,
			"inventory_qty": v.InventoryQty,
		}
		if v.Grams > 0 {
			variant["weight_value"] = float64(v.Grams)
			variant["weight_unit"] = "g"
		}
		opts := map[string]string{}
		raws := []string{v.Option1, v.Option2, v.Option3}
		for i, raw := range raws {
			if raw == "" || i >= len(optNames) || optNames[i] == "" {
				continue
			}
			opts[optNames[i]] = raw
		}
		if len(opts) > 0 {
			variant["options"] = opts
		}
		view.variants = append(view.variants, variant)
	}

	// Webhook payload doesn't carry metafields/collections by default.
	return view, nil
}

// --- helpers ---

// gidPattern captures the trailing numeric segment of a Shopify GID
// like "gid://shopify/Product/12345" → "12345". Falls through to input
// if it's already numeric.
var gidPattern = regexp.MustCompile(`/(\d+)$`)

func extractNumericID(gid string) string {
	if gid == "" {
		return ""
	}
	if m := gidPattern.FindStringSubmatch(gid); len(m) == 2 {
		return m[1]
	}
	return gid
}

// parsePriceCents converts a Shopify price string like "12.34" to integer cents.
// Returns 0 for empty / unparseable.
func parsePriceCents(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if !strings.Contains(s, ".") {
		// "1234" → 123400 (treat as whole units)
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0
		}
		return n * 100
	}
	parts := strings.SplitN(s, ".", 2)
	whole, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	frac := parts[1]
	if len(frac) > 2 {
		frac = frac[:2]
	} else if len(frac) == 1 {
		frac = frac + "0"
	} else if frac == "" {
		frac = "00"
	}
	cents, err := strconv.Atoi(frac)
	if err != nil {
		return 0
	}
	if whole < 0 {
		return whole*100 - cents
	}
	return whole*100 + cents
}

// parseTagsField handles both `["a","b"]` (Shopify GraphQL) and `"a, b"` (CSV).
func parseTagsField(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	// Try array first.
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	// Fall back to string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return parseCSVTags(s)
	}
	return nil
}

func parseCSVTags(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// aggregateVariants returns (minPriceCents, totalStock).
func aggregateVariants(variants []map[string]interface{}) (int, int) {
	var minPrice int = -1
	stock := 0
	for _, v := range variants {
		if p, ok := v["price_cents"].(int); ok && p > 0 {
			if minPrice == -1 || p < minPrice {
				minPrice = p
			}
		}
		if q, ok := v["inventory_qty"].(int); ok && q > 0 {
			stock += q
		}
	}
	if minPrice == -1 {
		minPrice = 0
	}
	return minPrice, stock
}

// stripHTML — naive tag stripper; good enough for descriptions destined for
// listing.description. Not a security measure (we never render this back as HTML).
var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)
var whitespacePattern = regexp.MustCompile(`\s+`)

func stripHTML(html string) string {
	if html == "" {
		return ""
	}
	noTags := htmlTagPattern.ReplaceAllString(html, " ")
	collapsed := whitespacePattern.ReplaceAllString(noTags, " ")
	return strings.TrimSpace(collapsed)
}
