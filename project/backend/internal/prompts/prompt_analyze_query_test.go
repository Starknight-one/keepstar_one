package prompts

import (
	"strings"
	"testing"

	"keepstar/internal/domain"
)

func TestBuildAgent1ContextPrompt_NoState(t *testing.T) {
	meta := domain.StateMeta{ProductCount: 0, ServiceCount: 0}
	result := BuildAgent1ContextPrompt(meta, nil, "покажи кроссовки")

	// Should just return the raw query (digest is now in conversation_history, not here)
	if result != "покажи кроссовки" {
		t.Errorf("expected raw query, got: %s", result)
	}
}

func TestBuildAgent1ContextPrompt_WithState(t *testing.T) {
	meta := domain.StateMeta{ProductCount: 5, ServiceCount: 0, Fields: []string{"id", "name", "price"}}
	result := BuildAgent1ContextPrompt(meta, nil, "покажи дешевле")

	if !strings.Contains(result, "<state>") {
		t.Error("expected <state> block when data is loaded")
	}
	if !strings.Contains(result, "покажи дешевле") {
		t.Error("expected user query in output")
	}
}

func TestBuildAgent1ContextPrompt_StateAndQuery(t *testing.T) {
	meta := domain.StateMeta{
		ProductCount: 10,
		ServiceCount: 0,
		Fields:       []string{"id", "name", "price", "brand"},
	}
	result := BuildAgent1ContextPrompt(meta, nil, "а теперь Adidas")

	if !strings.Contains(result, "<state>") {
		t.Error("expected <state> block")
	}

	// Query should come after state
	stateIdx := strings.Index(result, "<state>")
	queryIdx := strings.Index(result, "а теперь Adidas")
	if queryIdx < stateIdx {
		t.Errorf("expected query after <state>")
	}
}
