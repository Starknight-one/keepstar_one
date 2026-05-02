# Chunk 6b — Agent2 + first real LLM turn

## Context

Chunk 6a (commits `9d0169f` → `8121c85`, pushed) shipped the Anthropic SDK adapter — `LLMPort` with `ChatWithToolsCached` + `CountInputTokens`, cache-control placement (V4-proven pattern) covered by 8 unit tests, live smoke test green ($0.000048 / call), and the chunk-5.5 token measurement migrated to SDK without changing the numbers.

This chunk closes the gap from "we can call Anthropic" to "we run a full Agent2 turn end-to-end against V5". Concretely: a `go test`-driven scenario where a smoke test seeds product_card preset + 3 products, calls `Agent2Execute("Show 3 products")`, observes that Haiku invokes `visual_assembly` with `preset: "product_card", replicate: 3`, the tool runs the V5 engine pipeline, and the resulting Document carries 3 cloned cards bound to the actual catalog products.

**HARD GATE**: chunk-5.5 measurement showed V5's cacheable prefix at 3001 tokens — below Vlad's V4-prod stable-cache threshold of 4500. If 6b ships with a prefix below that gate, V5 doesn't get prompt-cache hits in production and pays full input price every turn, flipping the +48.8% per-turn token win into a +70.5% effective-cost loss over a 10-turn conversation. Chunk 6b's prompt-builder MUST clear ≥ 4500 cacheable tokens before the chunk closes.

V5 doesn't have Agent1 (catalog_search / state_filter) yet — that's a separate later chunk. For 6b's smoke test, the test directly populates `state.Current.Data` from `CatalogPort.ListProducts` so Agent2 has products to bind. Production deployment of Agent2 alone is fine; Agent1 lands when needed.

---

## What lands

1. Tool registry + ToolContext domain type
2. `visual_assembly` V5 tool — schema declares `preset` + `replicate` + `ops`. Implementation handles `preset` + `replicate` end-to-end through the V5 engine pipeline; `ops` field is parsed but **applied as a no-op for now** with a code comment + known-gap row pointing at chunk 6c. The schema includes `ops` so the LLM sees the architecture, the prompt teaches the ops vocabulary, and chunk 6c just needs to add the applier — no schema or prompt rewrite.
3. Agent2 prompt body — port V4's BUILDING / COMPOSING / FIELD BINDING / DECISION RULES / ANTI-PATTERNS sections, adapted for V5's preset+replicate-driven API. Target: prompt + tools cacheable prefix ≥ 4500 tokens.
4. `<fields>` block formatter from `FieldDefinitionPort`.
5. Per-tenant prompt cache via `sync.Map` (V4 pattern).
6. `Agent2Execute` use case — single-turn tool loop, history trimmed to 4 messages, `CacheConfig{tools:true, system:true, conversation:>1}`, `ToolChoice: "any"`, graceful retry on tool error with empty params.
7. End-to-end smoke test (build tag `live`, gated on `ANTHROPIC_API_KEY` + `TEST_DATABASE_URL`).
8. Re-run token measurement, confirm ≥ 4500 prefix gate clears.

`<tenant_design_context>` block — **deferred** to a later iteration. V4 reads canvas snapshot tables; V5 will read `v5_presets` + `v5_components` adapters. Useful but not blocking — Agent2 can pick presets from a hardcoded list in the prompt for chunk 6b. Add the dynamic block in chunk 6c or 6d.

---

## Files to add / modify

