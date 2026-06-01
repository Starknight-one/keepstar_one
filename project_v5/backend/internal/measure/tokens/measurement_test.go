//go:build tokens

// Token-cost paper measurement: V4 vs V5 input-tokens for a single
// representative scenario ("user asks: show me 3 products").
//
// Run with:
//
//	ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY \
//	    go test -tags=tokens -v ./internal/engine/tokens/...
//
// Test logs results via t.Logf and never asserts pass/fail — the goal is a
// number we can re-measure as v5 evolves, not a regression gate.

package tokens

import (
	_ "embed"
	"encoding/json"
	"os"
	"testing"
)

//go:embed sketches/v4_system_prompt.txt
var v4SystemPrompt string

//go:embed sketches/v4_tool_def.json
var v4ToolDefRaw []byte

//go:embed sketches/v5_system_prompt.txt
var v5SystemPrompt string

//go:embed sketches/v5_tool_def.json
var v5ToolDefRaw []byte

//go:embed sketches/fields_block.txt
var fieldsBlock string

//go:embed sketches/v4_tree_map.json
var v4TreeMapRaw string

//go:embed sketches/v5_tree_map.json
var v5TreeMapRaw string

const (
	// model picks an Anthropic model id supported by count_tokens. Haiku 4.5
	// is the production default per CLAUDE.md.
	model = "claude-haiku-4-5-20251001"
	// userQuery is the realistic user-side input for the scenario.
	userQuery = "Show me 3 products."

	// Anthropic Haiku 4.5 pricing (USD per million tokens):
	//   1.00 input, 5.00 output, 1.25× write, 0.10× read.
	// We work in "charge units" = USD × 1e6 = tokens × multiplier so the
	// numbers stay readable at single-turn granularity.
	priceInputPerMTok      = 1.00
	priceCacheWriteFactor  = 1.25
	priceCacheReadFactor   = 0.10

	// Practical cache-prefix threshold tracked by Vlad in V4 production.
	// Below this, prompt caching is unstable (Anthropic's documented Haiku
	// minimum is 2048, but real-world stable behaviour wants ≥ 4500).
	stableCacheThreshold = 4500
)

// buildSystem concats prompt body + a fields block + a tree_map block. The
// tree_map only enters Agent2 in modify mode; we include it in both V4 and
// V5 measurement so we capture the cost of the "what's currently on screen"
// hint that a typical second-turn call carries.
func buildSystem(prompt, treeMap string) string {
	return prompt + "\n\n" + fieldsBlock + "\n\n<formation_tree>\n" + treeMap + "\n</formation_tree>\n"
}

func mustParseTool(t *testing.T, raw []byte) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal tool def: %v", err)
	}
	return out
}

// turnCost returns the effective USD-µ (USD × 1e6, i.e. tokens × pricing
// factor) charge for one turn given the cache-prefix size, total input
// size, and whether this turn writes the cache (turn 1) or reads it (turn
// 2+ within 5-min TTL). The "stable" arg gates whether the cache actually
// activates: below stableCacheThreshold we conservatively assume no caching.
func turnCost(prefixTokens, totalTokens int, isFirstTurn bool, stable bool) float64 {
	uncached := totalTokens - prefixTokens
	if !stable {
		// Cache below threshold — pay full input price every turn.
		return float64(totalTokens) * priceInputPerMTok
	}
	if isFirstTurn {
		// Cache write: prefix billed at 1.25×, rest at 1×.
		return float64(prefixTokens)*priceInputPerMTok*priceCacheWriteFactor +
			float64(uncached)*priceInputPerMTok
	}
	// Cache read: prefix billed at 0.10×, rest at 1×.
	return float64(prefixTokens)*priceInputPerMTok*priceCacheReadFactor +
		float64(uncached)*priceInputPerMTok
}

