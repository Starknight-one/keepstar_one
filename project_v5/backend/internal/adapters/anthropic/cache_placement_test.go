package anthropic

import (
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// hasEphemeral reports whether a CacheControlEphemeralParam is set
// (non-zero / non-omitted). Zero values are skipped by the SDK marshaller,
// so checking via param.IsOmitted is the reliable test.
func hasEphemeral(p sdk.CacheControlEphemeralParam) bool {
	return !param.IsOmitted(p)
}

func sampleTool(name string) domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        name,
		Description: "test tool " + name,
		InputSchema: map[string]interface{}{
			"type":       "object",
			"required":   []string{"q"},
			"properties": map[string]interface{}{"q": map[string]interface{}{"type": "string"}},
		},
	}
}

// TestCachePlacementToolsLastOnly — only CacheTools=true. System block
// and history must NOT carry cache_control; the LAST tool in the
// (sorted-by-name) tools array must.
func TestCachePlacementToolsLastOnly(t *testing.T) {
	tools := []domain.ToolDefinition{sampleTool("zebra"), sampleTool("apple")}
	msgs := []domain.LLMMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
		{Role: "user", Content: "again"},
	}
	params := buildMessageNewParams("claude-haiku-4-5", 1024, "system", msgs, tools,
		ports.CacheConfig{CacheTools: true})

	if len(params.System) != 1 {
		t.Fatalf("expected 1 system block, got %d", len(params.System))
	}
	if hasEphemeral(params.System[0].CacheControl) {
		t.Errorf("system should NOT be cache-marked")
	}

	if len(params.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(params.Tools))
	}
	// Sorted ASC by name: apple, zebra. Last = zebra → cached.
	if got := params.Tools[0].OfTool.Name; got != "apple" {
		t.Errorf("tools[0] = %q, want apple", got)
	}
	if got := params.Tools[1].OfTool.Name; got != "zebra" {
		t.Errorf("tools[1] = %q, want zebra", got)
	}
	if hasEphemeral(params.Tools[0].OfTool.CacheControl) {
		t.Errorf("non-last tool should NOT be cache-marked")
	}
	if !hasEphemeral(params.Tools[1].OfTool.CacheControl) {
		t.Errorf("last tool must be cache-marked")
	}

	for i, m := range params.Messages {
		for j := range m.Content {
			if m.Content[j].OfText != nil && hasEphemeral(m.Content[j].OfText.CacheControl) {
				t.Errorf("messages[%d].block[%d] should NOT be cache-marked", i, j)
			}
		}
	}
}

// TestCachePlacementSystemOnly — only CacheSystem=true marks the system block.
func TestCachePlacementSystemOnly(t *testing.T) {
	params := buildMessageNewParams("claude-haiku-4-5", 1024, "system",
		[]domain.LLMMessage{{Role: "user", Content: "hi"}},
		[]domain.ToolDefinition{sampleTool("t")},
		ports.CacheConfig{CacheSystem: true})

	if !hasEphemeral(params.System[0].CacheControl) {
		t.Errorf("system block must be cache-marked")
	}
	if hasEphemeral(params.Tools[0].OfTool.CacheControl) {
		t.Errorf("tool should NOT be cache-marked when only CacheSystem set")
	}
}

// TestCachePlacementConversationSecondToLast — CacheConversation=true marks
// the second-to-last message, not the last and not earlier.
func TestCachePlacementConversationSecondToLast(t *testing.T) {
	msgs := []domain.LLMMessage{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "second"},
		{Role: "user", Content: "third"},
		{Role: "assistant", Content: "fourth"},
	}
	params := buildMessageNewParams("claude-haiku-4-5", 1024, "",
		msgs, nil, ports.CacheConfig{CacheConversation: true})

	if len(params.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(params.Messages))
	}
	for i, m := range params.Messages {
		for j := range m.Content {
			if m.Content[j].OfText == nil {
				continue
			}
			marked := hasEphemeral(m.Content[j].OfText.CacheControl)
			wantMarked := i == len(params.Messages)-2
			if marked != wantMarked {
				t.Errorf("messages[%d].block[%d] marked=%v, want=%v", i, j, marked, wantMarked)
			}
		}
	}
}

// TestCachePlacementConversationNoOpOnSingleMessage — CacheConversation
// requires ≥ 2 messages; single-message conversation gets no cache mark.
func TestCachePlacementConversationNoOpOnSingleMessage(t *testing.T) {
	msgs := []domain.LLMMessage{{Role: "user", Content: "only"}}
	params := buildMessageNewParams("claude-haiku-4-5", 1024, "", msgs, nil,
		ports.CacheConfig{CacheConversation: true})

	if hasEphemeral(params.Messages[0].Content[0].OfText.CacheControl) {
		t.Errorf("single-message conversation should not be cache-marked")
	}
}

