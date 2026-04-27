// Package usecases — Discovery agent tools (M4c).
//
// 8 tools exposed to the Sonnet 4.6 agent. Tool design follows the
// "≤2KB per response, ≤500-token summary, no raw dumps" rule from the
// spec rethink — each Dispatch* method clamps result sizes hard and
// returns concise structured data, never sprawling JSON blobs.
//
// Tools fall into three groups:
//   Discovery (read):     describe_field, sample_records, find_similar_masters, peek_master
//   Proposal (write):     propose_master_template, propose_field_mapping, propose_category_mapping
//   Termination:          commit_artifact
//
// "Write" tools record into an in-memory ArtifactBuilder, not the DB. The
// caller (DiscoveryAgent) finalizes once on commit_artifact.
package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"keepstar-admin/internal/adapters/anthropic"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/ports"
)

// toolMaxResponseBytes is the hard cap on any single tool_result content.
// Set generously above the 2KB target so the agent doesn't see truncated
// JSON; the per-tool clamps stay much tighter in practice.
const toolMaxResponseBytes = 4096

// =============================================================================
// Tool definitions — the JSON schemas the agent sees
// =============================================================================

// AgentTools returns the tool list as Anthropic ToolDef structs. Pure data —
// safe to call repeatedly.
func AgentTools() []anthropic.ToolDef {
	return []anthropic.ToolDef{
		{
			Name: "describe_field",
			Description: "Get statistics for one specific catalog field by path. " +
				"Returns: type, frequency, top values, numeric range, language. " +
				"Use this BEFORE proposing a mapping for any non-obvious field.",
			InputSchema: rawSchema(`{
				"type":"object",
				"properties":{
					"path":{"type":"string","description":"Dotted field path from the meta-report, e.g. 'metafields.custom.scent' or 'variants.[].weight'"}
				},
				"required":["path"]
			}`),
		},
		{
			Name: "sample_records",
			Description: "Fetch up to 5 raw product records from staging matching an optional filter. " +
				"Use sparingly — strings are truncated to 200 chars and only requested fields are returned. " +
				"Good for confirming what real values look like before deciding a mapping.",
			InputSchema: rawSchema(`{
				"type":"object",
				"properties":{
					"fields":{"type":"array","items":{"type":"string"},"description":"Top-level field names to include (e.g. ['title','vendor','metafields'])"},
					"filter_field":{"type":"string","description":"Optional: only return records where this field is non-empty"},
					"limit":{"type":"integer","minimum":1,"maximum":5,"default":3}
				}
			}`),
		},
		{
			Name: "find_similar_masters",
			Description: "Vector-search existing master_products by free text (e.g. 'Vitamin C Serum 30ml'). " +
				"Returns top 5 candidates with similarity scores. Use this to check whether incoming products " +
				"already have masters before proposing new templates. Optional vertical filter narrows scope.",
			InputSchema: rawSchema(`{
				"type":"object",
				"properties":{
					"query":{"type":"string","description":"Free-text query — usually 'brand + product name' or product description"},
					"vertical":{"type":"string","description":"Optional vertical filter: 'cosmetics', 'apparel', etc."},
					"limit":{"type":"integer","minimum":1,"maximum":10,"default":5}
				},
				"required":["query"]
			}`),
		},
		{
			Name: "peek_master",
			Description: "Look at the field shape and variant snippets of a single master_product by ID. " +
				"Use after find_similar_masters to confirm a candidate matches the incoming product structure.",
			InputSchema: rawSchema(`{
				"type":"object",
				"properties":{
					"master_id":{"type":"string","description":"Master product UUID, from find_similar_masters output"}
				},
				"required":["master_id"]
			}`),
		},
		{
			Name: "propose_master_template",
			Description: "Propose a master-template for a NEW vertical (when no existing masters cover the incoming products). " +
				"Defines what fields belong on the master, what fields belong on variants, and what categories to expect. " +
				"Use ONCE per new vertical. Cosmetics is already promoted — don't propose 'cosmetics' here.",
			InputSchema: rawSchema(`{
				"type":"object",
				"properties":{
					"vertical":{"type":"string","description":"Short slug, e.g. 'furniture', 'electronics', 'supplements'"},
					"description":{"type":"string","description":"One-sentence rationale for why this is a separate vertical"},
					"master_fields":{"type":"array","items":{"type":"string"},"description":"Field names for the master (shared across variants), e.g. ['name','brand','description','material']"},
					"variant_fields":{"type":"array","items":{"type":"string"},"description":"Field names that vary per SKU, e.g. ['sku','color','size','weight']"},
					"category_hints":{"type":"array","items":{"type":"string"},"description":"Likely category names for this vertical, e.g. ['Chairs','Tables','Lighting']"}
				},
				"required":["vertical","description","master_fields","variant_fields"]
			}`),
		},
		{
			Name: "propose_field_mapping",
			Description: "Map ONE Shopify field path to a master/listing target. " +
				"Targets: 'master.brand', 'master.description', 'master_variants.weight_g', 'master_variants.gtins[]', " +
				"'master_variants.volume_ml', 'master_cosmetics.scent', 'listing.original_name', 'listing.raw_attributes.<key>', " +
				"'candidate:<key>' (for new attribute candidates), or 'tier3.<key>' (for free-form enrichment data).",
			InputSchema: rawSchema(`{
				"type":"object",
				"properties":{
					"field_path":{"type":"string","description":"Source path from meta-report"},
					"target":{"type":"string","description":"Where this field lands in our schema"},
					"transform":{"type":"string","description":"Optional transform name: 'units.weight', 'units.volume', 'shorten:40', 'lowercase', 'split:comma'"},
					"vertical":{"type":"string","description":"Required when target starts with 'candidate:' — which vertical the candidate belongs to"},
					"type":{"type":"string","description":"For candidates: 'text', 'number', 'enum', 'bool', 'array'"},
					"samples":{"type":"array","items":{"type":"string"},"description":"For candidates: 3-5 example values"}
				},
				"required":["field_path","target"]
			}`),
		},
		{
			Name: "propose_category_mapping",
			Description: "Map a Shopify collection to either a master_category, mark it showcase/promo (no master mapping), " +
				"or leave master_category null for curator review. Use kind='showcase' for 'Best Sellers'/'Featured', " +
				"'promo' for 'Sale'/'Clearance', 'category' for real taxonomy nodes.",
			InputSchema: rawSchema(`{
				"type":"object",
				"properties":{
					"shopify_collection_id":{"type":"string","description":"GID or handle from collection tree"},
					"tenant_label":{"type":"string","description":"What the merchant calls this collection"},
					"kind":{"type":"string","enum":["category","showcase","promo"]},
					"master_category_slug":{"type":"string","description":"Optional master_category slug if you can match it"}
				},
				"required":["shopify_collection_id","tenant_label","kind"]
			}`),
		},
		{
			Name: "propose_brand_mapping",
			Description: "Map a tenant vendor (vendor field on Shopify products) to an action. " +
				"action='link_existing' for vendors that match an existing master_products.brand — set master_brand. " +
				"action='create_new' for new brands the tenant introduces — first matched listing seeds master. " +
				"action='skip' for internal/junk vendors (e.g. 'Keepstar Store' selling gift wraps) — set reason. " +
				"Skip vendors are reviewed by the merge applier alongside JunkRules.",
			InputSchema: rawSchema(`{
				"type":"object",
				"properties":{
					"vendor":{"type":"string","description":"Vendor string as it appears in Shopify product.vendor (case-insensitive on apply)"},
					"action":{"type":"string","enum":["link_existing","create_new","skip"]},
					"master_brand":{"type":"string","description":"Required for action=link_existing: the master_products.brand value to link to"},
					"reason":{"type":"string","description":"Optional human-readable note (especially useful for action=skip)"}
				},
				"required":["vendor","action"]
			}`),
		},
		{
			Name: "propose_junk_rule",
			Description: "Add a rule that tells the merge applier to skip listings without trying to match them. " +
				"rule_type='vendor_blacklist': skip every listing with vendor=value (case-insensitive). " +
				"rule_type='axis_name_pattern': skip listings whose variant.options key contains value (e.g. 'gift wrap'). " +
				"rule_type='require_identifier': call once with value='true'/'false' to set whether listings without SKU AND without GTIN are skipped (default true).",
			InputSchema: rawSchema(`{
				"type":"object",
				"properties":{
					"rule_type":{"type":"string","enum":["vendor_blacklist","axis_name_pattern","require_identifier"]},
					"value":{"type":"string","description":"Vendor / axis pattern / 'true'|'false' depending on rule_type"}
				},
				"required":["rule_type","value"]
			}`),
		},
		{
			Name: "set_match_strategy",
			Description: "Set match cascade order + score thresholds. Call ONCE near the end. " +
				"Defaults are applied if you skip this tool: order=[gtin_exact,vendor_sku_exact,fuzzy_title,embedding], " +
				"auto_link_threshold=0.90, needs_review_threshold=0.50, skip_below=0.30. " +
				"List embedding_disabled_for verticals where master coverage is zero (no point in vector search there).",
			InputSchema: rawSchema(`{
				"type":"object",
				"properties":{
					"order":{"type":"array","items":{"type":"string"},"description":"Strategy names in priority order"},
					"auto_link_threshold":{"type":"number","description":"Score >= this auto-links (default 0.90)"},
					"needs_review_threshold":{"type":"number","description":"Score in [needs_review_threshold, auto_link_threshold) goes to needs_review (default 0.50)"},
					"skip_below":{"type":"number","description":"Score < this is treated as no_match (default 0.30)"},
					"embedding_disabled_for":{"type":"array","items":{"type":"string"},"description":"Verticals to skip embedding strategy for (e.g. ['furniture','footwear'] when master is empty for them)"}
				},
				"required":["order"]
			}`),
		},
		{
			Name: "commit_artifact",
			Description: "Finalize the mapping artifact and end the discovery session. " +
				"Call this ONCE when you've proposed mappings for all the important fields and categories. " +
				"You can leave low-priority fields unmapped — they'll land in listing.raw_attributes by default.",
			InputSchema: rawSchema(`{
				"type":"object",
				"properties":{
					"agent_notes":{"type":"string","description":"Brief summary of decisions made and any caveats for the curator"},
					"match_strategy":{"type":"array","items":{"type":"string"},"description":"Order of cascade strategies, e.g. ['gtin','vendor+sku','vendor+title+axes','embedding']"},
					"variant_strategy":{"type":"string","description":"How variants relate to masters: 'master_with_variants' (default) or 'flat' (rare)"}
				},
				"required":["agent_notes"]
			}`),
		},
	}
}

