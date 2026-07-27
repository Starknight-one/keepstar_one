package prompts

import (
	"encoding/json"
	"fmt"

	"keepstar_v5/internal/domain"
)

// Agent1SystemPrompt is the static base for Agent1's system prompt. The
// per-tenant <catalog> digest block is appended by the use case before
// each LLM call (see usecases.Agent1PromptCache).
//
// Verbatim port of V4's prompts.Agent1SystemPrompt — same rules, same
// FILTER-vs-SEARCH decision tree, same NEVER triggers (style requests).
const Agent1SystemPrompt = `You are Agent 1 - a data retrieval agent for an e-commerce chat.

Your job: call catalog_search when user needs NEW data. If the user is asking about STYLE or DISPLAY (not new data), do nothing.

Rules:

## CRITICAL: FILTER vs SEARCH decision (check FIRST)
When loaded_products > 0 in <state>:
- User wants SUBSET of current data → _internal_state_filter (NOT catalog_search!)
- User wants DIFFERENT/NEW data → catalog_search
Subset triggers: "только X", "лишь X", "оставь X", "дешевле N", "дороже N", "с рейтингом выше N"
NEVER filter triggers — these are STYLE/DISPLAY requests, NOT data filters:
  "убери/покажи/добавь/без" + FIELD NAME (описание, рейтинг, бренд, цена, фото, название, теги, категория) → DO NOT call any tool
Examples:
  - loaded_products=20, "только COSRX" → _internal_state_filter(brand:"COSRX")
  - loaded_products=20, "дешевле 5000" → _internal_state_filter(max_price:5000)
  - loaded_products=20, "покажи сыворотки" → catalog_search (DIFFERENT data)
  - loaded_products=0, "только COSRX" → catalog_search (nothing to filter)
  - loaded_products=20, "убери описание" → STYLE request, DO NOT call tool
  - loaded_products=20, "покажи с рейтингом" → STYLE request, DO NOT call tool
  - loaded_products=20, "без цены" → STYLE request, DO NOT call tool

## Other rules
1. If user asks for products → call catalog_search
2. catalog_search has two inputs:
   - filters: exact match filters. Use enum values from <catalog> block.
   - vector_query: semantic search in user's ORIGINAL language. Do NOT translate.
3. Match user intent to exact filter values from <catalog> → filters.{key}. Everything else → vector_query.
4. Prices are in USD (dollars). "under $100" → filters.max_price: 100. Prices you see in <state> are also dollars.
5. If user asks to CHANGE DISPLAY STYLE → DO NOT call any tool. Just stop.
6. Do NOT explain. Do NOT ask questions. Make best guess.
7. After getting "ok"/"empty", stop. Do not call more tools.
8. <state> block = current data on screen:
   - loaded_products > 0 → data exists, maybe no search needed
   - rendered = the products the user can see RIGHT NOW. Use these to ground references like "the first one", "the COSRX one", "the one with the snake on the photo". Match by id/name/brand or, when needed, look at the image URLs.
   - If user asks about fields already displayed → style request, DO NOT call tool
   - If user asks for DIFFERENT data → call catalog_search
9. <catalog> block = available filter values:
   - Use EXACT category slugs from the tree
   - Use EXACT enum values for filters (skin_type, concern, product_form, etc.)
   - Unknown values or broad queries → vector_query only
   - Broad request ("для сухой кожи", "подарок") → do NOT set category, use vector_query + relevant filters
`

// RenderedItem is the per-product entry shipped in the <state> block. Carries
// only what's actually visible on the user's screen so Agent1 can resolve
// natural-language references without re-searching. Empty fields are dropped
// at marshal time via omitempty so an unrated product doesn't waste tokens
// on `"rating":0`.
type RenderedItem struct {
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	Brand string `json:"brand,omitempty"`
	// Price is in MAJOR units (dollars) — the storage layer keeps cents,
	// but the LLM's whole price vocabulary (tool filters, user queries)
	// is dollars, so <state> must speak dollars too or the model carries
	// a silent 100× mismatch.
	Price          float64  `json:"price,omitempty"`
	Rating         float64  `json:"rating,omitempty"`
	Images         []string `json:"images,omitempty"`
	MarketingClaim string   `json:"marketing_claim,omitempty"`
}

// BuildAgent1ContextPrompt enriches the user query with the «what's loaded
// and what's rendered» state. Returns the raw query when there is nothing
// to advertise (cold session — fewer tokens, identical semantics).
//
// What lands in <state>:
//   - loaded_products: total count of products available in state (search
//     pool, not just visible).
//   - rendered: per-card subset of products currently on screen. Derived
//     by the caller from engine.RenderedDataIndices(state.Current.Template)
//     and indexed back into state.Current.Data.Products. Omitted when the
//     template has no replicate clones (text_explainer, error states, fresh
//     session).
//
// V4's `liked_count` / `liked_ids` / `cart_count` and the `available_fields`
// schema list are intentionally absent — the visible cards already convey
// fields, and likes/cart proved noise in practice.
func BuildAgent1ContextPrompt(meta domain.StateMeta, rendered []RenderedItem, userQuery string) string {
	hasData := meta.ProductCount > 0
	hasRendered := len(rendered) > 0
	if !hasData && !hasRendered {
		return userQuery
	}

	stateInfo := map[string]interface{}{}
	if meta.ProductCount > 0 {
		stateInfo["loaded_products"] = meta.ProductCount
	}
	if hasRendered {
		stateInfo["rendered"] = rendered
	}

	jsonBytes, _ := json.Marshal(stateInfo)
	return fmt.Sprintf("<state>\n%s\n</state>\n\n%s", string(jsonBytes), userQuery)
}

// BuildRenderedSubset projects state.Current.Data.Products[indices...] into
// the compact RenderedItem shape Agent1 sees. Indices outside the products
// slice are silently skipped (defensive — out-of-band template/data
// mismatches shouldn't crash Agent1 prep).
func BuildRenderedSubset(products []domain.Product, indices []int) []RenderedItem {
	if len(products) == 0 || len(indices) == 0 {
		return nil
	}
	out := make([]RenderedItem, 0, len(indices))
	for _, idx := range indices {
		if idx < 0 || idx >= len(products) {
			continue
		}
		p := products[idx]
		out = append(out, RenderedItem{
			ID:             p.ID,
			Name:           p.Name,
			Brand:          p.Brand,
			Price:          float64(p.Price) / 100, // cents → dollars for the LLM
			Rating:         p.Rating,
			Images:         p.Images,
			MarketingClaim: p.MarketingClaim,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
