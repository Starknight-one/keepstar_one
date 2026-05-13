// Package tools houses the executable tools Agent2 invokes via the LLM's
// tool_use mechanism. Each tool implements the Tool interface; the
// Registry indexes tools by name and exposes them in a byte-stable order
// so prompt-cache hashes don't drift between runs.
package tools

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"keepstar_v5/internal/domain"
)

// Tool is the interface every Agent2-callable tool must implement.
//
//   - Definition returns the JSON-Schema declaration the LLM sees in the
//     tools array. Must be stable (same bytes across calls) so prompt
//     caching can match.
//   - Execute runs the tool against the live state. It receives a
//     per-call ToolContext (session id, tenant slug) plus the parsed
//     `input` map from the model's tool_use block.
type Tool interface {
	Definition() domain.ToolDefinition
	Execute(ctx context.Context, toolCtx domain.ToolContext, input map[string]interface{}) (*domain.ToolResult, error)
}

// Registry maps tool names to implementations. Concurrent-safe; tools are
// typically registered once at boot and read many times per request.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry returns an empty Registry. Tools are added via Register.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool. Names must be unique — registering the same name
// twice replaces the prior entry (last-wins) so swapping implementations
// in tests is straightforward.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Definition().Name] = t
}

// GetDefinitions returns all registered tool definitions, sorted by name
// for byte-stable output. The sort matters: Anthropic's prompt-cache key
// is hashed off the tools array verbatim, so any reordering busts the
// cache. V4's tool_registry.go:75 does the same sort for the same reason.
func (r *Registry) GetDefinitions() []domain.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.Definition())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Execute invokes the tool named by call.Name with call.Input. Returns an
// error when the tool isn't registered; tool-side errors land in the
// ToolResult with IsError=true so the caller can surface them to the LLM.
func (r *Registry) Execute(ctx context.Context, toolCtx domain.ToolContext, call domain.ToolCall) (*domain.ToolResult, error) {
	r.mu.RLock()
	tool, ok := r.tools[call.Name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("tool not registered: %s", call.Name)
	}
	res, err := tool.Execute(ctx, toolCtx, call.Input)
	if err != nil {
		return nil, fmt.Errorf("%s execute: %w", call.Name, err)
	}
	if res != nil {
		res.ToolUseID = call.ID
	}
	return res, nil
}