func rawSchema(s string) json.RawMessage {
	return json.RawMessage(s)
}

// =============================================================================
// Tool dispatch — what the loop calls when the model emits a tool_use block
// =============================================================================

// ToolDispatcher is the bundle of dependencies tool calls fan into. Held
// inside DiscoveryAgent and constructed once per session.
type ToolDispatcher struct {
	report          *domain.MetaReport
	staging         ports.ShopifyStagingPort
	stagingMeta     metadataReader // optional, for shop_references etc.
	variants        ports.MasterVariantsPort
	embedder        ports.EmbeddingPort
	builder         *ArtifactBuilder

	// CommittedArtifact is non-nil after commit_artifact tool fires;
	// DiscoveryAgent reads this to terminate the loop and return the result.
	CommittedArtifact *domain.MappingArtifact
}

// Dispatch runs one tool call and returns the JSON-encoded result string +
// an isError flag. We always return a structured response — even errors
// come back as JSON {"error":"..."} so the agent can read and self-correct
// rather than getting Anthropic-level tool_use errors.
func (d *ToolDispatcher) Dispatch(ctx context.Context, name string, input json.RawMessage) (string, bool) {
	switch name {
	case "describe_field":
		return d.describeField(input)
	case "sample_records":
		return d.sampleRecords(ctx, input)
	case "find_similar_masters":
		return d.findSimilarMasters(ctx, input)
	case "peek_master":
		return d.peekMaster(ctx, input)
	case "propose_master_template":
		return d.proposeMasterTemplate(input)
	case "propose_field_mapping":
		return d.proposeFieldMapping(input)
	case "propose_category_mapping":
		return d.proposeCategoryMapping(input)
	case "propose_brand_mapping":
		return d.proposeBrandMapping(input)
	case "propose_junk_rule":
		return d.proposeJunkRule(input)
	case "set_match_strategy":
		return d.setMatchStrategy(input)
	case "commit_artifact":
		return d.commitArtifact(input)
	default:
		return jsonResponse(map[string]string{"error": "unknown tool: " + name}), true
	}
}