// TestTokenComparisonV4vsV5 logs input_tokens for both shapes and effective
// economic cost over a representative multi-turn conversation, given the
// Anthropic prompt-caching pricing. Skips when ANTHROPIC_API_KEY is absent
// so it never fails CI.
//
// Two cost regimes:
//   - First turn:    cache write (1.25× on prefix)
//   - Subsequent:    cache read  (0.10× on prefix), only if prefix ≥ stable
//                    threshold; otherwise full price every turn (no caching)
//
// "Prefix" = system prompt + tools (the chat-runtime puts cache_control on
// these blocks per V4 prod behaviour; fields block + tree_map vary per turn
// and are NOT cached).
func TestTokenComparisonV4vsV5(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set — skipping live token measurement")
	}

	v4Tool := mustParseTool(t, v4ToolDefRaw)
	v5Tool := mustParseTool(t, v5ToolDefRaw)
	msgs := []CountMessage{{Role: "user", Content: userQuery}}

	// Full input per turn.
	v4Total, err := CountInputTokens(apiKey, model,
		buildSystem(v4SystemPrompt, v4TreeMapRaw),
		[]map[string]interface{}{v4Tool},
		msgs,
	)
	if err != nil {
		t.Fatalf("V4 count_tokens: %v", err)
	}
	v5Total, err := CountInputTokens(apiKey, model,
		buildSystem(v5SystemPrompt, v5TreeMapRaw),
		[]map[string]interface{}{v5Tool},
		msgs,
	)
	if err != nil {
		t.Fatalf("V5 count_tokens: %v", err)
	}

	// Cached prefix = system prompt + tools only (no fields, no tree_map).
	v4Prefix, err := CountInputTokens(apiKey, model, v4SystemPrompt,
		[]map[string]interface{}{v4Tool}, msgs)
	if err != nil {
		t.Fatalf("V4 prefix count: %v", err)
	}
	v5Prefix, err := CountInputTokens(apiKey, model, v5SystemPrompt,
		[]map[string]interface{}{v5Tool}, msgs)
	if err != nil {
		t.Fatalf("V5 prefix count: %v", err)
	}

	v4CacheStable := v4Prefix >= stableCacheThreshold
	v5CacheStable := v5Prefix >= stableCacheThreshold

	const conversationTurns = 10
	v4Cost := turnCost(v4Prefix, v4Total, true, v4CacheStable)
	v5Cost := turnCost(v5Prefix, v5Total, true, v5CacheStable)
	for i := 1; i < conversationTurns; i++ {
		v4Cost += turnCost(v4Prefix, v4Total, false, v4CacheStable)
		v5Cost += turnCost(v5Prefix, v5Total, false, v5CacheStable)
	}

	delta := v5Total - v4Total
	pct := float64(delta) / float64(v4Total) * 100
	costDelta := v5Cost - v4Cost
	costPct := costDelta / v4Cost * 100

	t.Logf("===== Token measurement: 'show me 3 products' =====")
	t.Logf("Per-turn input_tokens:")
	t.Logf("  V4: %5d total | %5d cacheable prefix (system+tools) | stable cache: %v", v4Total, v4Prefix, v4CacheStable)
	t.Logf("  V5: %5d total | %5d cacheable prefix (system+tools) | stable cache: %v", v5Total, v5Prefix, v5CacheStable)
	t.Logf("  Δ raw input tokens: %+d (%+.1f%%)", delta, pct)
	t.Logf("")
	t.Logf("Effective cost over %d-turn conversation (USD-µ):", conversationTurns)
	t.Logf("  V4: %.0f", v4Cost)
	t.Logf("  V5: %.0f", v5Cost)
	t.Logf("  Δ effective cost: %+.0f (%+.1f%%)", costDelta, costPct)
	t.Logf("")
	t.Logf("Stable-cache threshold check: %d tokens", stableCacheThreshold)
	if !v5CacheStable {
		t.Logf("  ⚠ V5 cacheable prefix %d < %d — caching unstable; V5 pays full price every turn.", v5Prefix, stableCacheThreshold)
		t.Logf("  ⚠ The naive token-count win disappears once cache economics are factored in.")
		t.Logf("  ⚠ Action: chunk-6b prompt-builder must grow the V5 prefix above the threshold")
		t.Logf("  ⚠ (V4 examples + decision rules + field-binding playbook are the natural source).")
	}
	t.Logf("")
	t.Logf("Sketches:")
	t.Logf("  v4 system prompt: %d bytes", len(v4SystemPrompt))
	t.Logf("  v5 system prompt: %d bytes", len(v5SystemPrompt))
	t.Logf("  v4 tool def: %d bytes", len(v4ToolDefRaw))
	t.Logf("  v5 tool def: %d bytes", len(v5ToolDefRaw))
	t.Logf("  fields block (shared): %d bytes", len(fieldsBlock))
	t.Logf("  v4 tree_map: %d bytes", len(v4TreeMapRaw))
	t.Logf("  v5 tree_map: %d bytes", len(v5TreeMapRaw))
	t.Logf("===================================================")
}