| File | Status | Purpose |
|---|---|---|
| `project_v5/backend/internal/domain/tool_context.go` | added | `ToolContext` carries SessionID + TenantSlug + per-call deps (StatePort, CatalogPort, PresetPort, ComponentPort) — mirrors V4's `ToolContext` |
| `project_v5/backend/internal/tools/tool_registry.go` | added | `Registry` with sorted-by-name iteration; `GetDefinitions()` returns the byte-stable tools array |
| `project_v5/backend/internal/tools/tool_visual_assembly.go` | added | V5 visual_assembly: parses preset+replicate (ops parsed-but-ignored), runs `Materialise → ExpandReplicates → ResolveAndInline → BindData`, marshals to JSON, writes `state.Current.Template` via `StatePort.UpdateTemplate` |
| `project_v5/backend/internal/tools/tool_visual_assembly_test.go` | added | 3-4 unit tests with mock state + in-memory presets/components — verify the pipeline runs and the resulting Template contains expected node ids |
| `project_v5/backend/internal/prompts/agent2_prompt.go` | added | Base system prompt body (~12-14 KB / 2800-3000 tokens). Sections: HOW IT WORKS / PRESETS / OPS (vocabulary + examples) / FIELD BINDING / BUILDING / COMPOSING / MODIFYING / DECISION RULES / ANTI-PATTERNS. Ports V4's content, adapted for V5's preset+replicate API. Includes a hardcoded preset catalog for chunk 6b (will become dynamic via tenant_design_context later) |
| `project_v5/backend/internal/prompts/fields_block.go` | added | `FormatFieldsBlock(ctx, fdPort, tenantSlug, entityType) (string, error)` — returns `<fields entity="product">…</fields>` with type/subtype/label/unit/slot/samples columns, sorted by FieldDefinition.Priority ASC |
| `project_v5/backend/internal/usecases/prompt_cache.go` | added | `PromptCache` struct wrapping `sync.Map`. Key: tenant slug. Value: `{prompt, builtAt}`. No invalidation in 6b (will hook into design-context version when tenant_design_context lands) |
| `project_v5/backend/internal/usecases/agent2_execute.go` | added | `Agent2Execute` use case: GetState → buildSystemPrompt(cached) → trim Agent2History to last 4 → ChatWithToolsCached → for each ToolCall: registry.Execute → AppendAgent2History → reload state, return Document. Single retry with `{}` params on tool error (V4 pattern) |
| `project_v5/backend/internal/usecases/agent2_smoke_test.go` | added | Build tag `live`; gated on ANTHROPIC_API_KEY+TEST_DATABASE_URL. Spins up adapters, picks heybabes-style tenant with ≥3 products, seeds product_card preset + components, creates session, prepopulates `state.Current.Data` from catalog, calls Agent2Execute("Show 3 products"), asserts: tool call observed, Document has 3 cloned cards bound to products, Usage.CacheCreationInputTokens > 0 (cache write happened) |
| `project_v5/backend/internal/engine/tokens/sketches/v5_system_prompt.txt` | rewritten | replaced with the production prompt content from `agent2_prompt.go` so the token measurement reflects the same prompt LLM sees |
| `project_v5/backend/internal/engine/tokens/sketches/v5_tool_def.json` | rewritten | regenerated from the production `visual_assembly` schema |
| `docs/Updates/v5/plans/chunk-6b-agent2.md` | added | frozen plan |
| `docs/Updates/v5/v5_<UTC>.md` | added | session log |

No existing files modified except the two prompt sketches to keep the measurement in sync.

---

## visual_assembly tool — concrete contract

### Schema (declared)

```json
{
  "name": "visual_assembly",
  "description": "Build or modify a scene-graph Document. Pick a preset, set replicate count, optionally layer ops on top.",
  "input_schema": {
    "type": "object",
    "required": ["preset"],
    "properties": {
      "preset":    { "type": "string", "description": "Published preset name…" },
      "replicate": { "type": "integer", "description": "How many clones to fan out…" },
      "ops":       { "type": "array",   "description": "Operations on top of the preset (insert/update/delete/move/override)…" }
    }
  }
}
```

### Implementation flow (chunk 6b scope)