// --- describe_field --------------------------------------------------------

type describeFieldArgs struct {
	Path string `json:"path"`
}

func (d *ToolDispatcher) describeField(input json.RawMessage) (string, bool) {
	var args describeFieldArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return jsonResponse(map[string]string{"error": err.Error()}), true
	}
	if args.Path == "" {
		return jsonResponse(map[string]string{"error": "path is required"}), true
	}
	for _, fs := range d.report.Fields {
		if fs.Path == args.Path {
			return jsonResponse(fs), false
		}
	}
	return jsonResponse(map[string]string{
		"error": "field not found in meta-report — list_fields hint: only fields present in ≥1 product appear here",
		"path":  args.Path,
	}), true
}

// --- sample_records --------------------------------------------------------

type sampleRecordsArgs struct {
	Fields      []string `json:"fields,omitempty"`
	FilterField string   `json:"filter_field,omitempty"`
	Limit       int      `json:"limit,omitempty"`
}

func (d *ToolDispatcher) sampleRecords(ctx context.Context, input json.RawMessage) (string, bool) {
	var args sampleRecordsArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return jsonResponse(map[string]string{"error": err.Error()}), true
	}
	if args.Limit <= 0 || args.Limit > 5 {
		args.Limit = 3
	}

	type sampleRow map[string]json.RawMessage
	samples := make([]sampleRow, 0, args.Limit)

	iterErr := d.staging.IterateProducts(ctx, d.report.TenantID, func(sourceID string, payload json.RawMessage, _ time.Time) error {
		if len(samples) >= args.Limit {
			return errStopIteration
		}
		var product map[string]json.RawMessage
		if err := json.Unmarshal(payload, &product); err != nil {
			return nil
		}
		if args.FilterField != "" {
			if v, ok := product[args.FilterField]; !ok || isEmptyJSON(v) {
				return nil
			}
		}
		row := make(sampleRow)
		row["__source_id"] = json.RawMessage(`"` + sourceID + `"`)
		if len(args.Fields) == 0 {
			// Sensible default — enough to identify the product without
			// dragging in metafield blobs.
			for _, k := range []string{"title", "vendor", "productType", "handle"} {
				if v, ok := product[k]; ok {
					row[k] = truncateJSONString(v, 200)
				}
			}
		} else {
			for _, k := range args.Fields {
				if v, ok := product[k]; ok {
					row[k] = truncateJSONString(v, 200)
				}
			}
		}
		samples = append(samples, row)
		return nil
	})
	if iterErr != nil && !errors.Is(iterErr, errStopIteration) {
		return jsonResponse(map[string]string{"error": iterErr.Error()}), true
	}

	return jsonResponse(map[string]any{
		"count":   len(samples),
		"samples": samples,
	}), false
}

