package tools

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	"keepstar/internal/ports"
	"keepstar/internal/prompts"
)

// NormalizeResult is the structured output from query normalization
type NormalizeResult struct {
	Query         string `json:"query"`
	Brand         string `json:"brand"`
	SourceLang    string `json:"source_lang"`
	AliasResolved bool   `json:"alias_resolved"`
}

// QueryNormalizer normalizes search queries via LLM or fast path
type QueryNormalizer struct {
	llm ports.LLMPort
}

// NewQueryNormalizer creates a new normalizer
func NewQueryNormalizer(llm ports.LLMPort) *QueryNormalizer {
	return &QueryNormalizer{llm: llm}
}

// Normalize translates and normalizes a query and brand.
// Fast path: if both are ASCII-only, returns as-is (no LLM call).
// LLM path: calls LLM to resolve aliases, transliterate brands, translate.
func (n *QueryNormalizer) Normalize(ctx context.Context, query, brand string) (*NormalizeResult, error) {
	// Fast path: ASCII-only input → no normalization needed
	if isASCII(query) && isASCII(brand) {
		return &NormalizeResult{
			Query:         query,
			Brand:         brand,
			SourceLang:    "en",
			AliasResolved: false,
		}, nil
	}

	// LLM path: call normalizer
	resp, err := n.llm.ChatWithUsage(ctx, prompts.NormalizeQueryPrompt, prompts.BuildNormalizeRequest(query, brand))
	if err != nil {
		// Fallback: return input as-is on error
		return &NormalizeResult{
			Query:      query,
			Brand:      brand,
			SourceLang: "unknown",
		}, nil
	}

	// Strip markdown code fences if LLM wrapped the response
	text := stripCodeFences(resp.Text)

	var result NormalizeResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		// Fallback: return input as-is on parse error
		return &NormalizeResult{
			Query:      query,
			Brand:      brand,
			SourceLang: "unknown",
		}, nil
	}

	return &result, nil
}

// isASCII checks if a string contains only ASCII characters (a-z, A-Z, 0-9, space, punctuation)
func isASCII(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// stripCodeFences removes markdown code fences from LLM responses
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	// Remove ```json ... ``` or ``` ... ```
	if strings.HasPrefix(s, "```") {
		// Find end of first line (skip ```json or ```)
		idx := strings.Index(s, "\n")
		if idx >= 0 {
			s = s[idx+1:]
		}
		// Remove trailing ```
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	}
	return s
}
