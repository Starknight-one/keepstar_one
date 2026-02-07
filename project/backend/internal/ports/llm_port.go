package ports

import (
	"context"

	"keepstar/internal/domain"
)

// LLMPort defines the interface for LLM providers
type LLMPort interface {
	// Chat sends a message to LLM and returns the response
	Chat(ctx context.Context, message string) (string, error)

	// ChatWithTools sends messages with tool definitions, returns potential tool calls
	ChatWithTools(
		ctx context.Context,
		systemPrompt string,
		messages []domain.LLMMessage,
		tools []domain.ToolDefinition,
	) (*domain.LLMResponse, error)

	// ChatWithToolsCached sends messages with prompt caching enabled
	ChatWithToolsCached(
		ctx context.Context,
		systemPrompt string,
		messages []domain.LLMMessage,
		tools []domain.ToolDefinition,
		cacheConfig *CacheConfig,
	) (*domain.LLMResponse, error)

	// ChatWithUsage sends a message with system prompt and returns response with usage stats
	ChatWithUsage(ctx context.Context, systemPrompt, userMessage string) (*ChatResponse, error)
}

// ChatResponse contains both text and usage info
type ChatResponse struct {
	Text  string
	Usage domain.LLMUsage
}

// CacheConfig controls prompt caching behavior
type CacheConfig struct {
	CacheTools        bool   // cache tool definitions
	CacheSystem       bool   // cache system prompt
	CacheConversation bool   // cache conversation history
	ToolChoice        string // "auto" (default), "any" (force tool use), or "tool:name" (force specific tool)
}