```
visual_assembly.Execute(ctx, toolCtx, params):
  1. presetName := params["preset"].(string); require non-empty
  2. replicateCount := params["replicate"].(int); default 0 (no fan-out)
  3. ops := params["ops"].([]any); LOG ignored count, do not apply (chunk 6c)
  4. preset := PresetPort.GetPublishedPreset(toolCtx.TenantSlug, presetName)
  5. componentDocs := for each ref in preset, ComponentPort.GetPublishedComponent(toolCtx.TenantSlug, refName)
     (For chunk 6b, just load ALL published components for the tenant — preset's ref targets are matched by id, extras are harmless. ComponentPort.ListPublishedComponents already exists.)
  6. doc := engine.Materialise(preset.Document, componentDocs)
  7. data := state.Current.Data → []map[string]any (already populated by smoke test or future Agent1)
  8. engine.ExpandReplicates(doc, replicateCount)
  9. engine.ResolveAndInline(doc)
  10. engine.BindData(doc, data)
  11. templateMap := json.Marshal(doc) → unmarshal into map[string]interface{}
  12. StatePort.UpdateTemplate(ctx, sessionID, templateMap, DeltaInfo{Source: "llm", ActorID: "agent2", Trigger: "visual_assembly"})
  13. Return ToolResult{Content: "OK; preset=X replicate=N bound=M missing=K", IsError: false}
```

### What `ops` ignoring looks like

```go
if rawOps, ok := params["ops"]; ok {
    if opsList, ok := rawOps.([]interface{}); ok && len(opsList) > 0 {
        // FIXME(chunk-6c): wire ops into engine.Command + CommandHistory.
        // For chunk 6b ship date, ops are intentionally a no-op so we can
        // get a real LLM turn end-to-end without rebuilding the engine
        // applier. The schema still declares ops so the prompt + LLM can
        // reason about the architecture.
        slog.Debug("visual_assembly: ignoring ops (chunk-6c work)", "count", len(opsList))
    }
}
```

This single conditional is the only place chunk 6c will need to grow. Adding it now keeps the tool's exported surface stable across the next sub-chunk.

---

## Prompt body — meeting the cache gate

### Current state (chunk 5.5 measurement)

- V5 prompt sketch: 1556 tokens (5637 bytes)
- V5 tool def: 1445 tokens (4179 bytes)
- **Total cacheable prefix: 3001 tokens** ← below 4500 gate

### Target

Prompt body **alone** ≥ 3000 tokens (~11 KB). Tool def stays around 1500-1700 tokens after we add inline doc strings on `ops` schema. Combined target 4500-4700 tokens — comfortably above the gate with some headroom for tenant-design-context drift later.

### Sections to port from V4 (`project_v4/backend/internal/prompts/prompt_compose_widgets.go:13-325`)

1. **HOW IT WORKS** — describe v9 scene-graph (Document → Frame/Text/Image/Ref nodes), preset + replicate model, automatic engine pipeline (Materialise → ExpandReplicates → ResolveAndInline → BindData)
2. **PRESETS** — list V4's preset catalog port (product_card, product_card_compact, product_card_horizontal, product_card_list_row, product_detail, product_detail_horizontal, text_explainer, empty_not_found, error_generic, catalog_category_card, liked_grid, cart_grid). For chunk 6b, hardcode the V4 list as a starting catalog — V5 will accumulate its own as canvas microservice ships
3. **OPS** — vocabulary (insert / update / delete / move / override), targeting rules (id from tree_map, ref slot ids, NOT component-internal ids — they collide across instances)
4. **FIELD BINDING** — slot ↔ field matching playbook ported verbatim (slot=title→short text, slot=hero→image, etc.)
5. **FORMAT + WRAPPER** — declarative properties on leaf nodes; `BindData` does NOT format; frontend renders. Single example op for changing format
6. **BUILDING examples** — 3-4 concrete tool calls (preset_only, preset+ops, modify mode) using V5 syntax
7. **MODIFYING EXISTING** — when tree_map is present, target by id, don't pass preset
8. **TREE_MAP** — what Agent2 sees in modify mode (instances list, components list, data_count, preset_in_use)
9. **DECISION RULES** — pick a preset whenever one matches, props are merged in update ops, no over-specification, etc.
10. **ANTI-PATTERNS** — V4 list adapted (don't hardcode N copies, don't format values yourself, don't target component-internal ids across instances, don't output text)

If after writing all sections the prefix is still < 4500, iterate — add more `BUILDING` examples (each is ~200-300 tokens of high-signal content), expand FIELD BINDING decision tree, or add a TROUBLESHOOTING section.

### Verification of the gate

