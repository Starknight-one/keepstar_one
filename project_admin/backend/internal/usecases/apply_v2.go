// Package usecases — ApplyV2 consumes catalog.inbox_items via the per-tenant
// MappingArtifactV3 and produces master_products + master_cosmetics (or
// tier3 fallback) + slim catalog.products rows.
//
// Run model (per inbox item):
//
//  1. Parse raw JSONB into a generic map.
//  2. Build a classify context from the raw (product_type, brand, name, tags)
//     and call ClassifyVertical → vertical class + source.
//  3. Pick the matching artifact branch (exact vertical match → fallback to
//     "unknown" branch → fallback to single-branch artifact). If no branch
//     covers the row, raise mapping_miss with trigger=unknown_vertical so
//     discovery_v2 can add the branch.
//  4. Walk the branch's field_map: build a MasterProductUpsert + optional
//     MasterCosmeticsUpsert + tier3 patch + listing fields. Override
//     mp.Vertical with the classified value (rules don't get to disagree).
//  5. Compute mp.NormalizedMatchKey via NormalizeMatchKey(brand, name).
//  6. Reject items with no SKU AND no Name (no signal). Synthesize SKU only
//     when SKU is empty but Name is present.
//  7. MatchOrCreateMaster — deterministic SKU → GTIN → NormalizedMatchKey
//     cascade. Returns (id, wasCreated).
//  8. wasCreated branch: write Tier 2 (master_cosmetics) when vertical is
//     cosmetics; write tier3 for everything else; merge tier3 patch.
//  9. !wasCreated branch (bind): master row is IMMUTABLE. Skip cosmetics,
//     skip tier3 writes. Per-tenant marketing fluff will live in
//     listing.tenant_overrides (column reserved; writer wired in a later pass).
// 10. UpsertListing — always. The listing carries price/stock/custom_title.
// 11. Mapping miss path: on transform error, unknown target prefix, or
//     required field absent → wrap as mappingMissErr; caller queues a
//     narrow discovery_v2 run with trigger=mapping_miss carrying the
//     offending rule.
//
// Per-call tenant_action_log row: one "apply" entry summarizing counts,
// status derived from miss/error tally.
package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

const (
	applyV2BatchSize             = 100
	applyV2MaxMissesPerRun       = 3 // cap mapping_miss agent triggers per apply call
)

// errMappingMiss is the sentinel apply_v2 raises when a rule can't be
// applied to one inbox item. The orchestration wraps it with item context
// before logging or triggering a narrow re-discovery.
var errMappingMiss = errors.New("apply_v2: mapping miss")

// MappingMissDetails carries the context apply_v2 hands to discovery_v2
// when it triggers a narrow re-discovery. It's the entire reason
// mapping-miss-driven evolution is cheap — discovery_v2 receives a precise
// hint instead of re-discovering the whole catalog.
type MappingMissDetails struct {
	InboxItemID  string `json:"inbox_item_id"`
	OffendingFrom string `json:"offending_from"` // rule.From that failed
	OffendingTo   string `json:"offending_to"`   // rule.To that failed
	Reason        string `json:"reason"`
}

type ApplyV2UseCase struct {
	inbox     ports.InboxPort
	artifact  ports.MappingArtifactV2Port
	writer    ports.CatalogV2WriterPort
	actionLog ports.TenantActionLogPort
	discovery *DiscoveryV2 // for mapping_miss / first_install cascade
	log       *logger.Logger
}

func NewApplyV2(
	inbox ports.InboxPort,
	artifact ports.MappingArtifactV2Port,
	writer ports.CatalogV2WriterPort,
	actionLog ports.TenantActionLogPort,
	discovery *DiscoveryV2,
	log *logger.Logger,
) *ApplyV2UseCase {
	return &ApplyV2UseCase{
		inbox:     inbox,
		artifact:  artifact,
		writer:    writer,
		actionLog: actionLog,
		discovery: discovery,
		log:       log,
	}
}

