// Package usecases — discovery_v2_tools defines the tool surface exposed
// to the LLM agent and the dispatcher that runs them against InboxPort.
//
// Why split from discovery_v2.go: the agent loop is the orchestration; the
// tool defs + JSON schemas + arg-parsing live here. Keeps both files
// reviewable.
package usecases

import (
	"context"
	"encoding/json"
	"fmt"

	"keepstar-admin/internal/adapters/anthropic"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/ports"
)

// discoveryToolName aliases for readability.
const (
	// Inbox-side tools: explore the tenant's raw data.
	toolCountTotal   = "count_total"
	toolListFields   = "list_fields"
	toolSampleValues = "sample_values"
	toolCountBy      = "count_by"
	toolFieldStats   = "field_stats"
	toolPeekFullRows = "peek_full_rows"

	// Master-side tools: see what the existing master catalog already
	// holds, so the agent can mirror structure instead of inventing
	// parallel taxonomies. Added in the catalog-flow bundled milestone.
	toolListMasterCategories       = "list_master_categories"
	toolDigestMasterCategory       = "digest_master_category"
	toolFindMasterBySKU            = "find_master_by_sku"
	toolFindMasterByGTIN           = "find_master_by_gtin"
	toolListMasterBrandsInCategory = "list_master_brands_in_category"

	// Terminal tool: emit the artifact and exit the loop.
	toolCommitArtifact = "commit_artifact"
)

