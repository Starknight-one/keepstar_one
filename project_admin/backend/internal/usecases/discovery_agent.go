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

	// Cache the system prompt + tool list. Both are static across all turns
	// of this run — caching makes the per-turn cost ~10× smaller after the
	// first turn (which writes the cache). The marker on the LAST tool tells
	// Anthropic to cache everything up to and including it.
	systemBlocks := []anthropic.SystemBlock{{
		Type:         "text",
		Text:         buildDiscoverySystemPrompt(report, tier1),
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
}

func NewArtifactBuilder(tier1 AutoMapTier1Result) *ArtifactBuilder {
	b := &ArtifactBuilder{
		fieldMapping:    make(map[string]domain.FieldMappingTarget),
		categoryMapping: make(map[string]domain.CategoryMappingTarget),
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
	return &domain.MappingArtifact{
		Version:         1,
		FieldMapping:    fm,
		CategoryMapping: cm,
		MasterTemplates: tpls,
		MatchStrategy:   matchStrategy,
		VariantStrategy: variantStrategy,
		AgentNotes:      notes,
	}
}

// =============================================================================
// System prompt construction
// =============================================================================

func buildDiscoverySystemPrompt(report *domain.MetaReport, tier1 AutoMapTier1Result) string {
	var sb strings.Builder
	sb.WriteString(`You are the Catalog Discovery Agent for the Keepstar admin platform.
Your job: produce a MAPPING ARTIFACT that tells the harvester how to convert
this tenant's Shopify catalog into our master/listing schema.

You have 30 tool-call turns and an 8-minute wallclock budget. Use them well —
Sonnet costs real money, but a wrong artifact costs more.

# How our schema works (read carefully)

- master_products = one canonical row per real product, shared across tenants.
  Fields: name, brand, description, image_url, vertical, tier3 (JSONB).
- master_variants = one row per SKU. Fields: sku, gtins (TEXT[]), weight_g,
  volume_ml, length_mm/width_mm/height_mm, color, size, material, axes (JSONB).
- master_cosmetics = Tier 2 typed columns for cosmetics:
  skin_type[], concern[], ingredients[], scent, spf.
  Other verticals (furniture, electronics, ...) DON'T have a Tier 2 table yet —
  for those, attribute candidates land in tier3 JSONB until curator promotes.
- catalog.products (listing) = per-tenant overrides. Fields: master_variant_id,
  display_name (short), original_name (full), price, stock_quantity,
  raw_attributes (JSONB), media (JSONB).

# What you produce

Mapping targets you can use in propose_field_mapping(target=...):
  master.brand, master.description, master.image_url, master.name
  master_variants.sku, master_variants.gtins[], master_variants.weight_g,
    master_variants.volume_ml, master_variants.color, master_variants.size,
    master_variants.material, master_variants.image_url
  master_cosmetics.skin_type[], master_cosmetics.concern[],
    master_cosmetics.ingredients[], master_cosmetics.scent, master_cosmetics.spf
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
8. commit_artifact when done.

# Hard rules

- English-only MVP. Skip non-English fields entirely (don't even propose mappings).
- Don't propose 'cosmetics' as a new vertical — it's already promoted.
- Each tool response is ≤2KB. If you need more depth, drill in with peek_master,
  not by re-running the same query with bigger limits.
- Errors come back as {"error": "..."} JSON. Read them and adjust — don't loop.

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
