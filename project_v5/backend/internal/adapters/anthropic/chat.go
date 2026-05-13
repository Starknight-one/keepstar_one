package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// ChatWithToolsCached calls Messages.New with cache_control hints placed
// per cfg. See ports.CacheConfig for the placement rules.
func (c *Client) ChatWithToolsCached(
	ctx context.Context,
	systemPrompt string,
	messages []domain.LLMMessage,
	tools []domain.ToolDefinition,
	cfg ports.CacheConfig,
) (*domain.LLMResponse, error) {
	params := buildMessageNewParams(c.model, c.maxTokens, systemPrompt, messages, tools, cfg)

	resp, err := c.sdk.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic.Messages.New: %w", err)
	}
	return convertResponse(resp, c.model), nil
}

// CountInputTokens calls Messages.CountTokens with the same input shape
// as a real chat would carry, so the count is what we'd actually be
// billed for. cache_control hints are NOT applied — count_tokens reports
// raw input tokens regardless of caching.
func (c *Client) CountInputTokens(
	ctx context.Context,
	systemPrompt string,
	messages []domain.LLMMessage,
	tools []domain.ToolDefinition,
) (int, error) {
	params := buildCountTokensParams(c.model, systemPrompt, messages, tools)
	resp, err := c.sdk.Messages.CountTokens(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("anthropic.Messages.CountTokens: %w", err)
	}
	return int(resp.InputTokens), nil
}

// buildMessageNewParams assembles the SDK request struct from V5 domain
// types. Factored out so cache_placement_test.go can inspect the result
// without making a network call.
//
// Cache_control placement (V4-proven):
//
//   - cfg.CacheTools=true        → ephemeral on the LAST tool entry
//   - cfg.CacheSystem=true       → ephemeral on the system block
//   - cfg.CacheConversation=true → ephemeral on the SECOND-TO-LAST message
//     (when there are ≥ 2 messages; no-op
//     otherwise)
//
// Tools are sorted by name for byte-stable cache keys (V4 pattern, see
// project_v4/.../tool_registry.go:75).
func buildMessageNewParams(
	model string,
	maxTokens int64,
	systemPrompt string,
	messages []domain.LLMMessage,
	tools []domain.ToolDefinition,
	cfg ports.CacheConfig,
) sdk.MessageNewParams {
	params := sdk.MessageNewParams{
		MaxTokens: maxTokens,
		Model:     sdk.Model(model),
	}

	// System prompt as a single text block, optionally cache-marked.
	if systemPrompt != "" {
		sys := sdk.TextBlockParam{Text: systemPrompt}
		if cfg.CacheSystem {
			sys.CacheControl = ephemeral()
		}
		params.System = []sdk.TextBlockParam{sys}
	}

	// Tools: sorted by name; last gets cache_control.
	if len(tools) > 0 {
		params.Tools = buildToolUnionParams(tools, cfg.CacheTools)
	}

	// Tool choice.
	if tc := buildToolChoice(cfg.ToolChoice); tc != nil {
		params.ToolChoice = *tc
	}

	// Messages with optional cache_control on second-to-last.
	params.Messages = buildMessageParams(messages, cfg.CacheConversation)

	return params
}

// buildCountTokensParams is the count_tokens twin of buildMessageNewParams.
// Same input shape, different SDK param type — and no MaxTokens (the
// count_tokens endpoint doesn't take one). Cache hints are also irrelevant
// here: count_tokens always reports raw input.
func buildCountTokensParams(
	model string,
	systemPrompt string,
	messages []domain.LLMMessage,
	tools []domain.ToolDefinition,
) sdk.MessageCountTokensParams {
	params := sdk.MessageCountTokensParams{
		Model: sdk.Model(model),
	}
	if systemPrompt != "" {
		params.System = sdk.MessageCountTokensParamsSystemUnion{
			OfTextBlockArray: []sdk.TextBlockParam{{Text: systemPrompt}},
		}
	}
	if len(tools) > 0 {
		params.Tools = buildCountTokensTools(tools)
	}
	params.Messages = buildMessageParams(messages, false)
	return params
}

