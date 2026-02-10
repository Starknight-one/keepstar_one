package ports

import "context"

type EmbeddingPort interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