// --- find_similar_masters --------------------------------------------------

type findSimilarMastersArgs struct {
	Query    string `json:"query"`
	Vertical string `json:"vertical,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

func (d *ToolDispatcher) findSimilarMasters(ctx context.Context, input json.RawMessage) (string, bool) {
	var args findSimilarMastersArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return jsonResponse(map[string]string{"error": err.Error()}), true
	}
	if args.Query == "" {
		return jsonResponse(map[string]string{"error": "query is required"}), true
	}
	if args.Limit <= 0 || args.Limit > 10 {
		args.Limit = 5
	}
	// pg_trgm fuzzy match over name + brand. No external dependency, no
	// API key. The harvester (M4d) layers semantic embedding search on top
	// for the long-tail cases — discovery doesn't need that depth.
	hits, err := d.variants.FindMasterProductsByName(ctx, args.Query, args.Vertical, args.Limit)
	if err != nil {
		return jsonResponse(map[string]string{"error": err.Error()}), true
	}
	// Trim variant snippets — find_similar_masters returns just headlines;
	// agent can call peek_master for one specific candidate to drill in.
	for i := range hits {
		hits[i].Variants = nil
	}
	return jsonResponse(map[string]any{
		"query":   args.Query,
		"matches": hits,
	}), false
}

// --- peek_master -----------------------------------------------------------

type peekMasterArgs struct {
	MasterID string `json:"master_id"`
}

func (d *ToolDispatcher) peekMaster(ctx context.Context, input json.RawMessage) (string, bool) {
	var args peekMasterArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return jsonResponse(map[string]string{"error": err.Error()}), true
	}
	if args.MasterID == "" {
		return jsonResponse(map[string]string{"error": "master_id is required"}), true
	}
	summary, err := d.variants.GetMasterProductSummary(ctx, args.MasterID)
	if err != nil {
		return jsonResponse(map[string]string{"error": err.Error()}), true
	}
	if summary == nil {
		return jsonResponse(map[string]string{"error": "master not found", "master_id": args.MasterID}), true
	}
	return jsonResponse(summary), false
}

// --- propose_master_template ----------------------------------------------

type proposeMasterTemplateArgs struct {
	Vertical       string   `json:"vertical"`
	Description    string   `json:"description"`
	MasterFields   []string `json:"master_fields"`
	VariantFields  []string `json:"variant_fields"`
	CategoryHints  []string `json:"category_hints,omitempty"`
}

func (d *ToolDispatcher) proposeMasterTemplate(input json.RawMessage) (string, bool) {
	var args proposeMasterTemplateArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return jsonResponse(map[string]string{"error": err.Error()}), true
	}
	if args.Vertical == "" || args.Description == "" || len(args.MasterFields) == 0 {
		return jsonResponse(map[string]string{"error": "vertical, description, master_fields are required"}), true
	}
	if strings.EqualFold(args.Vertical, "cosmetics") {
		return jsonResponse(map[string]string{"error": "cosmetics is already promoted — don't propose it as a new vertical"}), true
	}
	d.builder.AddTemplate(domain.MasterTemplateProposal{
		Vertical:      args.Vertical,
		Description:   args.Description,
		MasterFields:  args.MasterFields,
		VariantFields: args.VariantFields,
		CategoryHints: args.CategoryHints,
	})
	return jsonResponse(map[string]string{"status": "recorded", "vertical": args.Vertical}), false
}

// --- propose_field_mapping -------------------------------------------------

type proposeFieldMappingArgs struct {
	FieldPath string   `json:"field_path"`
	Target    string   `json:"target"`
	Transform string   `json:"transform,omitempty"`
	Vertical  string   `json:"vertical,omitempty"`
	Type      string   `json:"type,omitempty"`
	Samples   []string `json:"samples,omitempty"`
}

func (d *ToolDispatcher) proposeFieldMapping(input json.RawMessage) (string, bool) {
	var args proposeFieldMappingArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return jsonResponse(map[string]string{"error": err.Error()}), true
	}
	if args.FieldPath == "" || args.Target == "" {
		return jsonResponse(map[string]string{"error": "field_path and target are required"}), true
	}
	// candidate:<key> requires vertical so the candidate row knows where it belongs.
	if strings.HasPrefix(args.Target, "candidate:") && args.Vertical == "" {
		return jsonResponse(map[string]string{"error": "vertical is required for candidate: targets"}), true
	}
	d.builder.SetFieldMapping(args.FieldPath, domain.FieldMappingTarget{
		Target:    args.Target,
		Transform: args.Transform,
		Type:      args.Type,
		Samples:   args.Samples,
	})
	return jsonResponse(map[string]string{"status": "recorded", "field": args.FieldPath, "target": args.Target}), false
}

// --- propose_category_mapping ---------------------------------------------

type proposeCategoryMappingArgs struct {
	ShopifyCollectionID string `json:"shopify_collection_id"`
	TenantLabel         string `json:"tenant_label"`
	Kind                string `json:"kind"`
	MasterCategorySlug  string `json:"master_category_slug,omitempty"`
}

func (d *ToolDispatcher) proposeCategoryMapping(input json.RawMessage) (string, bool) {
	var args proposeCategoryMappingArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return jsonResponse(map[string]string{"error": err.Error()}), true
	}
	if args.ShopifyCollectionID == "" || args.TenantLabel == "" || args.Kind == "" {
		return jsonResponse(map[string]string{"error": "shopify_collection_id, tenant_label, kind required"}), true
	}
	if args.Kind != "category" && args.Kind != "showcase" && args.Kind != "promo" {
		return jsonResponse(map[string]string{"error": "kind must be category|showcase|promo"}), true
	}
	target := ""
	if args.MasterCategorySlug != "" {
		target = "master_category:" + args.MasterCategorySlug
	}
	d.builder.SetCategoryMapping(args.ShopifyCollectionID, domain.CategoryMappingTarget{
		Target:      target,
		TenantLabel: args.TenantLabel,
		Kind:        args.Kind,
	})
	return jsonResponse(map[string]string{"status": "recorded", "collection": args.ShopifyCollectionID}), false
}

// --- propose_brand_mapping -------------------------------------------------

type proposeBrandMappingArgs struct {
	Vendor      string `json:"vendor"`
	Action      string `json:"action"`
	MasterBrand string `json:"master_brand,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

func (d *ToolDispatcher) proposeBrandMapping(input json.RawMessage) (string, bool) {
	var args proposeBrandMappingArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return jsonResponse(map[string]string{"error": err.Error()}), true
	}
	if args.Vendor == "" || args.Action == "" {
		return jsonResponse(map[string]string{"error": "vendor and action are required"}), true
	}
	switch args.Action {
	case "link_existing":
		if args.MasterBrand == "" {
			return jsonResponse(map[string]string{"error": "master_brand is required for action=link_existing"}), true
		}
	case "create_new", "skip":
		// ok
	default:
		return jsonResponse(map[string]string{"error": "action must be link_existing|create_new|skip"}), true
	}
	d.builder.SetBrandMapping(args.Vendor, domain.BrandMappingTarget{
		Action:      args.Action,
		MasterBrand: args.MasterBrand,
		Reason:      args.Reason,
	})
	return jsonResponse(map[string]string{
		"status": "recorded",
		"vendor": args.Vendor,
		"action": args.Action,
	}), false
}

