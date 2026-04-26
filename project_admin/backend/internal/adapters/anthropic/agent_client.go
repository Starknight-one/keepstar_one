// Package anthropic — tool-use messages client for the discovery agent (M4c).
//
// Distinct from EnrichmentClient because the conversation pattern is
// completely different: enrichment is one-shot text → JSON, the agent is
// multi-turn with tool_use ↔ tool_result loops, content blocks instead of
// a single content string, and stop-reason driven flow control.
//
// Kept narrow on purpose — only what the discovery loop actually needs.
// If a future agent wants different parameters (streaming, vision, etc.)
// we add them here rather than bending EnrichmentClient.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AgentClient holds the API key + model + http client for a tool-use agent.
// Caller picks the model at construction (Sonnet 4.6 for discovery; tests
// may inject a stub via the Doer interface — see agent_doer.go pattern).
type AgentClient struct {
	apiKey string
	model  string
	http   *http.Client
}

// NewAgentClient defaults to Sonnet 4.6 — the discovery agent picks Sonnet
// because Haiku 4.5 doesn't reliably stay disciplined across 30 multi-turn
// hops with 8 distinct tools (we tested early; Haiku drifts off-task and
// burns budget). One-time per-tenant cost makes Sonnet acceptable.
func NewAgentClient(apiKey, model string) *AgentClient {
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	return &AgentClient{
		apiKey: apiKey,
		model:  model,
		// Long timeout — a single Messages call with tool_use can sit for
		// 30-60 seconds while the model thinks. The discovery loop layers
		// its own wallclock budget on top.
		http: &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *AgentClient) Model() string { return c.model }

// =============================================================================
// Messages API types — tool-use shape
// =============================================================================

// ToolDef is a single tool the agent can invoke. InputSchema must be a valid
// JSON Schema object (`{"type":"object", "properties":{...}, "required":[...]}`).
// Keep schemas small — the agent re-reads them on every turn.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ContentBlock is one item in a message's content array. Discovery uses
// text, tool_use, and tool_result block types. Other Anthropic block types
// (image, document, thinking) are intentionally absent — add them only when
// some future agent actually needs them.
type ContentBlock struct {
	Type string `json:"type"`

	// type=text
	Text string `json:"text,omitempty"`

	// type=tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// type=tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"` // JSON-encoded tool result
	IsError   bool   `json:"is_error,omitempty"`
}

// Message is one entry in the messages array. Role is "user" or "assistant".
// Content is always an array of blocks (we never use the shorthand string
// form because tool_result has to be a block).
type Message struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// MessagesRequest is the wire shape for POST /v1/messages with tools.
type MessagesRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Tools     []ToolDef `json:"tools,omitempty"`
	Messages  []Message `json:"messages"`
}

// MessagesResponse — what the API hands back. StopReason values we care
// about: "end_turn" (agent finished, no more tools), "tool_use" (next turn
// must include tool_result blocks), "max_tokens" (we ran out of room — for
// the agent loop this is essentially a budget overrun).
type MessagesResponse struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Role       string         `json:"role"`
	Model      string         `json:"model"`
	Content    []ContentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage      Usage          `json:"usage"`
}

// Usage tracks token spend per request. Discovery sums these across the loop
// to enforce the 50K-token ceiling.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// =============================================================================
// Send — single request, decoded response
// =============================================================================

// Send fires one Messages request and returns the parsed response. Caller
// drives the multi-turn loop (we don't auto-iterate here — looping is the
// usecase's job because budget tracking, timeouts, and tool dispatch are
// all loop-level concerns).
func (c *AgentClient) Send(ctx context.Context, req MessagesRequest) (*MessagesResponse, error) {
	if req.Model == "" {
		req.Model = c.model
	}
	if req.MaxTokens == 0 {
		// Discovery agent typically emits 200-800 output tokens per turn
		// (mostly tool calls; final commit_artifact emits more). 4K leaves
		// headroom without burning budget on a needlessly large generation.
		req.MaxTokens = 4096
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPI, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("content-type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic API status %d: %s", resp.StatusCode, string(respBody))
	}

	var msgResp MessagesResponse
	if err := json.Unmarshal(respBody, &msgResp); err != nil {
		return nil, fmt.Errorf("unmarshal: %w (body: %.500s)", err, string(respBody))
	}
	return &msgResp, nil
}