// TestSystemAndToolBreakdown — separate measurements so we can attribute
// the delta. Helpful when the v5 prompt evolves and we want to know if
// the change was in the prompt, the tool def, or the tree_map.
func TestSystemAndToolBreakdown(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set — skipping")
	}
	v4Tool := mustParseTool(t, v4ToolDefRaw)
	v5Tool := mustParseTool(t, v5ToolDefRaw)
	msgs := []CountMessage{{Role: "user", Content: userQuery}}

	type breakdown struct {
		name   string
		system string
		tool   map[string]interface{}
	}
	cases := []breakdown{
		{"V4 prompt only (no tool)", v4SystemPrompt, nil},
		{"V4 prompt + tool", v4SystemPrompt, v4Tool},
		{"V4 prompt + tool + fields", v4SystemPrompt + "\n\n" + fieldsBlock, v4Tool},
		{"V4 full (prompt + tool + fields + tree_map)", buildSystem(v4SystemPrompt, v4TreeMapRaw), v4Tool},
		{"V5 prompt only (no tool)", v5SystemPrompt, nil},
		{"V5 prompt + tool", v5SystemPrompt, v5Tool},
		{"V5 prompt + tool + fields", v5SystemPrompt + "\n\n" + fieldsBlock, v5Tool},
		{"V5 full (prompt + tool + fields + tree_map)", buildSystem(v5SystemPrompt, v5TreeMapRaw), v5Tool},
	}

	for _, c := range cases {
		var tools []map[string]interface{}
		if c.tool != nil {
			tools = []map[string]interface{}{c.tool}
		}
		n, err := CountInputTokens(apiKey, model, c.system, tools, msgs)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		t.Logf("%-50s %5d tokens", c.name, n)
	}
}

// TestAgent1PrefixSize logs the size of V5 Agent1's cacheable prefix —
// system prompt (no digest, no tenant) + Agent1's 3 tool definitions. No
// hard gate this chunk; we just want a number to track. Vlad's V4-prod
// experience says ≥ 4500 tokens for stable Haiku caching; if Agent1's bare
// prefix is below that, the first turn likely misses cache and only later
// turns benefit (once <conversation_history> grows past the threshold).
//
// The digest block adds 300-400 tokens per tenant — when the digest is
// present, prefix should clear ≥ 4500 for catalogs with non-trivial
// taxonomy. This test runs without DB so the digest portion is omitted.
func TestAgent1PrefixSize(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set — skipping")
	}

	type agent1Tool struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		InputSchema map[string]interface{} `json:"input_schema"`
	}

	// Build the three Agent1 tool defs inline so we don't import V5 packages
	// in the tokens-tag-only build (avoids cycle risk).
	tools := []map[string]interface{}{
		mustParseTool(t, []byte(catalogSearchSchema)),
		mustParseTool(t, []byte(stateFilterSchema)),
		mustParseTool(t, []byte(historyLookupSchema)),
	}

	msgs := []CountMessage{{Role: "user", Content: "Покажи 3 продукта"}}

	n, err := CountInputTokens(apiKey, model, agent1SystemPrompt, tools, msgs)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	t.Logf("Agent1 bare prefix (system + 3 tool defs, no digest): %d tokens", n)

	const stableCacheBar = 4500
	if n < stableCacheBar {
		t.Logf("⚠️  prefix %d < %d (stable-cache bar). Turn-1 cache likely misses; turn-N hits as conversation grows.", n, stableCacheBar)
	} else {
		t.Logf("✓ prefix ≥ %d — clears stable-cache bar.", stableCacheBar)
	}
}

