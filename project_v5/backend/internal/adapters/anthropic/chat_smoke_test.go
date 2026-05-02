//go:build live

// Live smoke test for the Anthropic adapter. Hits the real Messages API
// once with a minimal prompt. Cost: a handful of tokens (~$0.0001).
//
// Run with:
//
//	ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY \
//	  go test -tags=live -v ./internal/adapters/anthropic/...
//
// Skips when ANTHROPIC_API_KEY is absent so it never fails CI.

package anthropic

import (
	"context"
	"os"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

func TestLiveChatRoundtrip(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set — skipping live smoke")
	}

	c := NewClient(apiKey, "claude-haiku-4-5")
	resp, err := c.ChatWithToolsCached(
		context.Background(),
		"You are a helpful assistant. Reply with one word.",
		[]domain.LLMMessage{{Role: "user", Content: "Reply with the literal word OK and nothing else."}},
		nil,
		ports.CacheConfig{},
	)
	if err != nil {
		t.Fatalf("ChatWithToolsCached: %v", err)
	}
	if resp.Text == "" {
		t.Errorf("expected non-empty text, got %+v", resp)
	}
	if resp.Usage.InputTokens == 0 {
		t.Errorf("expected non-zero InputTokens, got %+v", resp.Usage)
	}
	if resp.Usage.Model != "claude-haiku-4-5" {
		t.Errorf("Usage.Model = %q, want claude-haiku-4-5", resp.Usage.Model)
	}
	t.Logf("response text: %q", resp.Text)
	t.Logf("usage: input=%d output=%d cost=%.6f USD",
		resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.CostUSD)
}

func TestLiveCountTokens(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set — skipping live smoke")
	}

	c := NewClient(apiKey, "claude-haiku-4-5")
	n, err := c.CountInputTokens(
		context.Background(),
		"You are a helpful assistant.",
		[]domain.LLMMessage{{Role: "user", Content: "Hello"}},
		nil,
	)
	if err != nil {
		t.Fatalf("CountInputTokens: %v", err)
	}
	if n <= 0 {
		t.Errorf("expected positive token count, got %d", n)
	}
	t.Logf("count_tokens reports %d input tokens for trivial prompt", n)
}
