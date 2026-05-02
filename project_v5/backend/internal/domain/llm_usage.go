package domain

// ToolDefinition describes a tool the LLM can call. Same wire-shape as V4
// (Name + Description + JSONSchema input_schema). The adapter converts this
// to the SDK's typed ToolParam at call time.
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// LLMUsage records token + cost accounting for a single LLM call. Carries
// the four token counters Anthropic returns plus a per-call cost figure
// computed by the adapter against a per-model pricing table.
//
//   - InputTokens:               total tokens billed at full input rate
//   - OutputTokens:              total tokens billed at output rate
//   - CacheCreationInputTokens:  tokens billed at 1.25× input rate
//                                (the cache-write tax on the FIRST turn)
//   - CacheReadInputTokens:      tokens billed at 0.10× input rate
//                                (subsequent turns within 5-min TTL)
//   - CostUSD:                   total USD for this call (sum of components)
//   - Model:                     model id used for the call
type LLMUsage struct {
	InputTokens              int     `json:"input_tokens"`
	OutputTokens             int     `json:"output_tokens"`
	CacheCreationInputTokens int     `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int     `json:"cache_read_input_tokens"`
	TotalTokens              int     `json:"total_tokens"`
	CostUSD                  float64 `json:"cost_usd"`
	Model                    string  `json:"model"`
}

// LLMResponse is the engine-facing shape of a Messages.New response.
//   - Text:       concatenation of any text blocks in the response
//   - ToolCalls:  parsed tool_use blocks, in declaration order
//   - StopReason: the SDK's stop reason verbatim
//                 ("end_turn" | "tool_use" | "max_tokens" | ...)
//   - Usage:      token + cost accounting (see LLMUsage)
type LLMResponse struct {
	Text       string     `json:"text"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	StopReason string     `json:"stop_reason"`
	Usage      LLMUsage   `json:"usage"`
}