// agent1SystemPrompt copies the static base from prompts.Agent1SystemPrompt.
// We can't import prompts here (build-tag isolation), so duplicate the
// string. Drift between this copy and the source is OK — token count is a
// rough gauge, and the test reports the gap for a human to act on.
const agent1SystemPrompt = `You are Agent 1 - a data retrieval agent for an e-commerce chat.

Your job: call catalog_search when user needs NEW data. If the user is asking about STYLE or DISPLAY (not new data), do nothing.

Rules:

## CRITICAL: FILTER vs SEARCH decision (check FIRST)
When loaded_products > 0 in <state>:
- User wants SUBSET of current data → _internal_state_filter (NOT catalog_search!)
- User wants DIFFERENT/NEW data → catalog_search
Subset triggers: "только X", "лишь X", "оставь X", "дешевле N", "дороже N", "с рейтингом выше N"
NEVER filter triggers — these are STYLE/DISPLAY requests, NOT data filters:
  "убери/покажи/добавь/без" + FIELD NAME (описание, рейтинг, бренд, цена, фото, название, теги, категория) → DO NOT call any tool

## Other rules
1. If user asks for products → call catalog_search
2. catalog_search has two inputs:
   - filters: exact match filters. Use enum values from <catalog> block.
   - vector_query: semantic search in user's ORIGINAL language. Do NOT translate.
3. Match user intent to exact filter values from <catalog> → filters.{key}. Everything else → vector_query.
4. Prices are in RUBLES.
5. If user asks to CHANGE DISPLAY STYLE → DO NOT call any tool. Just stop.
6. Do NOT explain. Do NOT ask questions. Make best guess.
7. After getting "ok"/"empty", stop. Do not call more tools.
`

// catalogSearchSchema, stateFilterSchema, historyLookupSchema are JSON
// snapshots of the V5 Agent1 tool defs. Kept inline so this test stays
// hermetic from the production code path. Update when the schemas change.
const catalogSearchSchema = `{
  "name": "catalog_search",
  "description": "Catalog search for products. Put structured/exact filters in 'filters'. Put semantic search intent in 'vector_query' in user's original language.",
  "input_schema": {
    "type": "object",
    "required": ["vector_query"],
    "properties": {
      "vector_query": {"type": "string", "description": "Semantic search in user's original language."},
      "filters": {
        "type": "object",
        "description": "Exact filters. Only include filters you're confident about.",
        "properties": {
          "brand": {"type": "string"},
          "category": {"type": "string"},
          "min_price": {"type": "number"},
          "max_price": {"type": "number"},
          "product_form": {"type": "string", "enum": ["cream", "gel", "serum", "toner", "essence", "lotion", "oil", "balm", "foam", "mousse", "mist", "spray", "powder", "stick", "patch", "sheet-mask", "wash-off-mask", "peel", "scrub", "soap"]},
          "skin_type": {"type": "string", "enum": ["normal", "dry", "oily", "combination", "sensitive", "acne-prone", "mature"]},
          "concern": {"type": "string", "enum": ["hydration", "anti-aging", "brightening", "acne", "pores", "dark-spots", "redness", "sun-protection", "exfoliation", "firmness", "dark-circles", "lip-dryness", "oil-control", "texture", "dullness"]},
          "key_ingredient": {"type": "string", "enum": ["hyaluronic-acid", "niacinamide", "retinol", "vitamin-c", "salicylic-acid", "glycolic-acid", "centella-asiatica", "ceramides", "peptides", "snail-mucin", "tea-tree", "aloe-vera", "collagen", "aha-bha", "squalane", "shea-butter", "argan-oil", "rice-extract", "green-tea", "propolis", "mugwort", "panthenol", "zinc", "turmeric", "charcoal"]},
          "routine_step": {"type": "string", "enum": ["cleansing", "toning", "exfoliation", "treatment", "moisturizing", "sun-protection", "makeup"]},
          "texture": {"type": "string", "enum": ["watery", "gel", "milky", "creamy", "thick", "oily", "powdery", "foamy", "balmy"]},
          "target_area": {"type": "string", "enum": ["face", "eye-area", "lips", "neck", "body", "hands", "feet", "scalp"]}
        }
      },
      "sort_by": {"type": "string", "enum": ["price", "rating", "name"]},
      "sort_order": {"type": "string", "enum": ["asc", "desc"]},
      "limit": {"type": "integer"}
    }
  }
}`

const stateFilterSchema = `{
  "name": "_internal_state_filter",
  "description": "Filter already loaded products in state. Use when user wants a SUBSET of existing data.",
  "input_schema": {
    "type": "object",
    "properties": {
      "brand": {"type": "string"},
      "category": {"type": "string"},
      "min_price": {"type": "number"},
      "max_price": {"type": "number"},
      "min_rating": {"type": "number"},
      "text_match": {"type": "string"}
    }
  }
}`

const historyLookupSchema = `{
  "name": "_internal_history_lookup",
  "description": "Search session history deltas.",
  "input_schema": {
    "type": "object",
    "properties": {
      "query": {"type": "string"},
      "last_n": {"type": "integer"}
    }
  }
}`
