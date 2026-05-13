// Package ports defines outbound interfaces for V5's hexagonal architecture.
package ports

import (
	"context"

	"keepstar_v5/internal/domain"
)

// CacheConfig drives where the adapter places `cache_control: ephemeral`
// hints on a Messages call. Three independent flags + the tool-choice
// override; matches V4's CacheConfig surface so existing prod knowledge
// transfers.
//
// Placement rules (V4-proven, applied verbatim by V5 adapter):
//   - CacheTools=true        → mark the LAST tool in the tools array
//   - CacheSystem=true       → mark the system block
//   - CacheConversation=true → mark the SECOND-TO-LAST message
//                              (the most-recent assistant turn before the
//                              new user message; that's the longest stable
//                              prefix the cache can match)
//
// ToolChoice values:
//   - ""          → SDK default ("auto")
//   - "auto"      → Anthropic's auto choice
//   - "any"       → must call SOME tool, model picks which
//   - "tool:NAME" → must call NAME (e.g. "tool:visual_assembly")
type CacheConfig struct {
	CacheTools        bool
	CacheSystem       bool
	CacheConversation bool
	ToolChoice        string
}

// LLMPort is V5's chat-runtime LLM surface. Two methods, both used by
// Agent2: the cached chat-with-tools path and the stand-alone
// token-counting path (for measurement and cache-prefix gating).
//
// Other V4 LLMPort methods (Chat / ChatWithTools / ChatWithUsage) were
// deliberately not ported — production Agent2 only uses the cached
// variant.
type LLMPort interface {
	// ChatWithToolsCached calls the Messages API with prompt-caching hints
	// applied per cfg. Returns parsed text + tool calls + token usage.
	ChatWithToolsCached(
		ctx context.Context,
		systemPrompt string,
		messages []domain.LLMMessage,
		tools []domain.ToolDefinition,
		cfg CacheConfig,
	) (*domain.LLMResponse, error)

	// CountInputTokens calls the count_tokens endpoint with the same input
	// shape (system + tools + messages). Used by the chunk-5.5 measurement
	// test and by Agent2 to gate prompt growth against the stable-cache
	// threshold (≥ 4500 tokens).
	CountInputTokens(
		ctx context.Context,
		systemPrompt string,
		messages []domain.LLMMessage,
		tools []domain.ToolDefinition,
	) (int, error)
}
