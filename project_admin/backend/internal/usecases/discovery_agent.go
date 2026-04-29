// Package usecases — Discovery agent (M4c spec §4.1 step 6, with rethink).
//
// Multi-turn tool-use loop with Sonnet 4.6 that produces the per-tenant
// mapping_artifact. Replaces the original "one-shot 5 tool calls" plan
// because real catalogs need exploration before mapping (especially when
// the agent has to propose master templates for new verticals).
//
// Flow:
//   1. Caller (typically a handler or DI'd job) constructs DiscoveryAgent
//      with the dependencies the tools need (staging, variants, embedder).
//   2. Discover() builds the system prompt with the meta-report + Tier 1
//      auto-mapping hints, then drives the Sonnet 4.6 conversation:
//        - send messages → receive content blocks (text + tool_use)
//        - dispatch each tool_use → build user message with tool_result blocks
//        - repeat until commit_artifact, end_turn, or budget exceeded
//   3. Returns the committed MappingArtifact (or nil + status=needs_human_review).
//
// Budget: 30 turns / 50K total input+output tokens / 8 minutes wallclock.
// Anything past those bounds → status='needs_human_review' so the curator
// (M11) can either approve a partial artifact or run discovery manually.
package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"keepstar-admin/internal/adapters/anthropic"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

const (
	discoveryMaxTurns = 30
	// discoveryMaxTokens counts NON-CACHED input + output. Cached reads cost
	// ~10% and don't burn this budget — the bulk of input on every turn is
	// the system prompt + tools, both of which we cache. Bumped from 50K
	// (which a real run hit on turn 6 before caching was wired).
	discoveryMaxTokens       = 150_000
	discoveryWallclockBudget = 8 * time.Minute
)

// AgentSender is the minimum LLM surface DiscoveryAgent needs. Real usage
// passes an *anthropic.AgentClient; tests pass a scripted fake.
type AgentSender interface {
	Send(ctx context.Context, req anthropic.MessagesRequest) (*anthropic.MessagesResponse, error)
}

// DiscoveryAgent owns the loop. Stateless across runs — Discover() is safe
// to call concurrently for different tenants (each gets its own dispatcher
// + builder).
type DiscoveryAgent struct {
	llm      AgentSender
	staging  ports.ShopifyStagingPort
	variants ports.MasterVariantsPort
	embedder ports.EmbeddingPort
	log      *logger.Logger
}

func NewDiscoveryAgent(
	llm AgentSender,
	staging ports.ShopifyStagingPort,
	variants ports.MasterVariantsPort,
	embedder ports.EmbeddingPort,
	log *logger.Logger,
) *DiscoveryAgent {
	return &DiscoveryAgent{
		llm:      llm,
		staging:  staging,
		variants: variants,
		embedder: embedder,
		log:      log,
	}
}

// DiscoveryResult carries everything the caller needs to persist the
// outcome and surface progress to UI / logs.
//
// PartialBuilder is non-nil whenever the loop ran at least one tool call,
// even if the agent never reached commit_artifact. The orchestrator uses
// it to persist whatever the agent did manage to propose as a
// needs_human_review artifact — otherwise we'd lose all the agent's work
// on a budget overrun.
type DiscoveryResult struct {
	Artifact          *domain.MappingArtifact // nil when status != active
	PartialBuilder    *ArtifactBuilder        // non-nil whenever any propose_* fired
	Status            domain.MappingArtifactStatus
	StopReason        string
	TurnsUsed         int
	InputTokens       int // total (cached + uncached + output is separate)
	OutputTokens      int
	CachedReadTokens  int // subset of InputTokens — billed at ~10% of normal
	CacheWriteTokens  int // one-time cache creation cost
	Duration          time.Duration
	Transcript        []TranscriptEntry
}

// TranscriptEntry is one entry in the human-readable transcript. Tool calls
// are summarized (name + key arg, not full input) to keep the log small.
type TranscriptEntry struct {
	Turn      int    `json:"turn"`
	Kind      string `json:"kind"` // "agent_text" | "tool_call" | "tool_result"
	Detail    string `json:"detail"`
	IsError   bool   `json:"isError,omitempty"`
}

