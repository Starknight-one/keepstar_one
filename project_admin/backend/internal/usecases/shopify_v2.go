// Package usecases — Shopify metadata-first import (M4 spec §4.1).
//
// This file is the entry point for the new pipeline that supersedes the
// legacy REST-based ShopifyUseCase. The flow lives across multiple files in
// this package, sequenced in commits 4a/b/c/d:
//
//   4a (THIS FILE)        — DumpToStaging: bulk pull → catalog.shopify_raw_imports
//   4b (metadata_harvest, auto_map_tier1, match_cascade, junk_detector)
//                          — pure-Go analysis + match logic, no LLM
//   4c (discovery_agent, validate_artifact)
//                          — Sonnet 4.6 multi-turn agent + validation pass
//   4d (shopify_harvester, embedding_job, webhook re-wire)
//                          — applies artifact, writes master/listing, hash-diff
//
// The legacy ShopifyUseCase (shopify.go + shopify_mapper.go) stays in place
// during 4a-c. Cut-over happens in 4d (DI swap in main.go + delete legacy).
package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"keepstar-admin/internal/adapters/shopify"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// bulkPollInterval is how often we ask Shopify for current bulk-op status.
// Shopify recommends 1-5s; faster is wasted work, slower delays UI updates.
const bulkPollInterval = 3 * time.Second

// bulkOpMaxWait is the hard ceiling on a single bulk operation. 100k products
// usually finish in 10-20 minutes; anything past 90 means something's wrong on
// Shopify's side (rare) or we sent a query they can't bulk-process (caller bug).
const bulkOpMaxWait = 90 * time.Minute

// ShopifyV2UseCase coordinates the new metadata-first import. In 4a it only
// runs DumpToStaging — the deterministic harvest, discovery agent, and final
// harvester land in subsequent commits and will hang off this same struct.
type ShopifyV2UseCase struct {
	client       *shopify.Client
	integrations ports.IntegrationsPort
	staging      ports.ShopifyStagingPort
	log          *logger.Logger
}

func NewShopifyV2UseCase(
	client *shopify.Client,
	integrations ports.IntegrationsPort,
	staging ports.ShopifyStagingPort,
	log *logger.Logger,
) *ShopifyV2UseCase {
	return &ShopifyV2UseCase{
		client:       client,
		integrations: integrations,
		staging:      staging,
		log:          log,
	}
}

// DumpToStagingResult carries the outcome of a single bulk-pull. ProductCount
// is the most useful single number for the progress UI; CollectionCount /
// MetafieldDefCount / Vendors are smaller numbers shown alongside.
type DumpToStagingResult struct {
	OperationID      string
	ProductCount     int
	CollectionCount  int
	MetafieldDefs    int
	NavigationItems  int
	VendorCount      int
	ProductTypeCount int
	TagCount         int
	Duration         time.Duration
}

