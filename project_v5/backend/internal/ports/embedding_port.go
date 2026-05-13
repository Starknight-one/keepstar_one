package ports

import "context"

// EmbeddingPort generates vector embeddings for text. Used by catalog_search
// to power the semantic side of hybrid retrieval (keyword SQL + pgvector
// cosine + RRF merge). Implementations: OpenAI Embeddings API today; could
// be replaced with a local model server without touching call sites.
//
// Batch shape mirrors V4: pass N strings, get N vectors back, vectors are
// indexed in the same order. Caller decides batch sizing — adapter does
// not chunk.
type EmbeddingPort interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
