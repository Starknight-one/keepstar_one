package prompts

import (
	"strings"
	"testing"
)

// TestAgent2SystemPromptSectionsPresent — chunk 9 added BUILDING FROM
// SCRATCH and COMPOSING sections to unblock the V5 tool surface (preset
// optional / multi-widget compose). If anyone removes them, V5 silently
// regresses to the chunk-6b shape that mirrors only V4's preset path.
func TestAgent2SystemPromptSectionsPresent(t *testing.T) {
	required := []string{
		"## BUILDING FROM SCRATCH",
		"## COMPOSING",
		"### Rules for freestyle",
		"### Rules for compose",
		"There are THREE call shapes",
	}
	for _, s := range required {
		if !strings.Contains(Agent2SystemPrompt, s) {
			t.Errorf("Agent2SystemPrompt is missing required section/marker: %q", s)
		}
	}
}

// TestAgent2SystemPromptCacheBudget — sanity-check the prompt body stays
// over the rough token-budget threshold (4500 tokens × ~4 chars/token =
// ~18000 chars). Real-token measurement is in engine/tokens; this guards
// against accidental deletions that would slip the cache gate without
// running the gated test.
func TestAgent2SystemPromptCacheBudget(t *testing.T) {
	const minChars = 18000
	if len(Agent2SystemPrompt) < minChars {
		t.Errorf("Agent2SystemPrompt = %d chars, want ≥ %d (cache prefix budget)", len(Agent2SystemPrompt), minChars)
	}
}