// DumpToStaging runs one full metadata-first pull for a tenant. It is
// idempotent: ON CONFLICT in the staging table replaces existing rows so
// re-running just refreshes the buffer. The caller is responsible for
// authorization (tenant must own the integration).
//
// Sequence:
//   1. Pull metafield definitions (3 owner types × small list each)
//   2. Pull main navigation menu (one snapshot)
//   3. Pull shop reference data (vendors, product types, tags)
//   4. Fire bulkOperationRunQuery for the full products graph
//   5. Poll until COMPLETED / FAILED / CANCELED
//   6. Stream JSONL → split into per-product rows → upsert into staging
//
// Steps 1-3 finish in seconds. Step 4-5 dominate wallclock for big catalogs.
// Step 6 is bound by Postgres write throughput (~1k products/sec on Neon).
func (uc *ShopifyV2UseCase) DumpToStaging(ctx context.Context, tenantID, integrationID string) (*DumpToStagingResult, error) {
	start := time.Now()
	integration, err := uc.integrations.Get(ctx, tenantID, integrationID)
	if err != nil {
		return nil, fmt.Errorf("lookup integration: %w", err)
	}
	shop := integration.ExternalID
	token := integration.Credentials
	if shop == "" || token == "" {
		return nil, fmt.Errorf("integration not connected (missing shop or token)")
	}

	res := &DumpToStagingResult{}

	// --- Phase 1: metadata snapshots (~5 sec) -----------------------------------
	if err := uc.dumpMetafieldDefs(ctx, tenantID, shop, token, res); err != nil {
		return nil, fmt.Errorf("metafield defs: %w", err)
	}
	if err := uc.dumpNavigation(ctx, tenantID, shop, token, res); err != nil {
		return nil, fmt.Errorf("navigation: %w", err)
	}
	if err := uc.dumpShopReferences(ctx, tenantID, shop, token, res); err != nil {
		return nil, fmt.Errorf("shop references: %w", err)
	}

	// --- Phase 2: bulk products pull (5-40 min) ---------------------------------
	op, err := uc.runBulkProductsAndWait(ctx, shop, token)
	if err != nil {
		return nil, fmt.Errorf("bulk products: %w", err)
	}
	res.OperationID = op.ID

	// --- Phase 3: stream JSONL → staging ----------------------------------------
	if op.URL == "" {
		// Empty catalogs return no URL. Not an error — just nothing to stream.
		uc.log.Info("shopify_v2_bulk_op_no_url", "shop", shop, "status", op.Status)
		res.Duration = time.Since(start)
		return res, nil
	}
	productCount, err := uc.streamJSONLToStaging(ctx, tenantID, op.URL)
	if err != nil {
		return nil, fmt.Errorf("stream jsonl: %w", err)
	}
	res.ProductCount = productCount

	// Recount collections from the staging table since the bulk JSONL inlines
	// collections under product nodes (no separate top-level "collection"
	// rows are written by streamJSONLToStaging in 4a).
	if counts, err := uc.staging.CountByKind(ctx, tenantID); err == nil {
		res.CollectionCount = counts["collection"]
	}

	res.Duration = time.Since(start)
	uc.log.Info("shopify_v2_dump_to_staging_completed",
		"shop", shop,
		"products", res.ProductCount,
		"metafield_defs", res.MetafieldDefs,
		"vendors", res.VendorCount,
		"duration_ms", res.Duration.Milliseconds(),
	)
	return res, nil
}

// dumpMetafieldDefs writes one staging row per owner type. The discovery agent
// reads these to know which metafields are formally declared (high-signal,
// merchant intentional) vs ad-hoc (lower-signal).
func (uc *ShopifyV2UseCase) dumpMetafieldDefs(ctx context.Context, tenantID, shop, token string, res *DumpToStagingResult) error {
	for _, ownerType := range []string{"PRODUCT", "PRODUCTVARIANT", "COLLECTION"} {
		defs, err := uc.client.MetafieldDefinitions(ctx, shop, token, ownerType)
		if err != nil {
			return fmt.Errorf("%s defs: %w", ownerType, err)
		}
		payload, _ := json.Marshal(defs)
		sourceID := "metafield_defs:" + strings.ToLower(ownerType)
		if err := uc.staging.UpsertRaw(ctx, tenantID, "metadata", sourceID, payload); err != nil {
			return fmt.Errorf("staging write %s: %w", sourceID, err)
		}
		res.MetafieldDefs += len(defs)
	}
	return nil
}