// --- propose_junk_rule -----------------------------------------------------

type proposeJunkRuleArgs struct {
	RuleType string `json:"rule_type"`
	Value    string `json:"value"`
}

func (d *ToolDispatcher) proposeJunkRule(input json.RawMessage) (string, bool) {
	var args proposeJunkRuleArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return jsonResponse(map[string]string{"error": err.Error()}), true
	}
	if args.RuleType == "" || args.Value == "" {
		return jsonResponse(map[string]string{"error": "rule_type and value are required"}), true
	}
	switch args.RuleType {
	case "vendor_blacklist":
		d.builder.AddJunkVendor(args.Value)
	case "axis_name_pattern":
		d.builder.AddJunkAxisPattern(args.Value)
	case "require_identifier":
		v := strings.ToLower(strings.TrimSpace(args.Value))
		d.builder.SetRequireIdentifier(v == "true" || v == "yes" || v == "1")
	default:
		return jsonResponse(map[string]string{"error": "rule_type must be vendor_blacklist|axis_name_pattern|require_identifier"}), true
	}
	return jsonResponse(map[string]string{"status": "recorded", "rule_type": args.RuleType, "value": args.Value}), false
}

// --- set_match_strategy ----------------------------------------------------

type setMatchStrategyArgs struct {
	Order                []string `json:"order"`
	AutoLinkThreshold    float64  `json:"auto_link_threshold"`
	NeedsReviewThreshold float64  `json:"needs_review_threshold"`
	SkipBelow            float64  `json:"skip_below"`
	EmbeddingDisabledFor []string `json:"embedding_disabled_for,omitempty"`
}