// ApplyResult summarises one ApplyForTenant call.
type ApplyResult struct {
	Total         int    `json:"total"`
	Applied       int    `json:"applied"`
	Errors        int    `json:"errors"`
	MappingMisses int    `json:"mapping_misses"`
	Skipped       int    `json:"skipped"`
	FirstError    string `json:"first_error,omitempty"`
}

// ApplyForTenant processes every unapplied inbox row for the tenant via
// the current artifact. Returns counts; never aborts on per-item errors
// (logs them and continues).
func (uc *ApplyV2UseCase) ApplyForTenant(ctx context.Context, tenantID string) (*ApplyResult, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("apply_v2: empty tenant_id")
	}
	artifact, _, err := uc.artifact.Get(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("apply_v2 fetch artifact: %w", err)
	}
	if artifact == nil {
		// No artifact yet — cascade to discovery (first_install path).
		if uc.discovery == nil {
			return nil, fmt.Errorf("apply_v2: no artifact and discovery_v2 not wired")
		}
		a, derr := uc.discovery.Discover(ctx, tenantID, "first_install", nil)
		if derr != nil {
			return nil, fmt.Errorf("apply_v2 cascade discovery: %w", derr)
		}
		if a == nil {
			return nil, fmt.Errorf("apply_v2: discovery produced no artifact")
		}
		artifact = a
	}

	res := &ApplyResult{}
	offset := 0
	missesTriggered := 0

	for {
		items, err := uc.inbox.ListUnapplied(ctx, tenantID, applyV2BatchSize, offset)
		if err != nil {
			return res, fmt.Errorf("apply_v2 list unapplied: %w", err)
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			res.Total++
			err := uc.applyOne(ctx, tenantID, artifact, item)
			if err == nil {
				if mErr := uc.inbox.MarkApplied(ctx, item.ID); mErr != nil {
					uc.log.Warn("apply_v2_mark_applied_failed", "item", item.ID, "error", mErr)
				}
				res.Applied++
				continue
			}
			// Failure path.
			if mm, ok := miss(err); ok {
				res.MappingMisses++
				if res.FirstError == "" {
					res.FirstError = mm.Reason
				}
				if missesTriggered < applyV2MaxMissesPerRun && uc.discovery != nil {
					missesTriggered++
					if err := uc.triggerNarrowDiscovery(ctx, tenantID, item.ID, mm); err != nil {
						uc.log.Warn("apply_v2_mapping_miss_discovery_failed", "tenant", tenantID, "error", err)
					}
					// Re-fetch artifact in case discovery rewrote it.
					if a, _, gErr := uc.artifact.Get(ctx, tenantID); gErr == nil && a != nil {
						artifact = a
					}
				}
				continue
			}
			res.Errors++
			if res.FirstError == "" {
				res.FirstError = err.Error()
			}
			uc.log.Warn("apply_v2_item_failed", "tenant", tenantID, "item", item.ID, "error", err)
		}
		if len(items) < applyV2BatchSize {
			break
		}
		offset += applyV2BatchSize
	}

	status := "ok"
	switch {
	case res.Errors > 0 && res.Applied == 0:
		status = "error"
	case res.Errors > 0 || res.MappingMisses > 0:
		status = "warning"
	}
	payload, _ := json.Marshal(res)
	_ = uc.actionLog.Log(ctx, &ports.TenantActionLogEntry{
		TenantID: tenantID,
		Action:   "apply",
		Status:   status,
		Payload:  payload,
	})
	return res, nil
}

