package anthropic

// cacheControl specifies caching behavior for a content block
type cacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// contentBlockWithCache is a text content block that supports cache_control
type contentBlockWithCache struct {
	Type         string        `json:"type"`
	Text         string        `json:"text,omitempty"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

// contentBlockFullCache is a content block with all fields + cache_control.
// Used for marking cache_control on tool_use/tool_result blocks in conversation history.
type contentBlockFullCache struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text,omitempty"`
	ID           string                 `json:"id,omitempty"`
	Name         string                 `json:"name,omitempty"`
	Input        map[string]interface{} `json:"input,omitempty"`
	ToolUseID    string                 `json:"tool_use_id,omitempty"`
	Content      string                 `json:"content,omitempty"`
	IsError      bool                   `json:"is_error,omitempty"`
	CacheControl *cacheControl          `json:"cache_control,omitempty"`
}

// anthropicToolWithCache is a tool definition with cache_control support
type anthropicToolWithCache struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	InputSchema  map[string]interface{} `json:"input_schema"`
	CacheControl *cacheControl          `json:"cache_control,omitempty"`
}

// toolChoiceConfig represents Anthropic's tool_choice parameter
type toolChoiceConfig struct {
	Type string `json:"type"`           // "auto", "any", "tool"
	Name string `json:"name,omitempty"` // only for type="tool"
}

// anthropicCachedRequest is the request format supporting prompt caching
type anthropicCachedRequest struct {
	Model      string                   `json:"model"`
	MaxTokens  int                      `json:"max_tokens"`
	System     []contentBlockWithCache  `json:"system"`
	Messages   []anthropicToolMsg       `json:"messages"`
	Tools      []anthropicToolWithCache `json:"tools,omitempty"`
	ToolChoice *toolChoiceConfig        `json:"tool_choice,omitempty"`
}

// anthropicCachedUsage extends usage response with cache metrics
type anthropicCachedUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// anthropicCachedResponse is the response format with cache metrics
type anthropicCachedResponse struct {
	Content    []contentBlock       `json:"content"`
	StopReason string               `json:"stop_reason"`
	Usage      anthropicCachedUsage `json:"usage"`
	Error      *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