func (d *ToolDispatcher) setMatchStrategy(input json.RawMessage) (string, bool) {
	var args setMatchStrategyArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return jsonResponse(map[string]string{"error": err.Error()}), true
	}
	if len(args.Order) == 0 {
		return jsonResponse(map[string]string{"error": "order is required (non-empty)"}), true
	}
	// Defaults if agent omitted any threshold (treat 0 as "use default").
	if args.AutoLinkThreshold == 0 {
		args.AutoLinkThreshold = 0.90
	}
	if args.NeedsReviewThreshold == 0 {
		args.NeedsReviewThreshold = 0.50
	}
	if args.SkipBelow == 0 {
		args.SkipBelow = 0.30
	}
	if args.NeedsReviewThreshold >= args.AutoLinkThreshold {
		return jsonResponse(map[string]string{"error": "needs_review_threshold must be < auto_link_threshold"}), true
	}
	if args.SkipBelow >= args.NeedsReviewThreshold {
		return jsonResponse(map[string]string{"error": "skip_below must be < needs_review_threshold"}), true
	}
	d.builder.SetMatchStrategyConfig(domain.MatchStrategyConfig{
		Order:                args.Order,
		AutoLinkThreshold:    args.AutoLinkThreshold,
		NeedsReviewThreshold: args.NeedsReviewThreshold,
		SkipBelow:            args.SkipBelow,
		EmbeddingDisabledFor: args.EmbeddingDisabledFor,
	})
	return jsonResponse(map[string]any{"status": "recorded", "order": args.Order}), false
}