// applyOne runs the transformation+write pipeline for a single inbox item.
// Returns an error wrapping errMappingMiss when the failure is recoverable
// via narrow discovery (transform fail, unknown target prefix, branch
// missing for a freshly-encountered vertical).
func (uc *ApplyV2UseCase) applyOne(ctx context.Context, tenantID string, art *domain.MappingArtifactV3, item *domain.InboxItem) error {
	var raw map[string]any
	if err := json.Unmarshal(item.Raw, &raw); err != nil {
		return fmt.Errorf("apply_v2: parse raw json: %w", err)
	}

	// --- Step 1: classify the row's vertical.
	cctx := classifyContextFromRaw(raw)
	vertical, classifySource := ClassifyVertical(ctx, uc.writer, art.ClassifyRules, cctx)

	// --- Step 2: pick the artifact branch matching that vertical.
	branch := art.FindBranch(vertical)
	if branch == nil {
		// No branch covers this vertical — fire a narrow discovery so the
		// agent adds one. apply_v2's caller catches mapping_miss and queues
		// the run; on the next pass FindBranch will succeed.
		return wrapMiss(item.ID, "", "branch."+vertical,
			fmt.Sprintf("no artifact branch for vertical %q (classified via %s); agent must add it", vertical, classifySource))
	}

	// --- Step 3: walk the branch's rules.
	mp := &ports.MasterProductUpsert{OwnerTenantID: tenantID}
	cosmetics := &ports.MasterCosmeticsUpsert{}
	hasCosmetics := false
	tier3 := map[string]any{}
	var listingPrice, listingStock int
	var listingCurrency, listingTitle string

	for _, rule := range branch.FieldMap {
		val := getPath(raw, rule.From)
		if val == nil {
			if rule.Default == "" {
				continue
			}
			val = rule.Default
		}
		transformed, err := applyV2Transform(val, rule.Transform)
		if err != nil {
			return wrapMiss(item.ID, rule.From, rule.To, fmt.Sprintf("transform %q failed: %v", rule.Transform, err))
		}
		switch {
		case strings.HasPrefix(rule.To, "master."):
			if err := assignMasterField(mp, strings.TrimPrefix(rule.To, "master."), transformed); err != nil {
				return wrapMiss(item.ID, rule.From, rule.To, err.Error())
			}
		case strings.HasPrefix(rule.To, "cosmetics."):
			if vertical != "cosmetics" {
				// Agent emitted a cosmetics.* rule inside a non-cosmetics
				// branch — forgive: route the value to tier3 under the
				// stripped key. (No mapping_miss; this is a soft misuse.)
				key := strings.TrimPrefix(rule.To, "cosmetics.")
				tier3[key] = transformed
				uc.log.Warn("apply_v2_cosmetics_rule_in_non_cosmetics_branch",
					"tenant", tenantID, "vertical", vertical, "rule_to", rule.To, "item", item.ID)
				continue
			}
			if err := assignCosmeticsField(cosmetics, strings.TrimPrefix(rule.To, "cosmetics."), transformed); err != nil {
				return wrapMiss(item.ID, rule.From, rule.To, err.Error())
			}
			hasCosmetics = true
		case strings.HasPrefix(rule.To, "tier3."):
			tier3[strings.TrimPrefix(rule.To, "tier3.")] = transformed
		case strings.HasPrefix(rule.To, "listing."):
			switch strings.TrimPrefix(rule.To, "listing.") {
			case "price_cents":
				if n, ok := asInt(transformed); ok {
					listingPrice = n
				}
			case "currency":
				if s, ok := asString(transformed); ok {
					listingCurrency = s
				}
			case "stock":
				if n, ok := asInt(transformed); ok {
					listingStock = n
				}
			case "custom_title":
				if s, ok := asString(transformed); ok {
					listingTitle = s
				}
			}
		default:
			// Forgiving fallback: agent emitted a per-vertical prefix
			// (e.g. electronics.cpu) for a vertical that lacks a typed
			// table — reroute to tier3 with a warning.
			if dotIdx := strings.IndexByte(rule.To, '.'); dotIdx > 0 {
				col := rule.To[dotIdx+1:]
				if col != "" {
					tier3[col] = transformed
					uc.log.Warn("apply_v2_unknown_vertical_routed_to_tier3",
						"tenant", tenantID, "vertical", vertical, "target", rule.To, "item", item.ID)
					continue
				}
			}
			return wrapMiss(item.ID, rule.From, rule.To, "unknown target prefix")
		}
	}

	// --- Step 4: classified vertical wins. Overrides any rule that set it.
	mp.Vertical = vertical

	// --- Step 5: signal check. No name AND no SKU = junk row.
	if mp.Name == "" && mp.SKU == "" {
		uc.log.Warn("apply_v2_reject_no_signal", "tenant", tenantID, "item", item.ID)
		return wrapMiss(item.ID, "", "master.name|master.sku",
			"row produced neither name nor sku; cannot create or match a master")
	}
	// If we have a name but no SKU, synthesize a stable per-source key. The
	// match cascade still gets a fair shot at stages 2-3 (gtin, normalized
	// match key) before this synthesizes.
	if mp.SKU == "" {
		mp.SKU = fmt.Sprintf("auto-%s-%s", item.SourceKind, item.ExternalID)
	}
	// Compute the normalized match key for stage 3 of the cascade. Empty
	// when both brand and name are blank; adapter skips stage 3 in that
	// case automatically.
	mp.NormalizedMatchKey = NormalizeMatchKey(mp.Brand, mp.Name)

	// --- Step 6: cascade or insert. Adapter is responsible for atomicity.
	masterID, wasCreated, err := uc.writer.MatchOrCreateMaster(ctx, mp)
	if err != nil {
		return fmt.Errorf("apply_v2: match or create master: %w", err)
	}

	// --- Step 7: Tier 2 + tier3 writes happen ONLY when we created the master.
	// On bind, master_products is immutable; per-tenant attribute differences
	// will eventually land in listing.tenant_overrides (column reserved).
	if wasCreated {
		if vertical == "cosmetics" && hasCosmetics {
			if err := uc.writer.UpsertCosmetics(ctx, masterID, cosmetics); err != nil {
				if errors.Is(err, ports.ErrCosmeticsSchemaNotReady) {
					for k, v := range cosmeticsToMap(cosmetics) {
						tier3[k] = v
					}
					uc.log.Warn("apply_v2_cosmetics_fallback_to_tier3",
						"tenant", tenantID, "master", masterID)
				} else {
					return fmt.Errorf("apply_v2: upsert cosmetics: %w", err)
				}
			}
		}
		if len(tier3) > 0 {
			if err := uc.writer.MergeTier3(ctx, masterID, tier3); err != nil {
				return fmt.Errorf("apply_v2: merge tier3: %w", err)
			}
		}
	}

	// --- Step 8: always upsert the listing.
	if _, err := uc.writer.UpsertListing(ctx, &ports.ListingUpsert{
		TenantID:        tenantID,
		MasterProductID: masterID,
		Price:           listingPrice,
		Currency:        listingCurrency,
		Stock:           listingStock,
		CustomTitle:     listingTitle,
		SourceSystem:    string(item.SourceKind),
		SourceID:        item.ExternalID,
	}); err != nil {
		return fmt.Errorf("apply_v2: upsert listing: %w", err)
	}

	return nil
}