// buildToolUnionParams converts domain ToolDefinitions to SDK ToolUnionParam,
// sorted by name. When cacheLast is true, the last tool gets
// cache_control: ephemeral. Tools array order matters for prompt caching:
// sorted by name so the array is byte-stable across calls.
func buildToolUnionParams(tools []domain.ToolDefinition, cacheLast bool) []sdk.ToolUnionParam {
	sorted := make([]domain.ToolDefinition, len(tools))
	copy(sorted, tools)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	out := make([]sdk.ToolUnionParam, len(sorted))
	for i, t := range sorted {
		tp := sdk.ToolParam{
			Name: t.Name,
			InputSchema: sdk.ToolInputSchemaParam{
				Properties: extractProperties(t.InputSchema),
				Required:   extractRequired(t.InputSchema),
			},
		}
		if t.Description != "" {
			tp.Description = sdk.String(t.Description)
		}
		if cacheLast && i == len(sorted)-1 {
			tp.CacheControl = ephemeral()
		}
		out[i] = sdk.ToolUnionParam{OfTool: &tp}
	}
	return out
}

// buildCountTokensTools produces the count_tokens variant of the tools
// array. Different union type than buildToolUnionParams — and count_tokens
// doesn't bill, so we don't bother with cache_control.
func buildCountTokensTools(tools []domain.ToolDefinition) []sdk.MessageCountTokensToolUnionParam {
	sorted := make([]domain.ToolDefinition, len(tools))
	copy(sorted, tools)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	out := make([]sdk.MessageCountTokensToolUnionParam, len(sorted))
	for i, t := range sorted {
		schema := sdk.ToolInputSchemaParam{
			Properties: extractProperties(t.InputSchema),
			Required:   extractRequired(t.InputSchema),
		}
		out[i] = sdk.MessageCountTokensToolParamOfTool(schema, t.Name)
		if t.Description != "" && out[i].OfTool != nil {
			out[i].OfTool.Description = sdk.String(t.Description)
		}
	}
	return out
}

// extractProperties pulls the `properties` map out of a JSONSchema
// document. domain.ToolDefinition.InputSchema is a generic
// map[string]interface{}; the SDK wants the inner properties subobject.
func extractProperties(schema map[string]interface{}) any {
	if p, ok := schema["properties"]; ok {
		return p
	}
	return nil
}

