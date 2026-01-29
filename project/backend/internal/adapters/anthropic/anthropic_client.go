package anthropic

import (
	"context"

	"keepstar/internal/ports"
)

// Client implements ports.LLMPort for Anthropic API
type Client struct {
	apiKey string
	model  string
}

// NewClient creates a new Anthropic client
func NewClient(apiKey, model string) *Client {
	return &Client{
		apiKey: apiKey,
		model:  model,
	}
}

// AnalyzeQuery implements LLMPort.AnalyzeQuery
func (c *Client) AnalyzeQuery(ctx context.Context, req ports.AnalyzeQueryRequest) (*ports.AnalyzeQueryResponse, error) {
	// TODO: implement
	return nil, nil
}

// ComposeWidgets implements LLMPort.ComposeWidgets
func (c *Client) ComposeWidgets(ctx context.Context, req ports.ComposeWidgetsRequest) (*ports.ComposeWidgetsResponse, error) {
	// TODO: implement
	return nil, nil
}