```sh
ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY go test -tags=tokens -v ./internal/engine/tokens/...
```

The existing `TestTokenComparisonV4vsV5` test already prints `cacheable prefix` and warns when it's below 4500. Chunk 6b's exit criterion: this test logs `stable cache: true` for V5.

---

## Agent2Execute — concrete contract

```go
type Agent2ExecuteRequest struct {
    SessionID  string
    TenantSlug string
    UserQuery  string
}

type Agent2ExecuteResponse struct {
    Document  map[string]interface{}      // marshalled engine.Document
    ToolCalls []domain.ToolCall            // what the LLM emitted
    Usage     domain.LLMUsage              // tokens + cost
    LatencyMs int64
}

func (uc *Agent2Execute) Execute(ctx context.Context, req Agent2ExecuteRequest) (*Agent2ExecuteResponse, error)
```

Steps inside Execute (port of V4 `agent2_execute.go:103-383`, simplified — no Agent1, no design-context):

1. `state := uc.statePort.GetState(ctx, req.SessionID)` — must exist; created upstream by session/init handler in chunk 6c
2. `systemPrompt := uc.promptCache.GetOrBuild(ctx, req.TenantSlug)` (which under the hood calls `agent2.AssembleSystemPrompt(basePrompt, fieldsBlock)`)
3. `messages := append(state.Agent2History[max(0, len-4):], LLMMessage{Role: "user", Content: req.UserQuery})`
4. `tools := uc.toolRegistry.GetDefinitions()`
5. `cfg := ports.CacheConfig{CacheTools: true, CacheSystem: true, CacheConversation: len(messages) > 1, ToolChoice: "any"}`
6. `resp := uc.llm.ChatWithToolsCached(ctx, systemPrompt, messages, tools, cfg)`
7. For each `tc := range resp.ToolCalls`:
   - `result := uc.toolRegistry.Execute(ctx, ToolContext{...}, tc)` — wraps in graceful retry: on error, retry once with `{}` input (V4 fallback pattern)
   - Append `LLMMessage{Role: "assistant", ToolCalls: [tc]}` and `LLMMessage{Role: "user", ToolResult: &ToolResult{ToolUseID: tc.ID, Content: result.Content, IsError: result.IsError}}` to history buffer
8. `uc.statePort.AppendAgent2History(ctx, req.SessionID, historyBuffer)`
9. `state := uc.statePort.GetState(ctx, req.SessionID)` — reload to pick up the tool's UpdateTemplate write
10. Return `{Document: state.Current.Template, ToolCalls: resp.ToolCalls, Usage: resp.Usage, LatencyMs: …}`

---

## Smoke test design

`internal/usecases/agent2_smoke_test.go`:

- Build tag `live`; skips if either `ANTHROPIC_API_KEY` or `TEST_DATABASE_URL` is absent.
- Setup: pick the first tenant with ≥ 3 published presets + 3 products (heybabescosmetics likely candidate).
- Seed `product_card` preset + `price_rating` / `brand_badge` components for that tenant — reuse `seedComponentFromBytes` / `seedPresetFromBytes` helpers from `engine_pipeline_integration_test.go` (chunks 4-5).
- Create a session via `StatePort.CreateState`.
- Pre-populate `state.Current.Data` with 3 catalog products via `StatePort.UpdateData` — simulates "Agent1 already ran" since we don't have Agent1 yet.
- Call `Agent2Execute(SessionID, TenantSlug, "Show me 3 products from your catalog")`.
- Assertions:
  - `len(resp.ToolCalls) == 1`
  - `resp.ToolCalls[0].Name == "visual_assembly"`
  - `resp.ToolCalls[0].Input["preset"] == "product_card"` (or any product preset)
  - `resp.ToolCalls[0].Input["replicate"] == 3` (or close — LLM might pass float64; tolerate)
  - `resp.Document["children"]` exists and contains ≥ 3 instance roots (3 cloned cards + reusable defs)
  - `resp.Usage.InputTokens > 0`
  - `resp.Usage.CacheCreationInputTokens > 0` ← proves cache was written (gate verification)
  - On a SECOND call to Agent2Execute, `resp.Usage.CacheReadInputTokens > 0` ← proves cache was read (round-trip evidence)