// Discover runs the full loop for one tenant. Returns the result whether
// it ended on commit_artifact (status=active), budget cap (status=
// needs_human_review), or any other terminal condition.
//
// report is the MetaReport from MetadataHarvest.Run (4b). tier1 is the
// AutoMapTier1Result that pre-fills FieldMapping with obvious entries.
func (da *DiscoveryAgent) Discover(ctx context.Context, tenantID string, report *domain.MetaReport, tier1 AutoMapTier1Result) (*DiscoveryResult, error) {
	if report == nil {
		return nil, errors.New("discovery: meta-report is required")
	}
	if report.TenantID == "" {
		report.TenantID = tenantID
	}
	if tenantID != "" && report.TenantID != tenantID {
		return nil, fmt.Errorf("discovery: tenant mismatch (report=%s arg=%s)", report.TenantID, tenantID)
	}

	// Wallclock cap is layered on top of the caller's ctx — we never run
	// past the spec budget even if the caller is happy to wait longer.
	loopCtx, cancel := context.WithTimeout(ctx, discoveryWallclockBudget)
	defer cancel()

	builder := NewArtifactBuilder(tier1)
	dispatcher := &ToolDispatcher{
		report:      report,
		staging:     da.staging,
		stagingMeta: stagingAsMetaReader(da.staging),
		variants:    da.variants,
		embedder:    da.embedder,
		builder:     builder,
	}

	// Master overview is fetched once at session start and inlined into the
	// system prompt. Catalog completion 2026-04-28: gives the agent a "soup"
	// view of master_products / master_categories / master_field_definitions
	// before it starts proposing brand_mapping / category_mapping. Failure to
	// fetch is non-fatal — agent falls back to find_similar_masters tool calls.
	var overview *ports.MasterOverview
	if da.variants != nil {
		o, err := da.variants.GetMasterOverview(ctx)
		if err != nil {
			da.log.Warn("discovery_master_overview_failed", "tenant_id", tenantID, "error", err.Error())
		} else {
			overview = o
		}
	}

	// Cache the system prompt + tool list. Both are static across all turns
	// of this run — caching makes the per-turn cost ~10× smaller after the
	// first turn (which writes the cache). The marker on the LAST tool tells
	// Anthropic to cache everything up to and including it.
	systemBlocks := []anthropic.SystemBlock{{
		Type:         "text",
		Text:         buildDiscoverySystemPrompt(report, tier1, overview),
		CacheControl: &anthropic.CacheControl{Type: "ephemeral"},
	}}
	tools := AgentTools()
	if len(tools) > 0 {
		tools[len(tools)-1].CacheControl = &anthropic.CacheControl{Type: "ephemeral"}
	}

	messages := []anthropic.Message{{
		Role: "user",
		Content: []anthropic.ContentBlock{{
			Type: "text",
			Text: "Begin discovery. Inspect the meta-report in the system message, " +
				"explore as needed via tools, then commit_artifact when ready. " +
				"Avoid asking me questions — I won't reply, just call tools.",
		}},
	}}

	res := &DiscoveryResult{
		Status:         domain.MappingArtifactStatusNeedsHumanReview,
		PartialBuilder: builder, // exposed so caller can persist partial work
	}
	start := time.Now()
	defer func() { res.Duration = time.Since(start) }()

	for turn := 1; turn <= discoveryMaxTurns; turn++ {
		res.TurnsUsed = turn

		// Budget pre-check — count only the un-cached tokens against the
		// budget. Cached reads are ~10× cheaper and would otherwise inflate
		// the count without representing real cost.
		uncached := (res.InputTokens - res.CachedReadTokens) + res.OutputTokens
		if uncached >= discoveryMaxTokens {
			res.StopReason = "token_budget_exceeded"
			da.log.Info("discovery_budget_tokens", "tenant", tenantID,
				"uncached", uncached, "cached_read", res.CachedReadTokens)
			return res, nil
		}
		if loopCtx.Err() != nil {
			res.StopReason = "wallclock_exceeded"
			return res, nil
		}

		resp, err := da.llm.Send(loopCtx, anthropic.MessagesRequest{
			SystemBlocks: systemBlocks,
			Tools:        tools,
			Messages:     messages,
		})
		if err != nil {
			res.StopReason = "llm_error: " + err.Error()
			return res, fmt.Errorf("discovery turn %d: %w", turn, err)
		}
		res.InputTokens += resp.Usage.InputTokens + resp.Usage.CacheCreationInputTokens + resp.Usage.CacheReadInputTokens
		res.OutputTokens += resp.Usage.OutputTokens
		res.CachedReadTokens += resp.Usage.CacheReadInputTokens
		res.CacheWriteTokens += resp.Usage.CacheCreationInputTokens

		// Append the assistant turn to the transcript verbatim — we'll
		// need it next turn whether or not there are tool calls.
		messages = append(messages, anthropic.Message{Role: "assistant", Content: resp.Content})

		// Walk the response: any text → transcript; any tool_use → dispatch
		// + accumulate tool_result blocks for the next user message.
		var toolResults []anthropic.ContentBlock
		for _, block := range resp.Content {
			switch block.Type {
			case "text":
				if t := strings.TrimSpace(block.Text); t != "" {
					res.Transcript = append(res.Transcript, TranscriptEntry{
						Turn:   turn,
						Kind:   "agent_text",
						Detail: truncateForTranscript(t, 500),
					})
				}
			case "tool_use":
				summary := summarizeToolCall(block.Name, block.Input)
				res.Transcript = append(res.Transcript, TranscriptEntry{
					Turn:   turn,
					Kind:   "tool_call",
					Detail: block.Name + ": " + summary,
				})
				resultStr, isErr := dispatcher.Dispatch(loopCtx, block.Name, block.Input)
				toolResults = append(toolResults, anthropic.ContentBlock{
					Type:      "tool_result",
					ToolUseID: block.ID,
					Content:   resultStr,
					IsError:   isErr,
				})
				res.Transcript = append(res.Transcript, TranscriptEntry{
					Turn:    turn,
					Kind:    "tool_result",
					Detail:  truncateForTranscript(resultStr, 200),
					IsError: isErr,
				})
			}
		}

		// commit_artifact terminates the loop regardless of stop_reason.
		if dispatcher.CommittedArtifact != nil {
			res.Artifact = dispatcher.CommittedArtifact
			res.Artifact.Status = domain.MappingArtifactStatusActive
			res.Artifact.ValidatedAt = time.Now().UTC()
			res.Status = domain.MappingArtifactStatusActive
			res.StopReason = "commit_artifact"
			return res, nil
		}

		// stop_reason == "end_turn" without commit means the agent stopped
		// without finalizing. We don't auto-commit — needs_human_review.
		if resp.StopReason == "end_turn" {
			res.StopReason = "end_turn_without_commit"
			return res, nil
		}
		if resp.StopReason == "max_tokens" {
			res.StopReason = "model_max_tokens"
			return res, nil
		}

		// stop_reason should be "tool_use" → next turn supplies tool_result
		// content as a user message.
		if len(toolResults) == 0 {
			// Defensive: model emitted neither text nor tool_use. Treat as
			// terminal — otherwise we'd loop forever.
			res.StopReason = "empty_assistant_turn"
			return res, nil
		}
		messages = append(messages, anthropic.Message{Role: "user", Content: toolResults})
	}

	res.StopReason = "max_turns_exceeded"
	return res, nil
}

