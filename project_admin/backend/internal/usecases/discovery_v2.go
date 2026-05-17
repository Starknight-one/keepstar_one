// Package usecases — DiscoveryV2 is the new discovery agent for the 6-step
// catalog flow. It explores tenant inbox data via narrow tools and produces
// a MappingArtifactV2 (vertical + field_map). Replaces the legacy
// DiscoveryAgent (35-LOC system prompt, 8 tools, candidates/promotion).
//
// Run shape:
//   1. Caller invokes Discover(ctx, tenantID, trigger, payload).
//   2. We open an agent_runs row and a tenant_action_log row (discovery_start).
//   3. Loop: send conversation → dispatch tool_use blocks → append tool_result.
//   4. Terminate on commit_artifact (success), end_turn (no commit → failed),
//      or budget exhaustion (budget_exhausted).
//   5. On success, save artifact via MappingArtifactV2Port; on failure,
//      leave previous artifact in place. Either way Finish() the run row
//      with final status, tokens, cost.
//
// Budget caps:
//   discoveryV2MaxTurns         — hard turn count cap.
//   discoveryV2WallclockBudget  — wallclock limit (real-time).
//   discoveryV2BudgetUSD        — dollar budget (Sonnet 4.6 pricing).
//   discoveryV2HeavyToolsCap    — max peek_full_rows calls per run.
//   discoveryV2BudgetForceFinalize — at this % of $ budget, last turn sent
//     gets a system reminder demanding commit_artifact RIGHT NOW.
package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"keepstar-admin/internal/adapters/anthropic"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

const (
	discoveryV2MaxTurns           = 30
	discoveryV2WallclockBudget    = 10 * time.Minute
	discoveryV2BudgetUSD          = 5.00
	discoveryV2BudgetForceFinalize = 0.90 // at 90% of $ budget, push commit_artifact
	discoveryV2HeavyToolsCap      = 10

	// Sonnet 4.6 pricing per 1M tokens (refresh if pricing changes).
	sonnet46InputUSDPer1M       = 3.00
	sonnet46OutputUSDPer1M      = 15.00
	sonnet46CacheReadUSDPer1M   = 0.30
	sonnet46CacheWriteUSDPer1M  = 3.75
)

// errBudgetExhausted is returned when any of the three budgets is hit
// before commit_artifact.
var errBudgetExhausted = errors.New("discovery v2: budget exhausted before commit")

// AgentSender is the minimal Anthropic adapter contract used by the loop.
type AgentSender interface {
	Send(ctx context.Context, req anthropic.MessagesRequest) (*anthropic.MessagesResponse, error)
}

// DiscoveryV2 runs the new discovery loop. Stateless across runs.
type DiscoveryV2 struct {
	llm       AgentSender
	inbox     ports.InboxPort
	artifact  ports.MappingArtifactV2Port
	actionLog ports.TenantActionLogPort
	agentRuns ports.AgentRunsPort
	log       *logger.Logger
}

func NewDiscoveryV2(
	llm AgentSender,
	inbox ports.InboxPort,
	artifact ports.MappingArtifactV2Port,
	actionLog ports.TenantActionLogPort,
	agentRuns ports.AgentRunsPort,
	log *logger.Logger,
) *DiscoveryV2 {
	return &DiscoveryV2{
		llm:       llm,
		inbox:     inbox,
		artifact:  artifact,
		actionLog: actionLog,
		agentRuns: agentRuns,
		log:       log,
	}
}

