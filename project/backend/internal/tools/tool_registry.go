package tools

import (
	"context"
	"fmt"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// ToolExecutor executes a tool and writes results to state
type ToolExecutor interface {
	// Definition returns the tool definition for LLM
	Definition() domain.ToolDefinition

	// Execute runs the tool with given input, writes to state, returns "ok"/"empty"
	Execute(ctx context.Context, sessionID string, input map[string]interface{}) (*domain.ToolResult, error)
}

// Registry holds all available tools
type Registry struct {
	tools       map[string]ToolExecutor
	statePort   ports.StatePort
	catalogPort ports.CatalogPort
}

// NewRegistry creates a tool registry with dependencies
func NewRegistry(statePort ports.StatePort, catalogPort ports.CatalogPort) *Registry {
	r := &Registry{
		tools:       make(map[string]ToolExecutor),
		statePort:   statePort,
		catalogPort: catalogPort,
	}

	// Register available tools
	r.Register(NewSearchProductsTool(statePort, catalogPort))

	return r
}

// Register adds a tool to the registry
func (r *Registry) Register(tool ToolExecutor) {
	def := tool.Definition()
	r.tools[def.Name] = tool
}

// GetDefinitions returns all tool definitions for LLM
func (r *Registry) GetDefinitions() []domain.ToolDefinition {
	defs := make([]domain.ToolDefinition, 0, len(r.tools))
	for _, tool := range r.tools {
		defs = append(defs, tool.Definition())
	}
	return defs
}

// Execute runs a tool by name
func (r *Registry) Execute(ctx context.Context, sessionID string, toolCall domain.ToolCall) (*domain.ToolResult, error) {
	tool, ok := r.tools[toolCall.Name]
	if !ok {
		return &domain.ToolResult{
			ToolUseID: toolCall.ID,
			Content:   fmt.Sprintf("unknown tool: %s", toolCall.Name),
			IsError:   true,
		}, nil
	}

	result, err := tool.Execute(ctx, sessionID, toolCall.Input)
	if err != nil {
		return &domain.ToolResult{
			ToolUseID: toolCall.ID,
			Content:   fmt.Sprintf("tool error: %v", err),
			IsError:   true,
		}, nil
	}

	result.ToolUseID = toolCall.ID
	return result, nil
}
