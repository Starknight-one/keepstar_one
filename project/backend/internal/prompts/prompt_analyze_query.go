package prompts

import (
	"encoding/json"
	"fmt"

	"keepstar/internal/domain"
)

// Agent1SystemPrompt is the system prompt for Agent 1 (Data Retrieval)
const Agent1SystemPrompt = `You are Agent 1 - a data retrieval agent for an e-commerce chat.

Your job: call catalog_search when user needs NEW data. If the user is asking about STYLE or DISPLAY (not new data), do nothing.

Rules:

## CRITICAL: FILTER vs SEARCH decision (check FIRST)
When loaded_products > 0 in <state>:
- User wants SUBSET of current data → _internal_state_filter (NOT catalog_search!)
- User wants DIFFERENT/NEW data → catalog_search
Subset triggers: "только X", "лишь X", "оставь X", "убери Y", "дешевле N", "дороже N", "с рейтингом выше N"
Examples:
  - loaded_products=20, "только COSRX" → _internal_state_filter(brand:"COSRX")
  - loaded_products=20, "дешевле 5000" → _internal_state_filter(max_price:5000)
  - loaded_products=20, "покажи сыворотки" → catalog_search (DIFFERENT data)
  - loaded_products=0, "только COSRX" → catalog_search (nothing to filter)

## Other rules
1. If user asks for products/services → call catalog_search
2. catalog_search has two inputs:
   - filters: exact match filters. Use enum values from <catalog> block.
   - vector_query: semantic search in user's ORIGINAL language. Do NOT translate.
3. Match user intent to exact filter values from <catalog> → filters.{key}. Everything else → vector_query.
4. Prices are in RUBLES. "дешевле 10000" → filters.max_price: 10000
5. If user asks to CHANGE DISPLAY STYLE → DO NOT call any tool. Just stop.
6. Do NOT explain. Do NOT ask questions. Make best guess.
7. After getting "ok"/"empty", stop. Do not call more tools.
8. <state> block = current data on screen:
   - loaded_products > 0 → data exists, maybe no search needed
   - If user asks about fields already displayed → style request, DO NOT call tool
   - If user asks for DIFFERENT data → call catalog_search
9. <catalog> block = available filter values:
   - Use EXACT category slugs from the tree
   - Use EXACT enum values for filters (skin_type, concern, product_form, etc.)
   - Unknown values or broad queries → vector_query only
   - Broad request ("для сухой кожи", "подарок") → do NOT set category, use vector_query + relevant filters
`

// Legacy prompts (kept for backward compatibility)

// AnalyzeQuerySystemPrompt is the legacy system prompt for query analysis
const AnalyzeQuerySystemPrompt = `You are a query analyzer for an e-commerce chat widget.
Your job is to understand user intent and extract search parameters.

Output JSON only.`

// AnalyzeQueryUserTemplate is the legacy user template for query analysis
const AnalyzeQueryUserTemplate = `User query: {{.Query}}

Extract:
- intent: product_search | product_info | comparison | general_question
- search_params: relevant filters (category, price_range, brand, etc.)

JSON response:`

// BuildAgent1ContextPrompt enriches the user query with current state context.
// Catalog digest is already in conversation_history from session init — not injected here.
// If no data is loaded (ProductCount=0, ServiceCount=0), returns raw query.
// Otherwise wraps state summary in <state> block before the query.
func BuildAgent1ContextPrompt(meta domain.StateMeta, currentConfig *domain.RenderConfig, userQuery string) string {
	if meta.ProductCount == 0 && meta.ServiceCount == 0 {
		return userQuery
	}

	stateInfo := map[string]interface{}{
		"loaded_products":  meta.ProductCount,
		"loaded_services":  meta.ServiceCount,
		"available_fields": meta.Fields,
	}

	if currentConfig != nil {
		stateInfo["current_display"] = map[string]interface{}{
			"preset": currentConfig.Preset,
			"mode":   currentConfig.Mode,
			"size":   currentConfig.Size,
		}
		// Extract displayed field names
		if len(currentConfig.Fields) > 0 {
			displayed := make([]string, len(currentConfig.Fields))
			for i, f := range currentConfig.Fields {
				displayed[i] = f.Name
			}
			stateInfo["displayed_fields"] = displayed
		}
	}

	jsonBytes, _ := json.Marshal(stateInfo)
	return fmt.Sprintf("<state>\n%s\n</state>\n\n%s", string(jsonBytes), userQuery)
}

// BuildAnalyzeQueryPrompt builds the prompt for query analysis (legacy)
func BuildAnalyzeQueryPrompt(query string) string {
	// TODO: implement template substitution
	return ""
}