// dumpNavigation snapshots the published storefront menu. Many shops use only
// "main-menu" — a missing menu is logged but not fatal (we'll fall back to
// collection-handle prefixes when building the category tree).
func (uc *ShopifyV2UseCase) dumpNavigation(ctx context.Context, tenantID, shop, token string, res *DumpToStagingResult) error {
	menu, err := uc.client.NavigationMenu(ctx, shop, token, "main-menu")
	if err != nil {
		// Log and continue — navigation is helpful but not required.
		uc.log.Info("shopify_v2_menu_unavailable", "shop", shop, "error", err.Error())
		return nil
	}
	if menu == nil {
		uc.log.Info("shopify_v2_menu_not_present", "shop", shop, "handle", "main-menu")
		return nil
	}
	payload, _ := json.Marshal(menu)
	if err := uc.staging.UpsertRaw(ctx, tenantID, "menu", "main-menu", payload); err != nil {
		return fmt.Errorf("staging menu: %w", err)
	}
	res.NavigationItems = countMenuItems(menu.Items)
	return nil
}

// dumpShopReferences captures the vendor/type/tag universe in one row. The
// discovery agent uses these as quick "what's the shape of this catalog"
// context before requesting samples.
func (uc *ShopifyV2UseCase) dumpShopReferences(ctx context.Context, tenantID, shop, token string, res *DumpToStagingResult) error {
	refs, err := uc.client.ShopReferences(ctx, shop, token)
	if err != nil {
		return fmt.Errorf("references: %w", err)
	}
	payload, _ := json.Marshal(refs)
	if err := uc.staging.UpsertRaw(ctx, tenantID, "metadata", "shop_references", payload); err != nil {
		return fmt.Errorf("staging references: %w", err)
	}
	res.VendorCount = len(refs.ProductVendors)
	res.ProductTypeCount = len(refs.ProductTypes)
	res.TagCount = len(refs.ProductTags)
	return nil
}

// runBulkProductsAndWait fires a new bulk-op and polls until it's terminal.
// If a previous op is still RUNNING for this app+shop, Shopify rejects the
// new RunQuery — we transparently switch to polling that one instead.
func (uc *ShopifyV2UseCase) runBulkProductsAndWait(ctx context.Context, shop, token string) (*shopify.BulkOperation, error) {
	if _, err := uc.client.RunBulkProductsQuery(ctx, shop, token); err != nil {
		// "already in progress" — fall through to polling current op.
		if !strings.Contains(strings.ToLower(err.Error()), "already in progress") {
			return nil, err
		}
		uc.log.Info("shopify_v2_bulk_op_already_running", "shop", shop)
	}

	deadline := time.Now().Add(bulkOpMaxWait)
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("bulk op exceeded %s", bulkOpMaxWait)
		}
		op, err := uc.client.GetCurrentBulkOperation(ctx, shop, token)
		if err != nil {
			return nil, fmt.Errorf("poll: %w", err)
		}
		if op == nil {
			return nil, fmt.Errorf("no current bulk op (race condition?)")
		}
		switch op.Status {
		case shopify.BulkOpCompleted:
			return op, nil
		case shopify.BulkOpFailed, shopify.BulkOpCanceled, shopify.BulkOpExpired:
			return nil, fmt.Errorf("bulk op terminal status %s (errorCode=%s)", op.Status, op.ErrorCode)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(bulkPollInterval):
		}
	}
}

// productAccum holds a partial product reconstruction during JSONL streaming.
// Children (variants/images/metafields/collections) arrive as separate lines
// with __parentId pointing at the product, so we bucket them here until the
// parent product line itself appears.
type productAccum struct {
	productPayload json.RawMessage
	variants       []json.RawMessage
	images         []json.RawMessage
	metafields     []json.RawMessage
	collections    []json.RawMessage
}

