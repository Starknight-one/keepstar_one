// Package tokens contains paper-sketch token measurements between V4 and V5
// prompt+tool+tree_map shapes. Not part of the engine runtime — it's
// scaffolding so we can iterate on the V5 prompt design with concrete
// numbers instead of intuition.
//
// The CountInputTokens helper wraps Anthropic's /v1/messages/count_tokens
// endpoint directly via net/http to avoid pulling the SDK as a dep before
// chunk 6a (which is when the SDK lands properly).
package tokens

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// CountTokensRequest is the subset of /v1/messages/count_tokens we use.
// System and Tools both contribute to input_tokens; messages must include at
// least one user message.
type CountTokensRequest struct {
	Model    string                   `json:"model"`
	System   string                   `json:"system,omitempty"`
	Tools    []map[string]interface{} `json:"tools,omitempty"`
	Messages []CountMessage           `json:"messages"`
}

type CountMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CountTokensResponse struct {
	InputTokens int `json:"input_tokens"`
}

// CountInputTokens calls the Anthropic count_tokens endpoint with the given
// system prompt, tool definitions, and messages, and returns input_tokens.
// Network errors and non-200 responses are surfaced as Go errors so the
// caller (test) can decide whether to skip or fail.
func CountInputTokens(apiKey, model string, system string, tools []map[string]interface{}, messages []CountMessage) (int, error) {
	body := CountTokensRequest{
		Model:    model,
		System:   system,
		Tools:    tools,
		Messages: messages,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages/count_tokens", bytes.NewReader(raw))
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("call count_tokens: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("count_tokens %d: %s", resp.StatusCode, string(respBytes))
	}

	var out CountTokensResponse
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return 0, fmt.Errorf("decode response: %w", err)
	}
	return out.InputTokens, nil
}