// =============================================================================
// Artifact builder — accumulates proposals from tool calls
// =============================================================================

// ArtifactBuilder is the in-memory accumulator the propose_* tools write
// into and commit_artifact finalizes. Concurrency-safe (in case future
// agent versions emit parallel tool calls — Anthropic doesn't yet, but
// the cost of a mutex here is negligible).
type ArtifactBuilder struct {
	mu              sync.Mutex
	fieldMapping    map[string]domain.FieldMappingTarget
	categoryMapping map[string]domain.CategoryMappingTarget
	templates       []domain.MasterTemplateProposal

	// Merge-side accumulators (catalog completion 2026-04-28). Lowercase-keyed
	// brand map so the applier matches case-insensitively against vendor.
	brandMapping        map[string]domain.BrandMappingTarget
	junkRules           *domain.JunkRules
	matchStrategyConfig *domain.MatchStrategyConfig
}

func NewArtifactBuilder(tier1 AutoMapTier1Result) *ArtifactBuilder {
	b := &ArtifactBuilder{
		fieldMapping:    make(map[string]domain.FieldMappingTarget),
		categoryMapping: make(map[string]domain.CategoryMappingTarget),
		brandMapping:    make(map[string]domain.BrandMappingTarget),
	}
	// Pre-populate with Tier 1 auto-mapping so the agent's proposals only
	// need to fill in what's not obvious.
	for k, v := range tier1.FieldMapping {
		b.fieldMapping[k] = v
	}
	for k, v := range tier1.CategoryMapping {
		b.categoryMapping[k] = v
	}
	return b
}

func (b *ArtifactBuilder) SetFieldMapping(path string, target domain.FieldMappingTarget) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fieldMapping[path] = target
}

func (b *ArtifactBuilder) SetCategoryMapping(id string, target domain.CategoryMappingTarget) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.categoryMapping[id] = target
}

