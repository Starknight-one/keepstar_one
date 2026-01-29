package prompts

// Agent 1: Query Analysis Prompt

const AnalyzeQuerySystemPrompt = `You are a query analyzer for an e-commerce chat widget.
Your job is to understand user intent and extract search parameters.

Output JSON only.`

const AnalyzeQueryUserTemplate = `User query: {{.Query}}

Extract:
- intent: product_search | product_info | comparison | general_question
- search_params: relevant filters (category, price_range, brand, etc.)

JSON response:`

// BuildAnalyzeQueryPrompt builds the prompt for query analysis
func BuildAnalyzeQueryPrompt(query string) string {
	// TODO: implement template substitution
	return ""
}