- Total cost: ~$0.005-0.01 per run (one Haiku call, ~5K input + few hundred output tokens).

---

## Verification

```sh
cd project_v5/backend

# Build + vet
go build ./... && go build -tags=integration ./... && go build -tags=live ./... && go build -tags=tokens ./...
go vet ./... && go vet -tags=integration ./... && go vet -tags=live ./... && go vet -tags=tokens ./...

# Unit tests
go test -count=1 ./...
# new tool_visual_assembly_test cases pass alongside chunks 1-6a

# Integration on Neon
TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration -count=1 ./...
# 14/14 still green

# Live Agent2 smoke (this is the key chunk-6b verification)
ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY \
TEST_DATABASE_URL=$DATABASE_URL \
  go test -tags=live -v -count=1 ./internal/usecases/...
# logs: tool call observed, Document has 3 bound cards, cache created+read

# Cache-prefix gate (HARD GATE)
ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY go test -tags=tokens -v ./internal/engine/tokens/...
# expect: V5 cacheable prefix ≥ 4500, "stable cache: true"
# expect: 10-turn effective cost V5 < V4 (was 37050 vs 21725 in chunk 5.5)
```

---

## Commit plan (chunk 6b)

5 commits:

1. `feat(v5): tool registry + ToolContext domain (chunk 6b)` — domain/tool_context.go, tools/tool_registry.go
2. `feat(v5): visual_assembly tool — preset + replicate pipeline (chunk 6b)` — tools/tool_visual_assembly.go + unit tests; ops field declared in schema, ignored in code with FIXME
3. `feat(v5): Agent2 prompt-builder + fields block + per-tenant cache (chunk 6b)` — prompts/agent2_prompt.go, prompts/fields_block.go, usecases/prompt_cache.go; updates tokens/sketches to match
4. `feat(v5): Agent2Execute use case + live smoke test (chunk 6b)` — usecases/agent2_execute.go + agent2_smoke_test.go
5. `docs(v5): chunk 6b session log + frozen plan + cache-gate verification`

Then `git push origin v5`.

---

## Time + risk

- ~1 working session (mostly prompt-engineering iteration to clear the cache gate; everything else is mechanical port).
- Largest risk: prompt body grows but remains under 4500 tokens. Mitigation: the measurement test runs after each prompt iteration; if at the end of writing we're at 4200, port one more BUILDING example.
- Second risk: smoke test flakes because Haiku picks a different valid preset name (e.g. `product_card_compact` instead of `product_card`). Mitigation: assert "preset starts with `product_`" rather than exact match.

---

## Out of scope (explicitly deferred to 6c / 6d)

- Ops applier — schema declares it, prompt teaches it, code ignores it. Chunk 6c wires the `engine.CommandHistory` translation layer.
- `<tenant_design_context>` block — V5 reads canvas tables. For 6b the prompt has a hardcoded preset catalog. Add when chunk 6c brings up the HTTP layer + canvas-microservice handshake.
- HTTP endpoints — chunk 6c.
- Tx fix on `zoneWriteWithDelta` + retry on `AddDelta` — chunk 6d.
- Tracer port + spans — chunk 6d.

---

## Sketches for later sub-chunks (preserved from earlier plan)

### Chunk 6c — HTTP server + handlers + DI

`cmd/server/main.go` becomes a real entrypoint. Three endpoints: `/api/v1/pipeline` (calls Agent2Execute), `/api/v1/session/init`, `/api/v1/session/{id}`. Middleware: logging, CORS, tenant. Migrations DI (state + preset + component). Graceful shutdown. Adds the ops applier to `visual_assembly` tool. Adds `<tenant_design_context>` to the prompt-builder.

### Chunk 6d — cleanup + caching plumbing

`zoneWriteWithDelta` tx wrap. `AddDelta` retry/advisory-lock. Tracer port + `domain.SpanFromContext` re-add to PG adapters + middleware integration. LLMMessage cache_control hint plumbing if needed beyond what 6a's adapter already does.