// classifyContextFromRaw plucks the four signals ClassifyVertical needs out
// of a Shopify-shaped raw inbox payload. Field names follow Shopify
// (product_type / vendor / title / tags); other source kinds will need
// their own extractor when added.
func classifyContextFromRaw(raw map[string]any) ClassifyContext {
	c := ClassifyContext{}
	if v := getPath(raw, "product_type"); v != nil {
		if s, ok := asString(v); ok {
			c.ProductType = s
		}
	} else if v := getPath(raw, "productType"); v != nil {
		if s, ok := asString(v); ok {
			c.ProductType = s
		}
	}
	if v := getPath(raw, "vendor"); v != nil {
		if s, ok := asString(v); ok {
			c.Brand = s
		}
	} else if v := getPath(raw, "brand"); v != nil {
		if s, ok := asString(v); ok {
			c.Brand = s
		}
	}
	if v := getPath(raw, "title"); v != nil {
		if s, ok := asString(v); ok {
			c.Name = s
		}
	} else if v := getPath(raw, "name"); v != nil {
		if s, ok := asString(v); ok {
			c.Name = s
		}
	}
	if v := getPath(raw, "tags"); v != nil {
		switch t := v.(type) {
		case []any:
			for _, item := range t {
				if s, ok := asString(item); ok && s != "" {
					c.Tags = append(c.Tags, s)
				}
			}
		case string:
			for _, tag := range strings.Split(t, ",") {
				tag = strings.TrimSpace(tag)
				if tag != "" {
					c.Tags = append(c.Tags, tag)
				}
			}
		}
	}
	return c
}