// discoveryTools returns the ToolDef list passed to Anthropic. CacheControl
// is set on the LAST tool — Anthropic caches everything up to and including
// the marked block, so the entire tool list (which is identical across
// turns) gets cached.
func discoveryTools() []anthropic.ToolDef {
	defs := []anthropic.ToolDef{
		{
			Name:        toolCountTotal,
			Description: "Return the total number of inbox rows for this tenant. No arguments.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		},
		{
			Name: toolListFields,
			Description: "Return up to `limit` distinct top-level JSONB keys observed across all inbox " +
				"rows for this tenant. Use this first to see the shape of the source data. Default limit 100.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{"limit":{"type":"integer","minimum":1,"maximum":500}},
				"additionalProperties":false
			}`),
		},
		{
			Name: toolSampleValues,
			Description: "Return up to `limit` distinct stringified values for ONE top-level field. " +
				"Cheap; safe to call for many fields. Default limit 50.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"field":{"type":"string"},
					"limit":{"type":"integer","minimum":1,"maximum":200}
				},
				"required":["field"],
				"additionalProperties":false
			}`),
		},
		{
			Name: toolCountBy,
			Description: "Return value→count distribution for ONE field, top-N buckets by frequency. " +
				"Use when you want to see how skewed a field is. Default max_buckets 50.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"field":{"type":"string"},
					"max_buckets":{"type":"integer","minimum":1,"maximum":500}
				},
				"required":["field"],
				"additionalProperties":false
			}`),
		},
		{
			Name: toolFieldStats,
			Description: "Compact stats for ONE field: non-null count, distinct count, sample values, " +
				"numeric min/max (if applicable), length min/max. One call per field; cheap.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"field":{"type":"string"},
					"sample_n":{"type":"integer","minimum":1,"maximum":100}
				},
				"required":["field"],
				"additionalProperties":false
			}`),
		},
		{
			Name: toolPeekFullRows,
			Description: "HEAVY — returns 1..5 ENTIRE raw inbox rows. Use sparingly when aggregates " +
				"aren't enough to disambiguate. Counted against a per-run cap of 10 calls; further " +
				"calls return an error. Default limit 3.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{"limit":{"type":"integer","minimum":1,"maximum":5}},
				"additionalProperties":false
			}`),
		},
		{
			Name: toolListMasterCategories,
			Description: "Return all master categories for a vertical (default vertical='cosmetics'). Each " +
				"node has id, slug, name, parent_id (root if empty), vertical, and product_count " +
				"(how many master_products already live there via the M:N table). Use this BEFORE " +
				"writing field_map rules so you can mirror the existing taxonomy instead of " +
				"inventing parallel structures.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"vertical":{"type":"string"},
					"limit":{"type":"integer","minimum":1,"maximum":500}
				},
				"additionalProperties":false
			}`),
		},
		{
			Name: toolDigestMasterCategory,
			Description: "Return compact roll-up for ONE master category: name, vertical, product_count, " +
				"top 10 brands by frequency, 10 most recent product names. Use to understand what " +
				"data shape the agent should mirror when mapping inbox fields into this category.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{"category_id":{"type":"string"}},
				"required":["category_id"],
				"additionalProperties":false
			}`),
		},
		{
			Name: toolFindMasterBySKU,
			Description: "Resolve a SKU to a master_product reference (id, sku, name, brand, vertical, " +
				"gtin). Case-insensitive. Returns null when no master has this SKU. Use to decide " +
				"whether an inbox row should bind to an existing master vs create a new one.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{"sku":{"type":"string"}},
				"required":["sku"],
				"additionalProperties":false
			}`),
		},
		{
			Name: toolFindMasterByGTIN,
			Description: "Resolve a GTIN/UPC/EAN to a master_product reference. Exact match. Returns null " +
				"when no master has this GTIN.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{"gtin":{"type":"string"}},
				"required":["gtin"],
				"additionalProperties":false
			}`),
		},
		{
			Name: toolListMasterBrandsInCategory,
			Description: "Top brands in ONE master category, ordered by product count desc. Default " +
				"limit 30, max 100.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"category_id":{"type":"string"},
					"limit":{"type":"integer","minimum":1,"maximum":100}
				},
				"required":["category_id"],
				"additionalProperties":false
			}`),
		},
		{
			Name: toolCommitArtifact,
			Description: "Finalize discovery and exit. Submit the artifact as branches by vertical class plus " +
				"optional classify_rules. One branch per vertical the tenant carries (cosmetics, electronics, " +
				"furniture, haircare, apparel, footwear, food, ski, unknown). Each branch has its own field_map: " +
				"target prefixes are master.<col> for Tier 1 columns, cosmetics.<col> ONLY when the branch is " +
				"'cosmetics' (typed table exists), and tier3.<key> for every other vertical's attributes. " +
				"classify_rules are evaluated row-by-row when Shopify product_type doesn't alias to a known " +
				"vertical — supported DSL: \"product_type contains 'X'\", \"brand = 'X'\", \"name contains 'X'\", " +
				"\"tag = 'X'\".",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"classifying_field":{
						"type":"string",
						"description":"Name of the inbox top-level key whose value identifies this row's category (e.g. 'product_type' for Shopify, 'primary_category' for Sephora CSV). Apply_v2 reads raw[classifying_field] as the input to vertical lookup."
					},
					"branches":{
						"type":"array",
						"minItems":1,
						"items":{
							"type":"object",
							"properties":{
								"vertical":{"type":"string","enum":["cosmetics","electronics","furniture","haircare","apparel","footwear","food","ski","unknown"]},
								"field_map":{
									"type":"array",
									"items":{
										"type":"object",
										"properties":{
											"from":{"type":"string"},
											"to":{"type":"string"},
											"transform":{"type":"string"},
											"default":{"type":"string"}
										},
										"required":["from","to"],
										"additionalProperties":false
									}
								}
							},
							"required":["vertical","field_map"],
							"additionalProperties":false
						}
					},
					"classify_rules":{
						"type":"array",
						"items":{
							"type":"object",
							"properties":{
								"when":{"type":"string"},
								"then_vertical":{"type":"string"}
							},
							"required":["when","then_vertical"],
							"additionalProperties":false
						}
					},
					"notes":{"type":"string"}
				},
				"required":["branches"],
				"additionalProperties":false
			}`),
		},
	}
	// Mark cache breakpoint on the last tool — Anthropic caches up to and
	// including it. Keep the tool list stable across turns to hit the cache.
	if n := len(defs); n > 0 {
		defs[n-1].CacheControl = &anthropic.CacheControl{Type: "ephemeral"}
	}
	return defs
}

// dispatchResult is what dispatchTool returns to the loop: rendered string
// for the LLM tool_result, a preview for agent_runs logging, plus the
// optional "committed artifact" signal.
type dispatchResult struct {
	Output         string                    // JSON-encoded result for LLM
	Preview        json.RawMessage           // compact preview for agent_runs log
	IsError        bool                      // surfaced to LLM as tool_result is_error
	CommitArtifact *domain.MappingArtifactV3 // non-nil iff commit_artifact was called
}

// dispatchTool runs one tool invocation. Caller is responsible for
// enforcing per-call ordering and budget — this function only touches the
// heavy-tool counter. The master-side digest port may be nil when this
// dispatcher is exercised in isolation (e.g. unit tests over inbox-only
// flows); master tool calls in that mode return a clear "not wired" error
// rather than crashing.
func dispatchTool(
	ctx context.Context,
	inbox ports.InboxPort,
	digest ports.MasterDigestPort,
	tenantID string,
	name string,
	args json.RawMessage,
	heavyCallsUsed *int,
	heavyCallsCap int,
) *dispatchResult {
	switch name {
	case toolCountTotal:
		n, err := inbox.CountTotal(ctx, tenantID)
		if err != nil {
			return errResult(err)
		}
		return okJSON(map[string]int{"total": n})

	case toolListFields:
		var a struct{ Limit int `json:"limit"` }
		_ = json.Unmarshal(args, &a)
		if a.Limit == 0 {
			a.Limit = 100
		}
		fs, err := inbox.ListFields(ctx, tenantID, a.Limit)
		if err != nil {
			return errResult(err)
		}
		return okJSON(map[string]any{"fields": fs, "count": len(fs)})

	case toolSampleValues:
		var a struct {
			Field string `json:"field"`
			Limit int    `json:"limit"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.Field == "" {
			return errResult(fmt.Errorf("sample_values: field required"))
		}
		if a.Limit == 0 {
			a.Limit = 50
		}
		vs, err := inbox.SampleValues(ctx, tenantID, a.Field, a.Limit)
		if err != nil {
			return errResult(err)
		}
		return okJSON(map[string]any{"field": a.Field, "values": vs, "count": len(vs)})

	case toolCountBy:
		var a struct {
			Field      string `json:"field"`
			MaxBuckets int    `json:"max_buckets"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.Field == "" {
			return errResult(fmt.Errorf("count_by: field required"))
		}
		if a.MaxBuckets == 0 {
			a.MaxBuckets = 50
		}
		dist, err := inbox.CountBy(ctx, tenantID, a.Field, a.MaxBuckets)
		if err != nil {
			return errResult(err)
		}
		return okJSON(map[string]any{"field": a.Field, "distribution": dist})

	case toolFieldStats:
		var a struct {
			Field    string `json:"field"`
			SampleN  int    `json:"sample_n"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.Field == "" {
			return errResult(fmt.Errorf("field_stats: field required"))
		}
		if a.SampleN == 0 {
			a.SampleN = 20
		}
		st, err := inbox.FieldStats(ctx, tenantID, a.Field, a.SampleN)
		if err != nil {
			return errResult(err)
		}
		return okJSON(st)

	case toolPeekFullRows:
		if *heavyCallsUsed >= heavyCallsCap {
			return errResult(fmt.Errorf("peek_full_rows: per-run cap of %d reached; rely on aggregate tools", heavyCallsCap))
		}
		*heavyCallsUsed++
		var a struct{ Limit int `json:"limit"` }
		_ = json.Unmarshal(args, &a)
		if a.Limit == 0 {
			a.Limit = 3
		}
		rows, err := inbox.PeekFullRows(ctx, tenantID, a.Limit)
		if err != nil {
			return errResult(err)
		}
		// Build a JSON array of raw payloads + minimal envelope.
		outs := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			outs = append(outs, map[string]any{
				"external_id": r.ExternalID,
				"source_kind": r.SourceKind,
				"raw":         r.Raw,
			})
		}
		preview, _ := json.Marshal(map[string]any{"rows_returned": len(outs)})
		body, _ := json.Marshal(map[string]any{"rows": outs})
		return &dispatchResult{Output: string(body), Preview: preview}

	case toolListMasterCategories:
		if digest == nil {
			return errResult(fmt.Errorf("%s: master digest port not wired", name))
		}
		var a struct {
			Vertical string `json:"vertical"`
			Limit    int    `json:"limit"`
		}
		_ = json.Unmarshal(args, &a)
		nodes, err := digest.ListMasterCategories(ctx, a.Vertical, a.Limit)
		if err != nil {
			return errResult(err)
		}
		return okJSON(map[string]any{"vertical": a.Vertical, "categories": nodes, "count": len(nodes)})

	case toolDigestMasterCategory:
		if digest == nil {
			return errResult(fmt.Errorf("%s: master digest port not wired", name))
		}
		var a struct {
			CategoryID string `json:"category_id"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.CategoryID == "" {
			return errResult(fmt.Errorf("digest_master_category: category_id required"))
		}
		d, err := digest.DigestMasterCategory(ctx, a.CategoryID)
		if err != nil {
			return errResult(err)
		}
		return okJSON(d)

	case toolFindMasterBySKU:
		if digest == nil {
			return errResult(fmt.Errorf("%s: master digest port not wired", name))
		}
		var a struct {
			SKU string `json:"sku"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.SKU == "" {
			return errResult(fmt.Errorf("find_master_by_sku: sku required"))
		}
		ref, err := digest.FindMasterBySKU(ctx, a.SKU)
		if err != nil {
			return errResult(err)
		}
		return okJSON(map[string]any{"sku": a.SKU, "master": ref, "found": ref != nil})

	case toolFindMasterByGTIN:
		if digest == nil {
			return errResult(fmt.Errorf("%s: master digest port not wired", name))
		}
		var a struct {
			GTIN string `json:"gtin"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.GTIN == "" {
			return errResult(fmt.Errorf("find_master_by_gtin: gtin required"))
		}
		ref, err := digest.FindMasterByGTIN(ctx, a.GTIN)
		if err != nil {
			return errResult(err)
		}
		return okJSON(map[string]any{"gtin": a.GTIN, "master": ref, "found": ref != nil})

	case toolListMasterBrandsInCategory:
		if digest == nil {
			return errResult(fmt.Errorf("%s: master digest port not wired", name))
		}
		var a struct {
			CategoryID string `json:"category_id"`
			Limit      int    `json:"limit"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.CategoryID == "" {
			return errResult(fmt.Errorf("list_master_brands_in_category: category_id required"))
		}
		brands, err := digest.ListMasterBrandsInCategory(ctx, a.CategoryID, a.Limit)
		if err != nil {
			return errResult(err)
		}
		return okJSON(map[string]any{"category_id": a.CategoryID, "brands": brands, "count": len(brands)})

	case toolCommitArtifact:
		var a struct {
			ClassifyingField string                  `json:"classifying_field"`
			Branches         []domain.VerticalBranch `json:"branches"`
			ClassifyRules    []domain.ClassifyRule   `json:"classify_rules"`
			Notes            string                  `json:"notes"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errResult(fmt.Errorf("commit_artifact: invalid args: %w", err))
		}
		// Validation suite. Each failure returns tool_error so the agent
		// can self-correct in the same run instead of committing junk.
		if len(a.Branches) == 0 {
			return errResult(fmt.Errorf("commit_artifact: at least one branch required"))
		}
		// classifying_field is RECOMMENDED but not required: legacy
		// artifacts (v3 written before this field existed) keep working
		// through classifyContextFromRaw's synonym fallback. When the
		// agent does provide it, we validate that the name is real.
		if a.ClassifyingField != "" {
			inboxFields, fieldsErr := inbox.ListFields(ctx, tenantID, 500)
			if fieldsErr == nil && len(inboxFields) > 0 {
				found := false
				for _, f := range inboxFields {
					if f == a.ClassifyingField {
						found = true
						break
					}
				}
				if !found {
					return errResult(fmt.Errorf("commit_artifact: classifying_field %q not present in inbox top-level keys; call list_fields and pick a real one", a.ClassifyingField))
				}
			}
		}
		ruleCount := 0
		for _, b := range a.Branches {
			if b.Vertical == "" {
				return errResult(fmt.Errorf("commit_artifact: every branch needs a vertical"))
			}
			if len(b.FieldMap) == 0 {
				return errResult(fmt.Errorf("commit_artifact: branch %q has empty field_map", b.Vertical))
			}
			ruleCount += len(b.FieldMap)
		}
		// Smoke-test the proposed classify rules + classifying_field on a
		// real sample of the inbox. Only run when classifying_field is set
		// AND the sample is large enough to be meaningful — protects the
		// legacy artifact path (which never set classifying_field) from
		// spurious tool_error noise. The check fails the commit only when
		// the agent's classification provably misroutes >80% of sampled
		// rows; that is the path that would otherwise cost $0.10 per row
		// in mapping_miss retries.
		if a.ClassifyingField != "" {
			if covered, sampled, smokeErr := smokeTestClassify(ctx, inbox, tenantID, a.ClassifyingField, a.ClassifyRules); smokeErr == nil && sampled >= 5 {
				coverage := float64(covered) / float64(sampled)
				if coverage < 0.20 {
					return errResult(fmt.Errorf("commit_artifact: classify smoke-test failed — only %d/%d (%.0f%%) sample rows classified to a known vertical. Either classifying_field=%q is wrong or classify_rules don't match real values. Use sample_values/count_by to inspect %q and try again", covered, sampled, coverage*100, a.ClassifyingField, a.ClassifyingField))
				}
			}
		}
		art := &domain.MappingArtifactV3{
			Version:          3,
			ClassifyingField: a.ClassifyingField,
			Branches:         a.Branches,
			ClassifyRules:    a.ClassifyRules,
			Notes:            a.Notes,
		}
		body, _ := json.Marshal(map[string]any{
			"committed":         true,
			"classifying_field": a.ClassifyingField,
			"branches":          art.VerticalNames(),
			"total_rules":       ruleCount,
			"classify_rules":    len(a.ClassifyRules),
		})
		preview, _ := json.Marshal(map[string]any{
			"classifying_field": a.ClassifyingField,
			"branches":          art.VerticalNames(),
			"rules":             ruleCount,
			"classify_rules":    len(a.ClassifyRules),
		})
		return &dispatchResult{Output: string(body), Preview: preview, CommitArtifact: art}

	default:
		return errResult(fmt.Errorf("unknown tool: %s", name))
	}
}

// smokeTestClassify pulls a small sample of inbox rows and runs the agent's
// proposed classifying_field + classify_rules against them. Returns the
// number of rows that mapped to a non-empty, non-"unknown" vertical and the
// total sample size. Cheap one-call check — bounded to PeekFullRows limit.
//
// This is a CORRECTNESS guard: it doesn't block on aesthetics, only when
// the agent's classification logic provably fails on real data.
func smokeTestClassify(ctx context.Context, inbox ports.InboxPort, tenantID, classifyingField string, rules []domain.ClassifyRule) (covered int, sampled int, err error) {
	rows, err := inbox.PeekFullRows(ctx, tenantID, 5)
	if err != nil {
		return 0, 0, err
	}
	for _, row := range rows {
		var raw map[string]any
		if err := json.Unmarshal(row.Raw, &raw); err != nil {
			continue
		}
		sampled++
		var ptVal string
		if v, ok := raw[classifyingField]; ok {
			if s, ok2 := v.(string); ok2 {
				ptVal = s
			}
		}
		cctx := ClassifyContext{ProductType: ptVal}
		// Brand + Name are best-effort filled from common synonyms so
		// rules that lean on brand=/name contains keep their chance.
		for _, k := range []string{"brand", "vendor", "brand_name"} {
			if v, ok := raw[k]; ok {
				if s, ok2 := v.(string); ok2 && s != "" {
					cctx.Brand = s
					break
				}
			}
		}
		for _, k := range []string{"name", "title", "product_name"} {
			if v, ok := raw[k]; ok {
				if s, ok2 := v.(string); ok2 && s != "" {
					cctx.Name = s
					break
				}
			}
		}
		// We deliberately pass nil VerticalAliasLookup — smoke-test only
		// evaluates the artifact's own rules + the synonym-bound name.
		vertical, _ := ClassifyVertical(ctx, nil, rules, cctx)
		if vertical != "" && vertical != "unknown" {
			covered++
		}
	}
	return covered, sampled, nil
}

// okJSON marshals v to JSON and packages it as a dispatchResult.
func okJSON(v any) *dispatchResult {
	body, _ := json.Marshal(v)
	preview := body
	if len(preview) > 500 {
		preview = json.RawMessage(`{"truncated":true}`)
	}
	return &dispatchResult{Output: string(body), Preview: preview}
}

func errResult(err error) *dispatchResult {
	msg := err.Error()
	body, _ := json.Marshal(map[string]string{"error": msg})
	return &dispatchResult{Output: string(body), IsError: true, Preview: json.RawMessage(fmt.Sprintf(`{"error":%q}`, msg))}
}