func (b *ArtifactBuilder) AddTemplate(t domain.MasterTemplateProposal) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.templates = append(b.templates, t)
}

// SetBrandMapping records how a tenant vendor maps. Vendor key is normalized
// to lowercase + trimmed so the applier can match case-insensitively.
func (b *ArtifactBuilder) SetBrandMapping(vendor string, target domain.BrandMappingTarget) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.brandMapping[strings.ToLower(strings.TrimSpace(vendor))] = target
}

// AddJunkVendor / AddJunkAxisPattern / SetRequireIdentifier are convenience
// setters used by the propose_junk_rule tool. The first call lazily initializes
// the JunkRules struct.
func (b *ArtifactBuilder) AddJunkVendor(vendor string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.junkRules == nil {
		b.junkRules = &domain.JunkRules{RequireIdentifier: true}
	}
	v := strings.ToLower(strings.TrimSpace(vendor))
	for _, existing := range b.junkRules.VendorBlacklist {
		if existing == v {
			return
		}
	}
	b.junkRules.VendorBlacklist = append(b.junkRules.VendorBlacklist, v)
}

func (b *ArtifactBuilder) AddJunkAxisPattern(pattern string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.junkRules == nil {
		b.junkRules = &domain.JunkRules{RequireIdentifier: true}
	}
	p := strings.ToLower(strings.TrimSpace(pattern))
	for _, existing := range b.junkRules.AxisNamePatterns {
		if existing == p {
			return
		}
	}
	b.junkRules.AxisNamePatterns = append(b.junkRules.AxisNamePatterns, p)
}

func (b *ArtifactBuilder) SetRequireIdentifier(v bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.junkRules == nil {
		b.junkRules = &domain.JunkRules{}
	}
	b.junkRules.RequireIdentifier = v
}

// SetMatchStrategyConfig replaces the current strategy config. Called by the
// set_match_strategy tool. If never called, Build() falls back to defaults.
func (b *ArtifactBuilder) SetMatchStrategyConfig(cfg domain.MatchStrategyConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()
	c := cfg
	b.matchStrategyConfig = &c
}

// Build snapshots the current state into a MappingArtifact. ValidatedAt /
// Status are set by the caller (DiscoveryAgent or the validation step).
func (b *ArtifactBuilder) Build(notes string, matchStrategy []string, variantStrategy string) *domain.MappingArtifact {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Copy maps so post-Build mutations don't affect the snapshot.
	fm := make(map[string]domain.FieldMappingTarget, len(b.fieldMapping))
	for k, v := range b.fieldMapping {
		fm[k] = v
	}
	cm := make(map[string]domain.CategoryMappingTarget, len(b.categoryMapping))
	for k, v := range b.categoryMapping {
		cm[k] = v
	}
	tpls := make([]domain.MasterTemplateProposal, len(b.templates))
	copy(tpls, b.templates)

	var bm map[string]domain.BrandMappingTarget
	if len(b.brandMapping) > 0 {
		bm = make(map[string]domain.BrandMappingTarget, len(b.brandMapping))
		for k, v := range b.brandMapping {
			bm[k] = v
		}
	}
	var jr *domain.JunkRules
	if b.junkRules != nil {
		j := *b.junkRules
		// Defensive copies of slices.
		if len(j.VendorBlacklist) > 0 {
			j.VendorBlacklist = append([]string(nil), j.VendorBlacklist...)
		}
		if len(j.AxisNamePatterns) > 0 {
			j.AxisNamePatterns = append([]string(nil), j.AxisNamePatterns...)
		}
		jr = &j
	}
	var msc *domain.MatchStrategyConfig
	if b.matchStrategyConfig != nil {
		c := *b.matchStrategyConfig
		if len(c.Order) > 0 {
			c.Order = append([]string(nil), c.Order...)
		}
		if len(c.EmbeddingDisabledFor) > 0 {
			c.EmbeddingDisabledFor = append([]string(nil), c.EmbeddingDisabledFor...)
		}
		msc = &c
	}

	return &domain.MappingArtifact{
		Version:             1,
		FieldMapping:        fm,
		CategoryMapping:     cm,
		MasterTemplates:     tpls,
		MatchStrategy:       matchStrategy,
		VariantStrategy:     variantStrategy,
		AgentNotes:          notes,
		BrandMapping:        bm,
		JunkRules:           jr,
		MatchStrategyConfig: msc,
	}
}

// =============================================================================
// System prompt construction
// =============================================================================