// --- commit_artifact -------------------------------------------------------

type commitArtifactArgs struct {
	AgentNotes      string   `json:"agent_notes"`
	MatchStrategy   []string `json:"match_strategy,omitempty"`
	VariantStrategy string   `json:"variant_strategy,omitempty"`
}

func (d *ToolDispatcher) commitArtifact(input json.RawMessage) (string, bool) {
	var args commitArtifactArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return jsonResponse(map[string]string{"error": err.Error()}), true
	}
	if args.AgentNotes == "" {
		return jsonResponse(map[string]string{"error": "agent_notes is required — even one line"}), true
	}
	if len(args.MatchStrategy) == 0 {
		args.MatchStrategy = []string{"gtin", "vendor+sku", "vendor+title+axes", "embedding"}
	}
	if args.VariantStrategy == "" {
		args.VariantStrategy = "master_with_variants"
	}
	d.CommittedArtifact = d.builder.Build(args.AgentNotes, args.MatchStrategy, args.VariantStrategy)
	return jsonResponse(map[string]any{
		"status":         "committed",
		"field_mappings": len(d.CommittedArtifact.FieldMapping),
		"categories":     len(d.CommittedArtifact.CategoryMapping),
	}), false
}

// =============================================================================
// Helpers
// =============================================================================

// jsonResponse marshals any value into the string form required by Anthropic
// tool_result content blocks. Caps total length at toolMaxResponseBytes —
// over-long responses are truncated with a note (better than blowing the
// agent's context budget).
func jsonResponse(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		b = []byte(`{"error":"marshal failed"}`)
	}
	if len(b) > toolMaxResponseBytes {
		b = append(b[:toolMaxResponseBytes-50], []byte(`","_truncated":true}`)...)
	}
	return string(b)
}

// truncateJSONString preserves valid JSON when shrinking string values.
// Non-string raw messages pass through unchanged.
func truncateJSONString(raw json.RawMessage, max int) json.RawMessage {
	r := strings.TrimSpace(string(raw))
	if len(r) < 2 || r[0] != '"' {
		return raw
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return raw
	}
	if len(s) <= max {
		return raw
	}
	out, _ := json.Marshal(s[:max] + "…")
	return out
}

func isEmptyJSON(raw json.RawMessage) bool {
	r := strings.TrimSpace(string(raw))
	if r == "" || r == "null" || r == `""` || r == "[]" || r == "{}" || r == "0" || r == "false" {
		return true
	}
	return false
}

// errStopIteration is the sentinel IterateProducts callbacks return to bail
// out early once we have enough samples. Other errors are real; this one
// is swallowed by the dispatch layer.
var errStopIteration = errors.New("stop iteration")