// Discover runs the agent for a tenant and returns the committed artifact
// (or nil on failure). Always finishes the agent_runs row and emits a
// discovery_done action_log entry.
//
// trigger ∈ {'first_install', 'manual', 'mapping_miss', 'unknown_vertical'}.
// triggerPayload is free-form context — for mapping_miss it carries {field,
// sample_value, inbox_item_id}; for unknown_vertical it carries
// {missing_vertical, sample_inbox_item_id}; for first_install it's empty;
// for manual it's {requested_by_user_id}.
func (d *DiscoveryV2) Discover(ctx context.Context, tenantID, trigger string, triggerPayload json.RawMessage) (*domain.MappingArtifactV3, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("discover: empty tenant_id")
	}
	if trigger == "" {
		trigger = "manual"
	}

	runID, err := d.agentRuns.Start(ctx, &ports.AgentRunStart{
		TenantID:       tenantID,
		Trigger:        trigger,
		TriggerPayload: triggerPayload,
	})
	if err != nil {
		return nil, fmt.Errorf("discover: start run: %w", err)
	}

	_ = d.actionLog.Log(ctx, &ports.TenantActionLogEntry{
		TenantID: tenantID,
		Action:   "discovery_start",
		Status:   "ok",
		Payload:  marshalOrEmpty(map[string]any{"run_id": runID, "trigger": trigger}),
	})

	artifact, loopErr := d.runLoop(ctx, tenantID, trigger, triggerPayload, runID)

	finalStatus := "success"
	artifactID := ""
	switch {
	case errors.Is(loopErr, errBudgetExhausted):
		finalStatus = "budget_exhausted"
	case loopErr != nil:
		finalStatus = "failed"
	}
	if artifact != nil {
		if err := d.artifact.Save(ctx, tenantID, artifact); err != nil {
			d.log.Warn("discovery_v2_save_artifact_failed", "tenant", tenantID, "run", runID, "error", err)
			finalStatus = "failed"
		} else {
			artifactID = tenantID // proxy — artifact lives on tenant_catalog_schema row
		}
	}

	if err := d.agentRuns.Finish(ctx, runID, finalStatus, artifactID); err != nil {
		d.log.Warn("discovery_v2_finish_run_failed", "run", runID, "error", err)
	}
	_ = d.actionLog.Log(ctx, &ports.TenantActionLogEntry{
		TenantID: tenantID,
		Action:   "discovery_done",
		Status:   actionStatusFromFinal(finalStatus),
		Payload: marshalOrEmpty(map[string]any{
			"run_id":         runID,
			"final_status":   finalStatus,
			"committed":      artifact != nil,
		}),
	})
	return artifact, loopErr
}

func (d *DiscoveryV2) runLoop(ctx context.Context, tenantID, trigger string, triggerPayload json.RawMessage, runID string) (*domain.MappingArtifactV3, error) {
	wallClockDeadline := time.Now().Add(discoveryV2WallclockBudget)

	systemBlocks := buildDiscoveryV2System(trigger, triggerPayload)
	tools := discoveryTools()

	messages := []anthropic.Message{
		{
			Role: "user",
			Content: []anthropic.ContentBlock{
				{Type: "text", Text: "Begin discovery. First, get a sense of the catalog size and field list."},
			},
		},
	}

	const maxNudges = 3 // retry-with-nudge when model emits end_turn w/o commit

	var (
		heavyCallsUsed int
		totalCostUSD   float64
		forceFinalize  bool
		nudgesUsed     int
	)

	for turn := 0; turn < discoveryV2MaxTurns; turn++ {
		if time.Now().After(wallClockDeadline) {
			return nil, errBudgetExhausted
		}
		if totalCostUSD >= discoveryV2BudgetUSD {
			return nil, errBudgetExhausted
		}

		req := anthropic.MessagesRequest{
			Model:        "claude-sonnet-4-6",
			MaxTokens:    4096,
			SystemBlocks: systemBlocks,
			Tools:        tools,
			Messages:     messages,
		}
		resp, err := d.llm.Send(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("llm send turn %d: %w", turn, err)
		}

		// Cost accounting.
		costDelta := costForUsage(resp.Usage)
		totalCostUSD += costDelta
		_ = d.agentRuns.AddTokens(ctx, runID, resp.Usage.InputTokens+resp.Usage.CacheReadInputTokens+resp.Usage.CacheCreationInputTokens, resp.Usage.OutputTokens, costDelta)

		// Append assistant message verbatim for the next turn's context.
		messages = append(messages, anthropic.Message{Role: "assistant", Content: resp.Content})

		// Collect tool_use blocks; dispatch each; build tool_result blocks.
		var toolUses []anthropic.ContentBlock
		for _, blk := range resp.Content {
			if blk.Type == "tool_use" {
				toolUses = append(toolUses, blk)
			}
		}

		if len(toolUses) == 0 {
			// Model ended its turn without invoking any tool. Common path:
			// agent wrote a text "I'm ready to commit" then stopped without
			// the actual tool_use. Nudge it up to maxNudges times before
			// giving up.
			if nudgesUsed >= maxNudges {
				return nil, fmt.Errorf("discovery v2: model ended turn without committing artifact after %d nudges (stop_reason=%s)", nudgesUsed, resp.StopReason)
			}
			nudgesUsed++
			messages = append(messages, anthropic.Message{
				Role: "user",
				Content: []anthropic.ContentBlock{
					{Type: "text", Text: "You stopped without calling any tool. You MUST call commit_artifact now with the best mapping you can produce from what you've seen. If you genuinely need more info, call one more aggregate tool (list_fields / sample_values / field_stats / count_by) and then commit. Do not produce a turn with only text again."},
				},
			})
			continue
		}

		var toolResults []anthropic.ContentBlock
		var committed *domain.MappingArtifactV3

		for _, tu := range toolUses {
			t0 := time.Now()
			dr := dispatchTool(ctx, d.inbox, tenantID, tu.Name, tu.Input, &heavyCallsUsed, discoveryV2HeavyToolsCap)
			elapsed := time.Since(t0)

			_ = d.agentRuns.AppendTool(ctx, runID, ports.AgentToolCall{
				Name:       tu.Name,
				Args:       tu.Input,
				ResultPrev: dr.Preview,
				DurationMS: elapsed.Milliseconds(),
				Error: func() string {
					if dr.IsError {
						return "tool returned error"
					}
					return ""
				}(),
				At: time.Now(),
			})

			toolResults = append(toolResults, anthropic.ContentBlock{
				Type:      "tool_result",
				ToolUseID: tu.ID,
				Content:   dr.Output,
				IsError:   dr.IsError,
			})

			if dr.CommitArtifact != nil {
				committed = dr.CommitArtifact
			}
		}

		// Append the user-side message carrying tool_results.
		userMsg := anthropic.Message{Role: "user", Content: toolResults}

		// If we're approaching budget, append a force-finalize nudge.
		if !forceFinalize && totalCostUSD >= discoveryV2BudgetUSD*discoveryV2BudgetForceFinalize {
			forceFinalize = true
			userMsg.Content = append(userMsg.Content, anthropic.ContentBlock{
				Type: "text",
				Text: "Budget warning: 90% of the $5 cost cap consumed. STOP exploring — submit commit_artifact on the NEXT turn with the best mapping you have, even if incomplete. Use vertical='unknown' if uncertain.",
			})
		}

		messages = append(messages, userMsg)

		if committed != nil {
			committed.BuiltAt = time.Now().UTC()
			return committed, nil
		}
	}

	return nil, fmt.Errorf("discovery v2: hit max turns (%d) without commit", discoveryV2MaxTurns)
}

