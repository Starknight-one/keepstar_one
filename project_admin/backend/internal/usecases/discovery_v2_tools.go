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
	"strings"

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

	// Builder tools: incrementally assemble the artifact with at-call-time
	// validation. Each mutation lands in the run-scoped discoveryDraft;
	// validation errors return tool_error so the agent self-corrects in
	// the same run rather than re-rolling a whole artifact JSON.
	toolSetClassifyingField = "set_classifying_field"
	toolAddBranch           = "add_branch"
	toolAddFieldMapping     = "add_field_mapping"
	toolAddClassifyRule     = "add_classify_rule"
	toolRemoveFieldMapping  = "remove_field_mapping"
	toolRemoveClassifyRule  = "remove_classify_rule"
	toolInspectDraft        = "inspect_draft"
	toolCommit              = "commit"
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
		// --- BUILDER TOOLS (mutate the run-scoped discoveryDraft) ---
		{
			Name: toolSetClassifyingField,
			Description: "Pick the inbox top-level key whose value identifies a row's category. " +
				"Common names: product_type (Shopify), primary_category (Sephora-CSV), " +
				"category, type, kind, department. The right field has DISCRETE STRING values " +
				"(Skincare, Makeup, Fragrance, Phone) — not numbers, not free text. " +
				"Validation rejects fields with no string samples. Call sample_values on candidates first.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{"field":{"type":"string","minLength":1,"maxLength":120,"pattern":"^[a-zA-Z0-9_\\-]+$"}},
				"required":["field"],
				"additionalProperties":false
			}`),
		},
		{
			Name: toolAddBranch,
			Description: "Declare a branch for one vertical class. Each tenant has 1..N branches " +
				"depending on what they sell. Common: cosmetics, haircare, unknown (fallback). " +
				"Mixed retailers can have several. Tier 2 typed columns (cosmetics.*) only valid " +
				"inside the cosmetics branch.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"vertical":{"type":"string","enum":["cosmetics","electronics","furniture","haircare","apparel","footwear","food","ski","unknown"]}
				},
				"required":["vertical"],
				"additionalProperties":false
			}`),
		},
		{
			Name: toolAddFieldMapping,
			Description: "Add one mapping rule to a branch: how an inbox field becomes a master / cosmetics / tier3 value. " +
				"`from` must be an existing inbox top-level key (validator returns typo suggestions). " +
				"`to` is <prefix>.<col>: master.{name,brand,description,sku,vertical,image_url,gtin}; " +
				"cosmetics.{skin_type,concern,key_ingredients,target_area,free_from,benefits,product_form,texture,routine_step,routine_time,application_method,scent,marketing_claim,how_to_use,spf,volume_ml,weight_g,unit_count} (cosmetics branch only); " +
				"tier3.<any_snake_case_key>. " +
				"transform applies to the value before write: lowercase, trim, split (requires split_delim), " +
				"ml_from_string, g_from_string, bool_from_yesno, int, numeric. " +
				"Split is only valid for cosmetics.<array_col>. " +
				"numeric/int/ml_from_string/g_from_string require a numeric target (gtin, spf, volume_ml, weight_g, unit_count, or tier3.*).",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"vertical":{"type":"string","enum":["cosmetics","electronics","furniture","haircare","apparel","footwear","food","ski","unknown"]},
					"from":{"type":"string","minLength":1,"maxLength":200},
					"to":{"type":"string","pattern":"^(master|cosmetics|tier3)\\.[a-z][a-z0-9_]*$"},
					"transform":{"type":"string","enum":["","lowercase","trim","split","ml_from_string","g_from_string","bool_from_yesno","int","numeric"]},
					"split_delim":{"type":"string","maxLength":4},
					"default":{"type":"string","maxLength":500}
				},
				"required":["vertical","from","to"],
				"additionalProperties":false
			}`),
		},
		{
			Name: toolAddClassifyRule,
			Description: "Add one classification rule. Apply evaluates these row-by-row when the alias lookup misses, " +
				"first match wins. signal is what apply will read from the row: product_type maps to " +
				"raw[classifying_field]; brand/name fall back to common synonyms (brand_name, product_name); " +
				"tag iterates the row's tags array. op is contains or =. value is the literal substring/exact " +
				"match. then_vertical must already have a branch declared. The validator runs a quick sample " +
				"check — rules that match 0/N sampled values are rejected as dead.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"signal":{"type":"string","enum":["product_type","brand","name","tag"]},
					"op":{"type":"string","enum":["contains","="]},
					"value":{"type":"string","minLength":1,"maxLength":120},
					"then_vertical":{"type":"string","enum":["cosmetics","electronics","furniture","haircare","apparel","footwear","food","ski","unknown"]}
				},
				"required":["signal","op","value","then_vertical"],
				"additionalProperties":false
			}`),
		},
		{
			Name:        toolRemoveFieldMapping,
			Description: "Delete a previously-added field mapping by exact (vertical, from, to) match. Use this when swapping a rule instead of leaving a duplicate.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"vertical":{"type":"string","enum":["cosmetics","electronics","furniture","haircare","apparel","footwear","food","ski","unknown"]},
					"from":{"type":"string"},
					"to":{"type":"string"}
				},
				"required":["vertical","from","to"],
				"additionalProperties":false
			}`),
		},
		{
			Name:        toolRemoveClassifyRule,
			Description: "Delete a previously-added classify_rule by exact (signal, op, value) match.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"signal":{"type":"string","enum":["product_type","brand","name","tag"]},
					"op":{"type":"string","enum":["contains","="]},
					"value":{"type":"string"}
				},
				"required":["signal","op","value"],
				"additionalProperties":false
			}`),
		},
		{
			Name: toolInspectDraft,
			Description: "Read-only — return the current draft state and a coverage snapshot (smoke-test on 5 sample rows). Use before commit to verify everything looks right.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		},
		{
			Name: toolCommit,
			Description: "Finalize the draft and exit the run. Validates: classifying_field set, ≥1 branch, every branch has ≥1 mapping, classify smoke-test passes ≥20% coverage on 10 sampled rows. Notes are optional free text for the curator.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{"notes":{"type":"string","maxLength":2000}},
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
// rather than crashing. draft is the per-run artifact being assembled by
// the builder tools (set_classifying_field, add_branch, …, commit). Old
// non-builder call sites pass nil; builder tool calls then return a
// "draft not wired" error.
func dispatchTool(
	ctx context.Context,
	inbox ports.InboxPort,
	digest ports.MasterDigestPort,
	draft *discoveryDraft,
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

	case toolSetClassifyingField:
		if draft == nil {
			return errResult(fmt.Errorf("%s: draft not wired", name))
		}
		var a struct {
			Field string `json:"field"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.Field == "" {
			return errResult(fmt.Errorf("set_classifying_field: field required"))
		}
		inboxFields, _ := inbox.ListFields(ctx, tenantID, 500)
		samples, _ := inbox.SampleValues(ctx, tenantID, a.Field, 10)
		if err := draft.setClassifyingField(a.Field, inboxFields, samples); err != nil {
			return errResult(err)
		}
		return okJSON(map[string]any{"classifying_field": draft.ClassifyingField})

	case toolAddBranch:
		if draft == nil {
			return errResult(fmt.Errorf("%s: draft not wired", name))
		}
		var a struct {
			Vertical string `json:"vertical"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errResult(fmt.Errorf("add_branch: invalid args: %w", err))
		}
		if err := draft.addBranch(a.Vertical); err != nil {
			return errResult(err)
		}
		return okJSON(map[string]any{"branches": draftVerticals(draft)})

	case toolAddFieldMapping:
		if draft == nil {
			return errResult(fmt.Errorf("%s: draft not wired", name))
		}
		var a struct {
			Vertical   string `json:"vertical"`
			From       string `json:"from"`
			To         string `json:"to"`
			Transform  string `json:"transform"`
			SplitDelim string `json:"split_delim"`
			Default    string `json:"default"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errResult(fmt.Errorf("add_field_mapping: invalid args: %w", err))
		}
		inboxFields, _ := inbox.ListFields(ctx, tenantID, 500)
		if err := draft.addFieldMapping(a.Vertical, a.From, a.To, a.Transform, a.SplitDelim, a.Default, inboxFields); err != nil {
			return errResult(err)
		}
		return okJSON(map[string]any{
			"vertical": a.Vertical, "from": a.From, "to": a.To,
			"branch_rule_count": len(branchByName(draft, a.Vertical).FieldMap),
		})

	case toolAddClassifyRule:
		if draft == nil {
			return errResult(fmt.Errorf("%s: draft not wired", name))
		}
		var a struct {
			Signal       string `json:"signal"`
			Op           string `json:"op"`
			Value        string `json:"value"`
			ThenVertical string `json:"then_vertical"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errResult(fmt.Errorf("add_classify_rule: invalid args: %w", err))
		}
		// Best-effort dead-rule check: fetch samples for the field the
		// signal actually reads. signal=product_type → samples from
		// draft.ClassifyingField; other signals reuse a small synonym
		// list. If draft.ClassifyingField isn't set yet, skip the dead
		// check (validator will still enforce branch existence + enums).
		var samples []string
		if a.Signal == "product_type" && draft.ClassifyingField != "" {
			samples, _ = inbox.SampleValues(ctx, tenantID, draft.ClassifyingField, 50)
		}
		if err := draft.addClassifyRule(a.Signal, a.Op, a.Value, a.ThenVertical, samples); err != nil {
			return errResult(err)
		}
		return okJSON(map[string]any{
			"classify_rule_count": len(draft.ClassifyRules),
		})

	case toolRemoveFieldMapping:
		if draft == nil {
			return errResult(fmt.Errorf("%s: draft not wired", name))
		}
		var a struct {
			Vertical string `json:"vertical"`
			From     string `json:"from"`
			To       string `json:"to"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errResult(fmt.Errorf("remove_field_mapping: invalid args: %w", err))
		}
		if err := draft.removeFieldMapping(a.Vertical, a.From, a.To); err != nil {
			return errResult(err)
		}
		return okJSON(map[string]any{"removed": true})

	case toolRemoveClassifyRule:
		if draft == nil {
			return errResult(fmt.Errorf("%s: draft not wired", name))
		}
		var a struct {
			Signal string `json:"signal"`
			Op     string `json:"op"`
			Value  string `json:"value"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errResult(fmt.Errorf("remove_classify_rule: invalid args: %w", err))
		}
		if err := draft.removeClassifyRule(a.Signal, a.Op, a.Value); err != nil {
			return errResult(err)
		}
		return okJSON(map[string]any{"removed": true})

	case toolInspectDraft:
		if draft == nil {
			return errResult(fmt.Errorf("%s: draft not wired", name))
		}
		// Compose a compact snapshot. Run a 5-row smoke test to show
		// coverage so far (only if classifying_field is set).
		coverage := map[string]any{"sampled": 0, "covered": 0}
		if draft.ClassifyingField != "" {
			if cv, sm, smokeErr := smokeTestClassify(ctx, inbox, tenantID, draft.ClassifyingField, draft.ClassifyRules); smokeErr == nil {
				coverage = map[string]any{"sampled": sm, "covered": cv}
			}
		}
		branchSummary := make([]map[string]any, 0, len(draft.Branches))
		for _, b := range draft.Branches {
			branchSummary = append(branchSummary, map[string]any{
				"vertical":   b.Vertical,
				"rule_count": len(b.FieldMap),
			})
		}
		return okJSON(map[string]any{
			"classifying_field":   draft.ClassifyingField,
			"branches":            branchSummary,
			"classify_rule_count": len(draft.ClassifyRules),
			"coverage_so_far":     coverage,
		})

	case toolCommit:
		if draft == nil {
			return errResult(fmt.Errorf("%s: draft not wired", name))
		}
		var a struct {
			Notes string `json:"notes"`
		}
		_ = json.Unmarshal(args, &a)
		draft.Notes = a.Notes
		// Final validation. Same gates as commit_artifact had post-hoc;
		// per-step builder validation has already caught most issues.
		if draft.ClassifyingField == "" {
			return errResult(fmt.Errorf("commit: classifying_field not set — call set_classifying_field first"))
		}
		if len(draft.Branches) == 0 {
			return errResult(fmt.Errorf("commit: no branches declared — call add_branch first"))
		}
		ruleCount := 0
		for _, b := range draft.Branches {
			if len(b.FieldMap) == 0 {
				return errResult(fmt.Errorf("commit: branch %q has empty field_map — add at least one mapping (typically master.name)", b.Vertical))
			}
			ruleCount += len(b.FieldMap)
		}
		// Smoke-test classification on a real sample. ≥20% coverage gate.
		if covered, sampled, smokeErr := smokeTestClassify(ctx, inbox, tenantID, draft.ClassifyingField, draft.ClassifyRules); smokeErr == nil && sampled >= 5 {
			coverage := float64(covered) / float64(sampled)
			if coverage < 0.20 {
				return errResult(fmt.Errorf("commit: classify smoke-test failed — only %d/%d (%.0f%%) sample rows classified to a known vertical. classifying_field=%q or classify_rules don't match real values. Use sample_values/count_by on %q to inspect, then add or fix rules", covered, sampled, coverage*100, draft.ClassifyingField, draft.ClassifyingField))
			}
		}
		art := draft.Finalize()
		body, _ := json.Marshal(map[string]any{
			"committed":         true,
			"classifying_field": art.ClassifyingField,
			"branches":          art.VerticalNames(),
			"total_rules":       ruleCount,
			"classify_rules":    len(art.ClassifyRules),
		})
		preview, _ := json.Marshal(map[string]any{
			"classifying_field": art.ClassifyingField,
			"branches":          art.VerticalNames(),
			"rules":             ruleCount,
			"classify_rules":    len(art.ClassifyRules),
		})
		return &dispatchResult{Output: string(body), Preview: preview, CommitArtifact: art}

	default:
		return errResult(fmt.Errorf("unknown tool: %s", name))
	}
}

// branchByName returns a pointer to the named branch in the draft, or
// a placeholder (zero VerticalBranch) when not found. Used only for the
// success-path okJSON return — caller already validated branch exists.
func branchByName(d *discoveryDraft, vertical string) *domain.VerticalBranch {
	for i := range d.Branches {
		if strings.EqualFold(d.Branches[i].Vertical, vertical) {
			return &d.Branches[i]
		}
	}
	return &domain.VerticalBranch{}
}

// draftVerticals returns the list of branch verticals for tool result
// summary.
func draftVerticals(d *discoveryDraft) []string {
	out := make([]string, 0, len(d.Branches))
	for _, b := range d.Branches {
		out = append(out, b.Vertical)
	}
	return out
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
