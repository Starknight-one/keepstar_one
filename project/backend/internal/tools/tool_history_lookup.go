package tools

import (
	"context"
	"fmt"
	"strings"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// HistoryLookupTool searches session deltas for tool/path history
type HistoryLookupTool struct {
	statePort ports.StatePort
}

// NewHistoryLookupTool creates the history lookup tool
func NewHistoryLookupTool(statePort ports.StatePort) *HistoryLookupTool {
	return &HistoryLookupTool{statePort: statePort}
}

// Definition returns the tool definition for LLM
func (t *HistoryLookupTool) Definition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "_internal_history_lookup",
		Description: "Search session history deltas. Use to understand what happened earlier in the session.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Search term to match against tool names and paths (case-insensitive).",
				},
				"last_n": map[string]interface{}{
					"type":        "integer",
					"description": "Only look at the last N deltas (default: all).",
				},
			},
		},
	}
}

// Execute searches session deltas and returns matching history entries
func (t *HistoryLookupTool) Execute(ctx context.Context, toolCtx ToolContext, input map[string]interface{}) (*domain.ToolResult, error) {
	query, _ := input["query"].(string)
	lastN := 0
	if v, ok := input["last_n"].(float64); ok && v > 0 {
		lastN = int(v)
	}

	deltas, err := t.statePort.GetDeltas(ctx, toolCtx.SessionID)
	if err != nil {
		return &domain.ToolResult{Content: "error: cannot read deltas", IsError: true}, nil
	}

	if lastN > 0 && lastN < len(deltas) {
		deltas = deltas[len(deltas)-lastN:]
	}

	var lines []string
	for _, d := range deltas {
		// Match query against tool name and path
		if query != "" {
			toolName := d.Action.Tool
			path := d.Path
			if !containsCI(toolName, query) && !containsCI(path, query) {
				continue
			}
		}
		line := fmt.Sprintf("step %d: %s %s → %d items", d.Step, d.Action.Tool, d.Path, d.Result.Count)
		if len(d.Result.Fields) > 0 {
			line += fmt.Sprintf(" [%s]", strings.Join(d.Result.Fields, ","))
		}
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return &domain.ToolResult{Content: "no matching history entries"}, nil
	}

	return &domain.ToolResult{Content: strings.Join(lines, "\n")}, nil
}
