package domain

// ToolContext carries per-call request scope to a tool's Execute method.
// It is populated by the use case (Agent2Execute) for each tool_use the
// LLM emits. Tools should NOT mutate it.
//
// Ports / dependencies are not on ToolContext — those are held by the tool
// itself (constructor injection). ToolContext only carries the per-call
// identifiers the tool needs to look up state, scope catalog reads to a
// tenant, etc.
type ToolContext struct {
	// SessionID is the session whose state the tool should read/write.
	SessionID string
	// TenantSlug scopes catalog / preset / component / field-definition
	// reads to one tenant. Resolved upstream by the HTTP middleware (chunk
	// 6c) or the smoke test wiring (chunk 6b).
	TenantSlug string
	// TurnID groups the deltas / spans this tool emits with the rest of
	// the same Agent2 turn. Empty in chunk 6b smoke tests; chunk 6c wires
	// it from the request id.
	TurnID string
}