func (uc *ApplyV2UseCase) triggerNarrowDiscovery(ctx context.Context, tenantID, itemID string, mm MappingMissDetails) error {
	mm.InboxItemID = itemID
	payload, _ := json.Marshal(mm)
	_ = uc.actionLog.Log(ctx, &ports.TenantActionLogEntry{
		TenantID: tenantID,
		Action:   "mapping_miss",
		Status:   "warning",
		Payload:  payload,
	})
	_, err := uc.discovery.Discover(ctx, tenantID, "mapping_miss", payload)
	return err
}

// wrapMiss returns an error that miss() can recover into MappingMissDetails.
func wrapMiss(itemID, from, to, reason string) error {
	return &mappingMissErr{
		base:    errMappingMiss,
		details: MappingMissDetails{InboxItemID: itemID, OffendingFrom: from, OffendingTo: to, Reason: reason},
	}
}

type mappingMissErr struct {
	base    error
	details MappingMissDetails
}

func (e *mappingMissErr) Error() string {
	return fmt.Sprintf("%v: from=%q to=%q reason=%s", e.base, e.details.OffendingFrom, e.details.OffendingTo, e.details.Reason)
}
func (e *mappingMissErr) Unwrap() error { return e.base }

func miss(err error) (MappingMissDetails, bool) {
	if err == nil {
		return MappingMissDetails{}, false
	}
	var mm *mappingMissErr
	if errors.As(err, &mm) {
		return mm.details, true
	}
	return MappingMissDetails{}, false
}

// ----- field assignment helpers (deterministic, no DB) -----

func assignMasterField(mp *ports.MasterProductUpsert, col string, val any) error {
	s, _ := asString(val)
	switch col {
	case "name":
		mp.Name = s
	case "brand":
		mp.Brand = s
	case "description":
		mp.Description = s
	case "sku":
		mp.SKU = s
	case "vertical":
		// Per-row vertical is set by ClassifyVertical AFTER all rules run;
		// this assignment is a no-op-style fallback so rules emitting
		// master.vertical don't surface a mapping_miss.
		if s != "" {
			mp.Vertical = s
		}
	case "image_url":
		mp.ImageURL = s
	case "gtin", "barcode", "ean", "upc":
		// All four names map to the same Tier-1 column. Strip non-digits
		// because Shopify barcode fields sometimes include hyphens.
		mp.GTIN = stripGTIN(s)
	default:
		return fmt.Errorf("master.%s is not a known Tier-1 column", col)
	}
	return nil
}

// stripGTIN keeps only digit characters. GTIN is digits-only; merchants
// occasionally upload "00-800001000001" or "8 800001 000001" — the
// equality lookup is byte-for-byte so we must normalize.
func stripGTIN(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			out = append(out, c)
		}
	}
	return string(out)
}

