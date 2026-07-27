// Package tools houses the executable tools the agents invoke via the
// LLM's tool_use mechanism. Each tool implements the Tool interface.
//
// Execution and visibility moved to the operation registry
// (internal/operations, RUNTIME_SPEC.md R8): legacy tools are wrapped as
// registry executors via operations.WrapLegacyTool and no longer run
// through a package-local registry — tools.Registry is deleted.
package tools

import (
	"context"

	"keepstar_v5/internal/domain"
)

// Tool is the interface every legacy LLM-callable tool implements.
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
