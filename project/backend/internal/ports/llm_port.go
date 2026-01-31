package ports

import "context"

// LLMPort defines the interface for LLM providers
type LLMPort interface {
	// Chat sends a message to LLM and returns the response
	Chat(ctx context.Context, message string) (string, error)
}
