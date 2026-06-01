// Package tokens contains paper-sketch token measurements between V4 and V5
// prompt+tool+tree_map shapes. Not part of the engine runtime — it's
// scaffolding so we can iterate on the V5 prompt design with concrete
// numbers instead of intuition.
//
// As of chunk 6a, CountInputTokens delegates to the SDK-backed adapter
// (`internal/adapters/anthropic`). The exported signature is preserved so
// `measurement_test.go` keeps working without changes; the underlying call
// is now typed (Anthropic Go SDK) instead of hand-rolled HTTP.
package tokens

import (
	"context"
	"fmt"

	"keepstar_v5/internal/adapters/anthropic"
	"keepstar_v5/internal/domain"
)

// CountMessage is the trivial message shape exposed to callers (mostly the
// measurement test). Role is "user" | "assistant"; Content is plain text.
type CountMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CountInputTokens calls the Anthropic count_tokens endpoint via the SDK
// adapter and returns input_tokens.
//
// `tools` is each tool's full schema as a parsed JSON map (matches what
// json.Unmarshal of a tool-def file produces). The map must carry `name`,
// `description`, and `input_schema` top-level keys.
func CountInputTokens(apiKey, model string, system string, tools []map[string]interface{}, messages []CountMessage) (int, error) {
	client := anthropic.NewClient(apiKey, model)

	domainTools, err := convertTools(tools)
	if err != nil {
		return 0, err
	}
	domainMsgs := convertMessages(messages)

	return client.CountInputTokens(context.Background(), system, domainMsgs, domainTools)
}

// convertTools turns each parsed tool-schema map into a typed
// domain.ToolDefinition. Returns an error if a required field is missing
// or has the wrong type — these would silently produce empty SDK params
// and skew the measurement.
func convertTools(tools []map[string]interface{}) ([]domain.ToolDefinition, error) {
	out := make([]domain.ToolDefinition, 0, len(tools))
	for i, raw := range tools {
		name, _ := raw["name"].(string)
		if name == "" {
			return nil, fmt.Errorf("tools[%d]: missing or non-string `name`", i)
		}
		desc, _ := raw["description"].(string)
		schema, ok := raw["input_schema"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("tools[%d]: missing or non-object `input_schema`", i)
		}
		out = append(out, domain.ToolDefinition{
			Name:        name,
			Description: desc,
			InputSchema: schema,
		})
	}
	return out, nil
}

// convertMessages maps CountMessage to domain.LLMMessage. Pure plumbing —
// the test layer doesn't construct tool_use / tool_result messages today.
func convertMessages(msgs []CountMessage) []domain.LLMMessage {
	out := make([]domain.LLMMessage, len(msgs))
	for i, m := range msgs {
		out[i] = domain.LLMMessage{Role: m.Role, Content: m.Content}
	}
	return out
}