// buildDiscoveryV2System constructs the SystemBlocks slice. The static
// instruction block is marked cacheable; dynamic trigger context lives in
// a separate, uncached block at the end.
func buildDiscoveryV2System(trigger string, triggerPayload json.RawMessage) []anthropic.SystemBlock {
	staticPrompt := strings.TrimSpace(`
You are Keepstar's catalog discovery agent. Your job: examine a tenant's raw
product data in the inbox and produce a mapping artifact that tells our code
how to translate every product — across every vertical the tenant carries —
into our master schema.

Pipeline you live in:
  source (Shopify) → inbox (raw JSONB) → YOU → mapping artifact (v3, branched)
                                          ↓
                            apply_v2 reads inbox + your artifact → master catalog

Artifact shape (v3):
  - branches: one per vertical class the tenant ships products in
    (cosmetics, electronics, furniture, haircare, apparel, footwear, food,
    ski, unknown). Each branch has its own field_map.
  - classify_rules: row-level fallback when shopify product_type doesn't
    alias to a known vertical. Tiny DSL (case-insensitive):
        "product_type contains 'sofa'"
        "brand = 'Apple'"
        "name contains 'serum'"
        "tag = 'sale'"
    Apply_v2 evaluates in order; first match wins.

Tools (full JSON schemas in tool defs):
  count_total      — row count
  list_fields      — distinct top-level JSONB keys
  sample_values    — distinct values of one field
  count_by         — frequency distribution of one field
  field_stats      — non-null/distinct/samples/min/max for one field
  peek_full_rows   — HEAVY: 1-5 full raw rows (cap: 10 calls/run)
  commit_artifact  — finalize and exit

Target prefixes inside each branch's field_map (THREE, nothing else):
  master.<col>     — Tier 1 column on master_products: name, brand,
                     description, sku, vertical, image_url, gtin
  cosmetics.<col>  — Tier 2 typed columns. VALID ONLY inside the
                     'cosmetics' branch. Allowed columns: skin_type,
                     concern, key_ingredients, target_area, product_form,
                     texture, routine_step, routine_time,
                     application_method, free_from, scent, spf,
                     marketing_claim, benefits, how_to_use, volume_ml,
                     weight_g, unit_count.
  tier3.<key>      — JSONB pocket on master_products. Free-form key.

CRITICAL: cosmetics is the ONLY vertical with a typed Tier 2 table today.
For electronics, furniture, haircare, apparel, footwear, food, ski, or
unknown — EVERY non-master attribute goes into tier3.<key>. Do NOT invent
prefixes like 'electronics.cpu' or 'furniture.material' — apply_v2 will
warn and reroute to tier3, but you waste tokens.

Examples by branch:
  cosmetics serum  → cosmetics.skin_type, cosmetics.concern, cosmetics.volume_ml
  electronics laptop → tier3.cpu, tier3.ram_gb, tier3.screen_size
  furniture sofa   → tier3.material, tier3.dimensions, tier3.upholstery_type
  ski boots        → tier3.size_mondo, tier3.flex_index

Transforms supported in apply_v2 (set 'transform' on a rule):
  lowercase, trim
  split:<delim>      — only when target is a text[] column (cosmetics arrays)
  ml_from_string     — '30 ml' → 30
  g_from_string      — '200 g' → 200
  bool_from_yesno    — 'yes'/'true'/'1' → true
  int                — strconv.Atoi
  numeric            — ParseFloat

Approach:
  1. count_total + list_fields. See the catalog size and field shape.
  2. Use field_stats / sample_values / count_by on each promising field to
     learn its distinct/sample/range before deciding its target.
  3. Inspect product_type distribution via count_by — this drives branch
     selection. Multi-vertical tenants (e.g. cosmetics + electronics) need
     a branch per class. Single-vertical tenants need one branch.
  4. peek_full_rows ONLY when aggregates can't disambiguate.
  5. Aim for ~15 tool calls. Budget cap is $5. ~$0.01-0.05 per call.

Branch decision:
  - count_by 'product_type' tells you which verticals are present. Build
    a branch for each.
  - If product_type is missing or unreliable, lean on classify_rules
    (brand-based or name-based) so apply_v2 still routes correctly.
  - When unsure, commit an 'unknown' branch — apply_v2 routes that vertical's
    attributes to tier3 and chat search still works.

Trigger-specific behaviour:
  - first_install: build the full artifact from scratch.
  - mapping_miss: a previous apply failed on one field. Investigate THAT
    field, append/fix one rule, commit. Don't re-explore the whole catalog.
  - unknown_vertical: tenant added a product whose vertical you didn't
    cover. Add ONE new branch for that vertical and commit.
  - manual: full re-discovery; the curator pressed a button.

When ready, call commit_artifact.
	`)

	blocks := []anthropic.SystemBlock{
		{Type: "text", Text: staticPrompt, CacheControl: &anthropic.CacheControl{Type: "ephemeral"}},
	}
	// Dynamic trigger context — small, not cached.
	dynamic := fmt.Sprintf("Run trigger: %s.", trigger)
	if len(triggerPayload) > 0 && string(triggerPayload) != "{}" && string(triggerPayload) != "null" {
		dynamic += fmt.Sprintf(" Trigger payload: %s.", string(triggerPayload))
	}
	if trigger == "mapping_miss" {
		dynamic += " A previous apply hit a mapping miss — focus your tools on the offending field rather than re-discovering the whole catalog."
	}
	blocks = append(blocks, anthropic.SystemBlock{Type: "text", Text: dynamic})
	return blocks
}

// costForUsage prices one Anthropic round-trip using Sonnet 4.6 rates.
func costForUsage(u anthropic.Usage) float64 {
	cost := float64(u.InputTokens) * sonnet46InputUSDPer1M / 1_000_000.0
	cost += float64(u.OutputTokens) * sonnet46OutputUSDPer1M / 1_000_000.0
	cost += float64(u.CacheCreationInputTokens) * sonnet46CacheWriteUSDPer1M / 1_000_000.0
	cost += float64(u.CacheReadInputTokens) * sonnet46CacheReadUSDPer1M / 1_000_000.0
	return cost
}

func actionStatusFromFinal(final string) string {
	switch final {
	case "success":
		return "ok"
	case "budget_exhausted":
		return "warning"
	default:
		return "error"
	}
}

func marshalOrEmpty(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}
