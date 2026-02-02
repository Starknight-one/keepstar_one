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
}
