package anthropic

import (
	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"keepstar_v5/internal/ports"
)

// Default model when the caller doesn't pin one. Haiku 4.5 is V5's
// production target per CLAUDE.md.
const DefaultModel = "claude-haiku-4-5"

// Default MaxTokens for a Messages.New call. Agent2's typical tool_use
// response is ~200-500 tokens; 4096 leaves headroom for free-form
// fallback responses without ever clipping.
const DefaultMaxTokens int64 = 4096

// Client wraps the SDK MessageService and adds the model id + pricing
// table. Implements ports.LLMPort.
//
// Construction: NewClient(apiKey, model). Empty model → DefaultModel.
type Client struct {
	sdk       sdk.Client
	model     string
	maxTokens int64
}

// Compile-time check that Client implements LLMPort.
var _ ports.LLMPort = (*Client)(nil)

// NewClient returns a configured adapter. Empty apiKey falls back to the
// SDK's environment-variable lookup (`ANTHROPIC_API_KEY`).
func NewClient(apiKey, model string) *Client {
	var opts []option.RequestOption
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	if model == "" {
		model = DefaultModel
	}
	return &Client{
		sdk:       sdk.NewClient(opts...),
		model:     model,
		maxTokens: DefaultMaxTokens,
	}
}

// Model returns the model id this client targets.
func (c *Client) Model() string { return c.model }