// streamJSONLToStaging parses the bulk-op JSONL and writes one staging row per
// top-level product. Variant/image/metafield rows arrive as separate JSONL
// entries with __parentId pointing back at their product — we group them into
// a single "product" payload by reading sequentially and flushing on parent
// change. (The bulk-op spec guarantees each parent's children appear together
// in the file — children for product A, then product A itself, then product B
// children, then product B, and so on. Order: children FIRST, then parent.)
func (uc *ShopifyV2UseCase) streamJSONLToStaging(ctx context.Context, tenantID, jsonlURL string) (int, error) {
	body, err := uc.client.FetchBulkJSONL(ctx, jsonlURL)
	if err != nil {
		return 0, err
	}
	defer body.Close()

	scanner := shopify.ScanBulkJSONL(body)

	pending := make(map[string]*productAccum)
	count := 0

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var head struct {
			ID       string `json:"id"`
			ParentID string `json:"__parentId"`
		}
		if err := json.Unmarshal(line, &head); err != nil {
			uc.log.Info("shopify_v2_jsonl_parse_warning", "error", err.Error())
			continue
		}

		// Make a copy — scanner.Bytes() is reused on next Scan.
		row := make(json.RawMessage, len(line))
		copy(row, line)

		if head.ParentID == "" {
			// Top-level node. We assume it's a Product (the bulk query only
			// has products at top level). Flush accumulated children into
			// this product, write to staging.
			a := pending[head.ID]
			if a == nil {
				a = &productAccum{}
			}
			a.productPayload = row
			merged, err := mergeChildrenIntoProduct(a)
			if err != nil {
				return count, fmt.Errorf("merge children for %s: %w", head.ID, err)
			}
			if err := uc.staging.UpsertRaw(ctx, tenantID, "product", head.ID, merged); err != nil {
				return count, fmt.Errorf("staging upsert %s: %w", head.ID, err)
			}
			delete(pending, head.ID)
			count++
			continue
		}

		// Child row — bucket by parent id and a type discriminator we infer
		// from the GID prefix.
		a := pending[head.ParentID]
		if a == nil {
			a = &productAccum{}
			pending[head.ParentID] = a
		}
		switch {
		case strings.Contains(head.ID, "/ProductVariant/"):
			a.variants = append(a.variants, row)
		case strings.Contains(head.ID, "/MediaImage/"), strings.Contains(head.ID, "/ProductImage/"):
			a.images = append(a.images, row)
		case strings.Contains(head.ID, "/Metafield/"):
			a.metafields = append(a.metafields, row)
		case strings.Contains(head.ID, "/Collection/"):
			a.collections = append(a.collections, row)
		default:
			// Unknown child type — keep as a generic "extras" so we don't drop data.
			a.metafields = append(a.metafields, row)
		}
	}
	if err := scanner.Err(); err != nil {
		return count, fmt.Errorf("scanner: %w", err)
	}
	return count, nil
}

// mergeChildrenIntoProduct splices accumulated child rows into the product
// payload under canonical keys. Even though the bulk query already returns
// nested edges for variants/images/metafields/collections inline, those
// nested arrays are paginated under the hood; the JSONL emission flattens
// them into separate lines. We re-attach under "_v2_*" keys so downstream
// code can prefer them when present without colliding with the top-level
// fields we kept in the GraphQL query.
func mergeChildrenIntoProduct(a *productAccum) (json.RawMessage, error) {
	var product map[string]json.RawMessage
	if err := json.Unmarshal(a.productPayload, &product); err != nil {
		return nil, err
	}
	if len(a.variants) > 0 {
		raw, _ := json.Marshal(a.variants)
		product["_v2_variants"] = raw
	}
	if len(a.images) > 0 {
		raw, _ := json.Marshal(a.images)
		product["_v2_images"] = raw
	}
	if len(a.metafields) > 0 {
		raw, _ := json.Marshal(a.metafields)
		product["_v2_metafields"] = raw
	}
	if len(a.collections) > 0 {
		raw, _ := json.Marshal(a.collections)
		product["_v2_collections"] = raw
	}
	return json.Marshal(product)
}

// countMenuItems totals all nodes in a (possibly-nested) menu tree.
func countMenuItems(items []shopify.NavigationMenuItem) int {
	total := len(items)
	for _, it := range items {
		total += countMenuItems(it.Items)
	}
	return total
}