func assignCosmeticsField(c *ports.MasterCosmeticsUpsert, col string, val any) error {
	switch col {
	case "skin_type":
		if v, ok := asStringSlice(val); ok {
			c.SkinType = v
		}
	case "concern":
		if v, ok := asStringSlice(val); ok {
			c.Concern = v
		}
	case "key_ingredients", "ingredients":
		if v, ok := asStringSlice(val); ok {
			c.KeyIngredients = v
		}
	case "target_area":
		if v, ok := asStringSlice(val); ok {
			c.TargetArea = v
		}
	case "product_form":
		if s, ok := asString(val); ok {
			c.ProductForm = &s
		}
	case "texture":
		if s, ok := asString(val); ok {
			c.Texture = &s
		}
	case "routine_step":
		if s, ok := asString(val); ok {
			c.RoutineStep = &s
		}
	case "routine_time":
		if s, ok := asString(val); ok {
			c.RoutineTime = &s
		}
	case "application_method":
		if s, ok := asString(val); ok {
			c.ApplicationMethod = &s
		}
	case "free_from":
		if v, ok := asStringSlice(val); ok {
			c.FreeFrom = v
		}
	case "scent":
		if s, ok := asString(val); ok {
			c.Scent = &s
		}
	case "spf":
		if n, ok := asInt(val); ok {
			c.SPF = &n
		}
	case "marketing_claim":
		if s, ok := asString(val); ok {
			c.MarketingClaim = &s
		}
	case "benefits":
		if v, ok := asStringSlice(val); ok {
			c.Benefits = v
		}
	case "how_to_use":
		if s, ok := asString(val); ok {
			c.HowToUse = &s
		}
	case "volume_ml":
		if n, ok := asInt(val); ok {
			c.VolumeML = &n
		}
	case "weight_g":
		if n, ok := asInt(val); ok {
			c.WeightG = &n
		}
	case "unit_count":
		if n, ok := asInt(val); ok {
			c.UnitCount = &n
		}
	default:
		// Unknown cosmetic-named field → stash in Extra so it survives.
		if c.Extra == nil {
			c.Extra = map[string]any{}
		}
		c.Extra[col] = val
	}
	return nil
}

// cosmeticsToMap flattens MasterCosmeticsUpsert into a plain map for tier3
// fallback (used when master_cosmetics schema isn't reshaped yet or
// vertical mismatch).
func cosmeticsToMap(c *ports.MasterCosmeticsUpsert) map[string]any {
	out := map[string]any{}
	if len(c.SkinType) > 0 {
		out["skin_type"] = c.SkinType
	}
	if len(c.Concern) > 0 {
		out["concern"] = c.Concern
	}
	if len(c.KeyIngredients) > 0 {
		out["key_ingredients"] = c.KeyIngredients
	}
	if len(c.TargetArea) > 0 {
		out["target_area"] = c.TargetArea
	}
	if c.ProductForm != nil {
		out["product_form"] = *c.ProductForm
	}
	if c.Texture != nil {
		out["texture"] = *c.Texture
	}
	if c.RoutineStep != nil {
		out["routine_step"] = *c.RoutineStep
	}
	if c.RoutineTime != nil {
		out["routine_time"] = *c.RoutineTime
	}
	if c.ApplicationMethod != nil {
		out["application_method"] = *c.ApplicationMethod
	}
	if len(c.FreeFrom) > 0 {
		out["free_from"] = c.FreeFrom
	}
	if c.Scent != nil {
		out["scent"] = *c.Scent
	}
	if c.SPF != nil {
		out["spf"] = *c.SPF
	}
	if c.MarketingClaim != nil {
		out["marketing_claim"] = *c.MarketingClaim
	}
	if len(c.Benefits) > 0 {
		out["benefits"] = c.Benefits
	}
	if c.HowToUse != nil {
		out["how_to_use"] = *c.HowToUse
	}
	if c.VolumeML != nil {
		out["volume_ml"] = *c.VolumeML
	}
	if c.WeightG != nil {
		out["weight_g"] = *c.WeightG
	}
	if c.UnitCount != nil {
		out["unit_count"] = *c.UnitCount
	}
	for k, v := range c.Extra {
		out[k] = v
	}
	return out
}
