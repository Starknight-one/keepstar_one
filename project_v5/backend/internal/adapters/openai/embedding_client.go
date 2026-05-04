package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// EmbeddingClient implements ports.EmbeddingPort against OpenAI's Embeddings
// API using raw net/http — same pattern as adapters/anthropic. Default model
// is text-embedding-3-small at 384 dims to match the existing
// `master_products.embedding vector(384)` schema.
type EmbeddingClient struct {
	apiKey string
	model  string
	dims   int
	client *http.Client
}

// NewEmbeddingClient constructs a client. Empty model/dims fall back to the
// V4 defaults (text-embedding-3-small / 384).
func NewEmbeddingClient(apiKey, model string, dims int) *EmbeddingClient {
	if model == "" {
		model = "text-embedding-3-small"
	}
	if dims == 0 {
		dims = 384
	}
	return &EmbeddingClient{
		apiKey: apiKey,
		model:  model,
		dims:   dims,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

type embeddingRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// Embed generates embeddings for the given texts via OpenAI's Embeddings API.
// Vectors are returned in input order. Empty input slice short-circuits
// without an HTTP call.
func (c *EmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := embeddingRequest{
		Model:      c.model,
		Input:      texts,
		Dimensions: c.dims,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/embeddings", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai embeddings API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var embResp embeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	result := make([][]float32, len(texts))
	for _, d := range embResp.Data {
		if d.Index < len(result) {
			result[d.Index] = d.Embedding
		}
	}
	return result, nil
}
