package usecases

import (
	"context"

	"keepstar/internal/ports"
)

// SendMessageUseCase handles simple chat message sending
type SendMessageUseCase struct {
	llm ports.LLMPort
}

// NewSendMessageUseCase creates a new use case
func NewSendMessageUseCase(llm ports.LLMPort) *SendMessageUseCase {
	return &SendMessageUseCase{llm: llm}
}

// Execute sends a message and returns the response
func (uc *SendMessageUseCase) Execute(ctx context.Context, message string) (string, error) {
	return uc.llm.Chat(ctx, message)
}