// extractRequired pulls the `required` list out of a JSONSchema document.
func extractRequired(schema map[string]interface{}) []string {
	raw, ok := schema["required"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// buildToolChoice maps cfg.ToolChoice strings to the SDK union type.
// Empty string → nil (let SDK default to "auto" behavior).
//
//   - "auto"        → ToolChoiceAutoParam
//   - "any"         → ToolChoiceAnyParam
//   - "tool:NAME"   → ToolChoiceToolParam{Name: NAME}
func buildToolChoice(s string) *sdk.ToolChoiceUnionParam {
	if s == "" {
		return nil
	}
	switch {
	case s == "auto":
		return &sdk.ToolChoiceUnionParam{OfAuto: &sdk.ToolChoiceAutoParam{}}
	case s == "any":
		return &sdk.ToolChoiceUnionParam{OfAny: &sdk.ToolChoiceAnyParam{}}
	case strings.HasPrefix(s, "tool:"):
		name := strings.TrimPrefix(s, "tool:")
		return &sdk.ToolChoiceUnionParam{OfTool: &sdk.ToolChoiceToolParam{Name: name}}
	}
	return nil
}

// buildMessageParams converts V5 domain.LLMMessage entries to SDK
// MessageParam entries. Each message becomes a user/assistant message
// with one content block (text, tool_use, or tool_result).
//
// When cacheConv is true and there are ≥ 2 messages, cache_control:
// ephemeral lands on the SECOND-TO-LAST message's content blocks. The
// most-recent assistant turn is the longest stable prefix that the next
// user message will retain — so cache it.
func buildMessageParams(messages []domain.LLMMessage, cacheConv bool) []sdk.MessageParam {
	out := make([]sdk.MessageParam, 0, len(messages))
	cacheTargetIdx := -1
	if cacheConv && len(messages) >= 2 {
		cacheTargetIdx = len(messages) - 2
	}
	for i, m := range messages {
		blocks := blocksForMessage(m)
		if i == cacheTargetIdx {
			markBlocksCacheable(blocks)
		}
		switch m.Role {
		case "assistant":
			out = append(out, sdk.NewAssistantMessage(blocks...))
		default:
			// tool_result blocks travel on user-role messages.
			out = append(out, sdk.NewUserMessage(blocks...))
		}
	}
	return out
}

// blocksForMessage returns the SDK content blocks corresponding to one
// domain message:
//
//   - ToolResult set       → one tool_result block (role: user)
//   - ToolCalls set        → one tool_use block per call (role: assistant)
//   - Plain Content set    → one text block
//
// Mutually exclusive in practice; we handle them in priority order.
func blocksForMessage(m domain.LLMMessage) []sdk.ContentBlockParamUnion {
	if m.ToolResult != nil {
		return []sdk.ContentBlockParamUnion{
			sdk.NewToolResultBlock(m.ToolResult.ToolUseID, m.ToolResult.Content, m.ToolResult.IsError),
		}
	}
	if len(m.ToolCalls) > 0 {
		out := make([]sdk.ContentBlockParamUnion, len(m.ToolCalls))
		for i, tc := range m.ToolCalls {
			out[i] = sdk.NewToolUseBlock(tc.ID, tc.Input, tc.Name)
		}
		return out
	}
	return []sdk.ContentBlockParamUnion{sdk.NewTextBlock(m.Content)}
}

// markBlocksCacheable attaches cache_control: ephemeral to whichever
// content-block variant the union holds. Each block carries CacheControl
// on its specific param type — we walk the union variants and set the
// one that's populated.
func markBlocksCacheable(blocks []sdk.ContentBlockParamUnion) {
	for i := range blocks {
		switch {
		case blocks[i].OfText != nil:
			blocks[i].OfText.CacheControl = ephemeral()
		case blocks[i].OfToolUse != nil:
			blocks[i].OfToolUse.CacheControl = ephemeral()
		case blocks[i].OfToolResult != nil:
			blocks[i].OfToolResult.CacheControl = ephemeral()
		}
	}
}

// ephemeral returns a CacheControlEphemeralParam with default 5m TTL.
// Type must be set explicitly — the SDK's IsOmitted check is reflect-based
// IsZero(), so a struct literal with all zero fields would be elided
// during marshalling.
func ephemeral() sdk.CacheControlEphemeralParam {
	return sdk.CacheControlEphemeralParam{Type: "ephemeral"}
}

// convertResponse maps an SDK Message to the domain LLMResponse shape
// Agent2 understands. Concatenates text blocks, lifts tool_use blocks,
// reads usage counters and computes USD cost.
func convertResponse(msg *sdk.Message, model string) *domain.LLMResponse {
	if msg == nil {
		return &domain.LLMResponse{Usage: domain.LLMUsage{Model: model}}
	}
	out := &domain.LLMResponse{
		StopReason: string(msg.StopReason),
	}
	var textBuf strings.Builder
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			textBuf.WriteString(block.Text)
		case "tool_use":
			input := map[string]interface{}{}
			if len(block.Input) > 0 {
				_ = json.Unmarshal(block.Input, &input)
			}
			out.ToolCalls = append(out.ToolCalls, domain.ToolCall{
				ID:    block.ID,
				Name:  block.Name,
				Input: input,
			})
		}
	}
	out.Text = textBuf.String()
	out.Usage = domain.LLMUsage{
		InputTokens:              int(msg.Usage.InputTokens),
		OutputTokens:             int(msg.Usage.OutputTokens),
		CacheCreationInputTokens: int(msg.Usage.CacheCreationInputTokens),
		CacheReadInputTokens:     int(msg.Usage.CacheReadInputTokens),
		TotalTokens:              int(msg.Usage.InputTokens + msg.Usage.OutputTokens),
		Model:                    model,
	}
	out.Usage.CostUSD = Calculate(out.Usage)
	return out
}
