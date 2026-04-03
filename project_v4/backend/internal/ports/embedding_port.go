package ports

import "context"

// EmbeddingPort generates vector embeddings for text.
// Implementations: OpenAI API, local model server, etc.
type EmbeddingPort interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
