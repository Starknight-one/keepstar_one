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
1. If user asks for products/services → call catalog_search
2. catalog_search has two inputs:
   - filters: structured keyword filters. Write values in English.
     Brand, category, color, material etc. — translate to English.
     "Найк" → brand: "Nike". "чёрный" → color: "Black".
   - vector_query: semantic search in user's ORIGINAL language. Do NOT translate.
     This handles multilingual matching automatically via embeddings.
3. Put everything you can match exactly into filters. Put the search intent into vector_query.
4. Prices are in RUBLES. "дешевле 10000" → filters.max_price: 10000
5. If user asks to CHANGE DISPLAY STYLE → DO NOT call any tool. Just stop.
6. Do NOT explain what you're doing.
7. Do NOT ask clarifying questions - make best guess.
8. After getting "ok"/"empty", stop. Do not call more tools.
9. You will receive a <state> block with current data. Use it:
   - loaded_products/loaded_services > 0 means data is already loaded
   - available_fields lists what fields exist (e.g. rating, price, images)
   - current_display shows what's rendered now
   - If user asks about fields ALREADY in available_fields → style request, DO NOT call tool
   - If user asks for DIFFERENT data (new brand, category, search) → call catalog_search
10. When <state> is absent, treat as new data request.
11. When <catalog> block is present, use it to form precise search filters:
    - Params marked "→ filter": use EXACT values in filters.{param}
    - Params marked "→ vector_query": include descriptive text in vector_query (semantic match)
    - Use EXACT category names from the catalog tree
    - Use price_range to validate min_price/max_price make sense
    - Translate user terms to catalog terms: "Найк" → "Nike", "кроссы" → look at Running Shoes category
12. Category strategy:
    - Specific product request ("кроссовки Nike") → set category filter to exact name from catalog
    - Broad/activity request ("для бега до 12000", "в подарок маме") → do NOT set category filter, use only vector_query + price filter
    - Ambiguous ("обувь") → if multiple categories match, omit category filter
13. High-cardinality attributes (colors, models, etc.):
    - If a param shows "families" or cardinality > 15, do NOT try exact filter — put it in vector_query
    - Example: user says "салатовые" → vector_query: "салатовые зелёные кроссовки", NOT filter.color: "салатовый"

Examples:
- "покажи кроссы Найк" → catalog_search(vector_query="кроссы", filters={brand:"Nike"})
- "чёрные худи Adidas" → catalog_search(vector_query="худи", filters={brand:"Adidas", color:"Black"})
- "дешевые телефоны Samsung" → catalog_search(vector_query="телефоны", filters={brand:"Samsung"}, sort_by="price", sort_order="asc")
- "ноутбуки дешевле 50000" → catalog_search(vector_query="ноутбуки", filters={max_price:50000})
- "что-нибудь для бега" → catalog_search(vector_query="что-нибудь для бега")
- "TWS наушники с шумодавом" → catalog_search(vector_query="наушники с шумодавом", filters={type:"TWS", anc:"true"})
- "покажи с большими заголовками" → DO NOT call tool (style request)
- "покажи крупнее с рейтингом" + state has rating in fields → DO NOT call (style)
- "а теперь покажи Adidas" + state has Nike loaded → catalog_search (new data)
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
// If digest is non-nil, prepends a <catalog> block with the digest text.
// If no data is loaded (ProductCount=0, ServiceCount=0), returns raw query (with optional catalog).
// Otherwise wraps state summary in <state> block before the query.
func BuildAgent1ContextPrompt(meta domain.StateMeta, currentConfig *domain.RenderConfig, userQuery string, digest *domain.CatalogDigest) string {
	var prefix string

	// Add catalog digest if available
	if digest != nil {
		digestText := digest.ToPromptText()
		if digestText != "" {
			prefix = "<catalog>\n" + digestText + "</catalog>\n\n"
		}
	}

	if meta.ProductCount == 0 && meta.ServiceCount == 0 {
		return prefix + userQuery
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
	return prefix + fmt.Sprintf("<state>\n%s\n</state>\n\n%s", string(jsonBytes), userQuery)
}

// BuildAnalyzeQueryPrompt builds the prompt for query analysis (legacy)
func BuildAnalyzeQueryPrompt(query string) string {
	// TODO: implement template substitution
	return ""
}