// TestCachePlacementAllThreeFlags — production Agent2 path: tools last,
// system first, conversation second-to-last.
func TestCachePlacementAllThreeFlags(t *testing.T) {
	msgs := []domain.LLMMessage{
		{Role: "user", Content: "search"},
		{Role: "assistant", Content: "results"},
		{Role: "user", Content: "show me 3"},
	}
	tools := []domain.ToolDefinition{sampleTool("visual_assembly"), sampleTool("catalog_search")}
	params := buildMessageNewParams("claude-haiku-4-5", 1024, "Agent2 prompt",
		msgs, tools, ports.CacheConfig{
			CacheTools:        true,
			CacheSystem:       true,
			CacheConversation: true,
		})

	if !hasEphemeral(params.System[0].CacheControl) {
		t.Errorf("system block must be cache-marked")
	}
	// Tools sorted by name: catalog_search (0), visual_assembly (1, last).
	if hasEphemeral(params.Tools[0].OfTool.CacheControl) {
		t.Errorf("first tool (catalog_search) must NOT be cache-marked")
	}
	if !hasEphemeral(params.Tools[1].OfTool.CacheControl) {
		t.Errorf("last tool (visual_assembly) must be cache-marked")
	}
	// Second-to-last message = index 1 (assistant "results").
	for i, m := range params.Messages {
		marked := hasEphemeral(m.Content[0].OfText.CacheControl)
		wantMarked := i == 1
		if marked != wantMarked {
			t.Errorf("messages[%d] marked=%v, want=%v", i, marked, wantMarked)
		}
	}
}

// TestToolChoiceMapping — empty / auto / any / tool:NAME variants.
func TestToolChoiceMapping(t *testing.T) {
	cases := []struct {
		input string
		want  string // "" | "auto" | "any" | "tool:visual_assembly"
	}{
		{"", ""},
		{"auto", "auto"},
		{"any", "any"},
		{"tool:visual_assembly", "tool"},
	}
	for _, tc := range cases {
		params := buildMessageNewParams("claude-haiku-4-5", 1024, "", nil, nil,
			ports.CacheConfig{ToolChoice: tc.input})
		switch tc.want {
		case "":
			if !param.IsOmitted(params.ToolChoice) {
				t.Errorf("ToolChoice %q: expected unset", tc.input)
			}
		case "auto":
			if params.ToolChoice.OfAuto == nil {
				t.Errorf("ToolChoice %q: expected OfAuto", tc.input)
			}
		case "any":
			if params.ToolChoice.OfAny == nil {
				t.Errorf("ToolChoice %q: expected OfAny", tc.input)
			}
		case "tool":
			if params.ToolChoice.OfTool == nil {
				t.Errorf("ToolChoice %q: expected OfTool", tc.input)
			} else if got := params.ToolChoice.OfTool.Name; got != "visual_assembly" {
				t.Errorf("ToolChoice tool name = %q, want visual_assembly", got)
			}
		}
	}
}

// TestToolUseAndToolResultBlocks — multi-turn with assistant ToolCalls and
// user ToolResult — each lands in the right SDK block variant.
func TestToolUseAndToolResultBlocks(t *testing.T) {
	msgs := []domain.LLMMessage{
		{Role: "user", Content: "do it"},
		{Role: "assistant", ToolCalls: []domain.ToolCall{{
			ID: "toolu_xyz", Name: "visual_assembly",
			Input: map[string]interface{}{"preset": "product_card"},
		}}},
		{Role: "user", ToolResult: &domain.ToolResult{ToolUseID: "toolu_xyz", Content: "ok"}},
	}
	params := buildMessageNewParams("claude-haiku-4-5", 1024, "", msgs, nil,
		ports.CacheConfig{})

	if params.Messages[1].Content[0].OfToolUse == nil {
		t.Errorf("assistant message should hold a tool_use block, got %+v", params.Messages[1])
	}
	if params.Messages[2].Content[0].OfToolResult == nil {
		t.Errorf("user message should hold a tool_result block, got %+v", params.Messages[2])
	}
}

// TestCostCalculate — sanity check on pricing math: 1k input + 500 output
// on Haiku = $0.001 + $0.0025 = $0.0035.
func TestCostCalculate(t *testing.T) {
	usage := domain.LLMUsage{
		InputTokens:  1000,
		OutputTokens: 500,
		Model:        "claude-haiku-4-5",
	}
	cost := Calculate(usage)
	const want = 0.0035
	if delta := cost - want; delta > 1e-9 || delta < -1e-9 {
		t.Errorf("Calculate(%+v) = %v, want %v", usage, cost, want)
	}
}

// TestCostCalculateWithCache — cache write at 1.25× and cache read at
// 0.10× of input rate. 1000 cache_write + 1000 cache_read on Haiku
// (input rate $1/MTok) = $0.00125 + $0.0001 = $0.00135.
func TestCostCalculateWithCache(t *testing.T) {
	usage := domain.LLMUsage{
		CacheCreationInputTokens: 1000,
		CacheReadInputTokens:     1000,
		Model:                    "claude-haiku-4-5",
	}
	cost := Calculate(usage)
	const want = 0.00135
	if delta := cost - want; delta > 1e-9 || delta < -1e-9 {
		t.Errorf("Calculate(%+v) = %v, want %v", usage, cost, want)
	}
}
