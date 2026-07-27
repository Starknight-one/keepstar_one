package ports

import (
	"context"

	"keepstar_v5/internal/domain"
)

// Operation plane ports (RUNTIME_SPEC.md §4.1, rulings R1/R8/R14/R30).
// The registry is THE single execution choke point: instance resolution →
// mode+role gate → input validation/coercion → executor → output validation
// → x-sensitive redaction → v5_operation_runs audit.

// Executor is one operation implementation, registered per OperationKind at
// boot (R8: onboarding meta-ops plug in via RegisterExecutor(KindMeta, …)).
type Executor interface {
	// Template returns the executor's static definition — the seeded
	// v5_operations row projected as a spec.
	Template() domain.OperationSpec
	// SpecForTenant derives the per-tenant schema (CatalogSearchSchemaForDigest
	// pattern generalized: schemas derive from catalog digests / entity
	// definitions). Return Template() when tenant-independent.
	SpecForTenant(ctx context.Context, t domain.Tenant, cfg map[string]any) (domain.OperationSpec, error)
	// Execute runs the operation. Input arrives validated+coerced by the
	// registry choke point; the result is structured (no "empty:" sniffing).
	Execute(ctx context.Context, octx domain.OperationContext, input map[string]any) (*domain.OperationResult, error)
}

// OperationRegistry is the surface of operations.OperationRegistry —
// replaces tools.Registry (R8). Implemented by the operations package;
// consumed by both agents, the pipeline and the invoke handler.
type OperationRegistry interface {
	// RegisterExecutor plugs an executor in at boot time.
	RegisterExecutor(kind domain.OperationKind, ex Executor)
	// DefinitionsFor returns the LLM tool definitions visible to one agent
	// plane for a tenant, form (mode) and role — per-tenant schemas arrive
	// materialized, sort is byte-stable (prompt-cache duty C3).
	DefinitionsFor(ctx context.Context, tenantSlug string, mode domain.PipelineMode,
		agent domain.AgentPlane, role domain.Role) []domain.ToolDefinition
	// Get resolves one operation (tenant instance first, else auto-enabled
	// template) to its tenant-derived spec.
	Get(ctx context.Context, tenantSlug, name string) (*domain.OperationSpec, error)
	// Execute is THE choke point. Boundary order (deterministic): resolve
	// instance (miss → denied) → gate Mode ∈ Modes && Role.AtLeast(MinRole) →
	// validate+coerce input (violations → OutcomeInvalid as IsError
	// tool_result) → executor.Execute → validate output (log-only) → redact
	// x-sensitive keys → append v5_operation_runs (always, incl.
	// denied/invalid; best-effort) → return.
	Execute(ctx context.Context, octx domain.OperationContext, call domain.ToolCall) (*domain.OperationResult, error)
	// InvalidateTenant drops the per-tenant spec cache (called by the scoped
	// cache handler, R15, and by OperationStorePort mutators post-commit).
	InvalidateTenant(tenantSlug string)
}

// OperationStorePort persists templates + instances (public.v5_operations /
// v5_tenant_operations). All mutators call registry.InvalidateTenant +
// Agent1PromptCache invalidation post-commit (cache matrix §6.1).
type OperationStorePort interface {
	// SearchLibrary runs pgvector cosine over v5_operations — system
	// templates + preset_pack rows only (R30); FTS fallback when embedding
	// is nil. Returned templates INCLUDE Card + schema summaries (beat-2
	// op-cards).
	SearchLibrary(ctx context.Context, embedding []float32, query string, kinds []string, k int) ([]domain.OperationTemplate, error)
	// EnableOperation creates a tenant instance of a template under an
	// instance wire name; cfg is validated against the template's
	// config_schema on write.
	EnableOperation(ctx context.Context, tenantID, templateName, instanceName string, cfg map[string]any) (*domain.TenantOperation, error)
	UpdateInstanceConfig(ctx context.Context, tenantID, instanceName string, cfg map[string]any) error
	DisableOperation(ctx context.Context, tenantID, instanceName string) error
	ListEnabled(ctx context.Context, tenantID string) ([]domain.TenantOperation, error)
}

// OperationRunPort appends to the public.v5_operation_runs audit log.
// x-sensitive keys are redacted BEFORE Append (R6); best-effort — an audit
// failure never fails the operation.
type OperationRunPort interface {
	Append(ctx context.Context, run domain.OperationRun) error
}