func buildDiscoverySystemPrompt(report *domain.MetaReport, tier1 AutoMapTier1Result, overview *ports.MasterOverview) string {
	var sb strings.Builder
	sb.WriteString(`You are the Catalog Discovery & Merge Agent for the Keepstar admin platform.
Your single job is to produce a MAPPING ARTIFACT that fully describes how this
tenant's Shopify catalog converts into our master/listing schema AND how each
listing should be merged with master products. The artifact you commit drives a
deterministic Go applier — there is no per-listing LLM call after this run, so
your rules need to be complete enough to handle every listing in the tenant.

You have 30 tool-call turns and an 8-minute wallclock budget. Use them well —
Sonnet costs real money, but a wrong artifact costs more.

# How our schema works (read carefully)

- master_products = one canonical row per real product, shared across tenants.
  Fields: name, brand, description, image_url, vertical, tier2 (JSONB),
  tier3 (JSONB).
- master_variants = one row per SKU. Fields: sku, gtins (TEXT[]), weight_g,
  volume_ml, length_mm/width_mm/height_mm, color, size, material, axes (JSONB).
- tier2 (JSONB on master_products) = vertical-specific structured attributes.
  Examples: cosmetics → skin_type, concern, ingredients, scent, spf.
  Furniture → material, dimensions, assembly_required.
  Footwear → upper_material, heel_height, closure.
  These are real keys we read in the chat widget — propose them precisely.
- catalog.products (listing) = per-tenant overrides. Fields: master_variant_id,
  display_name (short), original_name (full), price, stock_quantity,
  raw_attributes (JSONB), media (JSONB).

# What you produce

Mapping targets you can use in propose_field_mapping(target=...):
  master.brand, master.description, master.image_url, master.name
  master_variants.sku, master_variants.gtins[], master_variants.weight_g,
    master_variants.volume_ml, master_variants.color, master_variants.size,
    master_variants.material, master_variants.image_url
  master.tier2.<key>        — vertical-specific structured field (skin_type,
                              material, ingredients, etc.) — see registered
                              tier2 fields below; propose new keys freely
  listing.original_name, listing.display_name, listing.price,
    listing.stock_quantity, listing.media[],
    listing.raw_attributes.<any_key>
  candidate:<key>           — attribute candidate for promotion review (set vertical)
  tier3.<any_key>           — free-form bag for non-promoted enrichment data

Transforms supported in propose_field_mapping(transform=...):
  units.weight              — parse "2.5 kg" → grams
  units.volume              — parse "236ml" → ml
  shorten:N                 — truncate to N characters
  lowercase / split:comma   — basic normalization

Merge-side outputs (these are NEW, the artifact's other half):

  propose_brand_mapping(vendor, action, master_brand?, reason?):
    For each distinct vendor in the tenant's catalog, decide:
      action="link_existing" + master_brand="X" — vendor matches a brand already in master.
                                                  Look at the master overview below for what's there.
      action="create_new"                       — vendor introduces a new brand. The applier
                                                  seeds master.brand on the first matched listing.
      action="skip" + reason                    — internal/junk vendor (e.g. "Keepstar Store"
                                                  if it only sells gift wraps). Skipped at apply.

  propose_junk_rule(rule_type, value):
    rule_type="vendor_blacklist" + value="vendor name" — skip every listing under that vendor.
    rule_type="axis_name_pattern" + value="gift wrap" — skip listings whose variant.options
                                                         have a key matching this substring.
    rule_type="require_identifier" + value="true"|"false" — controls whether listings without
                                                            both SKU and GTIN are skipped (default true).

  set_match_strategy(order, auto_link_threshold, needs_review_threshold, skip_below, embedding_disabled_for):
    Tune the match-cascade for this tenant. Defaults are sensible — only override if the
    tenant's catalog has a specific quirk (e.g. furniture vertical with no master coverage
    at all → put "furniture" in embedding_disabled_for).

# Recommended order

1. Skim the meta-report fields below.
2. Note Tier 1 auto-mappings (already done). Don't redo those.
3. For ambiguous fields, use describe_field then sample_records.
4. For products that look familiar, find_similar_masters → if hits, link via
   the existing master's vertical. peek_master to confirm.
5. If no existing masters fit, propose_master_template ONCE per new vertical.
6. propose_field_mapping for each field that should map (skip the trivial ones
   that should land in raw_attributes).
7. propose_category_mapping for collections you can confidently classify
   (showcase / promo) or link to master categories.
8. propose_brand_mapping for every distinct vendor that appears in the
   meta-report (most tenants have 5-30 vendors — this is cheap and lets the
   applier handle every listing without falling back to "needs_review").
9. propose_junk_rule for vendors / axis patterns the meta-report shows are
   junk (gift wraps, services, warranty add-ons).
10. set_match_strategy ONCE if you want non-default thresholds — otherwise skip.
11. commit_artifact when done.

# Hard rules

- English-only MVP. Skip non-English fields entirely (don't even propose mappings).
- Don't propose 'cosmetics' as a new vertical — it's already promoted.
- Each tool response is ≤2KB. If you need more depth, drill in with peek_master,
  not by re-running the same query with bigger limits.
- Errors come back as {"error": "..."} JSON. Read them and adjust — don't loop.

# Master overview (what's already in master across ALL tenants)

`)
	if overview != nil && overview.TotalProducts > 0 {
		sb.WriteString(fmt.Sprintf("Total master_products: %d\n\n", overview.TotalProducts))
		for _, v := range overview.Verticals {
			sb.WriteString(fmt.Sprintf("vertical=%s (%d products)\n", v.Vertical, v.ProductCount))
			if len(v.TopBrands) > 0 {
				sb.WriteString("  top_brands: ")
				sb.WriteString(strings.Join(v.TopBrands, ", "))
				sb.WriteString("\n")
			}
			if len(v.Categories) > 0 {
				sb.WriteString("  master_categories: ")
				sb.WriteString(strings.Join(v.Categories, ", "))
				sb.WriteString("\n")
			}
		}
		if len(overview.RegisteredFields) > 0 {
			sb.WriteString("\nregistered_tier2_fields (use these as master.tier2.<key> in field_mapping):\n")
			byVert := make(map[string][]string)
			for _, fd := range overview.RegisteredFields {
				entry := fd.Key + ":" + fd.Type
				if len(fd.EnumValues) > 0 {
					entry += "(" + strings.Join(fd.EnumValues, "|") + ")"
				}
				byVert[fd.Vertical] = append(byVert[fd.Vertical], entry)
			}
			for v, fields := range byVert {
				sb.WriteString(fmt.Sprintf("  %s: %s\n", v, strings.Join(fields, ", ")))
			}
		} else {
			sb.WriteString("\nregistered_tier2_fields: (none yet — propose candidate:<key> with vertical for new ones)\n")
		}
	} else {
		sb.WriteString("(master is empty — every tenant listing this run will create new masters)\n")
	}

	sb.WriteString(`
# Meta-report (current tenant)

`)
	// Embed the meta-report compact JSON. We trust report.Fields is already
	// capped (≤80 fields, ≤10 values each from MetadataHarvest).
	if reportJSON, err := json.MarshalIndent(report, "", "  "); err == nil {
		sb.Write(reportJSON)
	}

	// Tier 1 auto-mapping summary (compact list, not full JSON to save tokens)
	sb.WriteString("\n\n# Tier 1 auto-mapping (already done — don't redo)\n\n")
	if len(tier1.FieldMapping) == 0 {
		sb.WriteString("(none)\n")
	} else {
		for path, target := range tier1.FieldMapping {
			sb.WriteString(fmt.Sprintf("- %s → %s\n", path, target.Target))
		}
	}

	if n := len(tier1.UnmappedFields); n > 0 {
		sb.WriteString(fmt.Sprintf("\n# Fields needing your attention (%d)\n\n", n))
		for _, p := range tier1.UnmappedFields {
			sb.WriteString("- " + p + "\n")
		}
	}

	return sb.String()
}

// =============================================================================
// Helpers
// =============================================================================

// stagingAsMetaReader returns the staging port as a metadataReader if the
// adapter implements it, else nil. The metadata harvester uses this trick
// (via interface assertion); we mirror it so the discovery agent can use
// the same dispatched access pattern in tools.
func stagingAsMetaReader(s ports.ShopifyStagingPort) metadataReader {
	if mr, ok := s.(metadataReader); ok {
		return mr
	}
	return nil
}

func truncateForTranscript(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// summarizeToolCall picks the first 1-2 keys of the input for the
// transcript. Avoids dumping full arg blobs into the log.
func summarizeToolCall(name string, input json.RawMessage) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(input, &m); err != nil {
		return "(unparseable args)"
	}
	keys := []string{"path", "field_path", "query", "vertical", "master_id", "shopify_collection_id"}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return k + "=" + truncateForTranscript(string(v), 80)
		}
	}
	return ""
}
