package operations

import (
	"context"
	"strings"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/tools"
)

// Legacy tool wraps (RUNTIME_SPEC.md §4.1 "Legacy wrap"): the four
// pre-registry tools run VERBATIM as registry executors — byte-identical
// wire behavior. The wrapper maps ToolResult → OperationResult; the two
// known "empty:" strings map to OutcomeEmpty HERE — the LAST place that
// prefix is ever read. Summary strings are preserved byte-exact (asserted
// in wrap_legacy_test.go) so conversation-prefix caches stay warm.
//
// Wrapped executors are passthrough: registry input/output validation is
// skipped so behavior (incl. the agents' retry-with-empty-input path)
// cannot drift. Native executors (M2) get full validation.

// DigestSource is the slice of CatalogPort the catalog_search wrap needs
// for its per-tenant schema (CatalogSearchSchemaForDigest pattern).
type DigestSource interface {
	BuildCatalogDigest(ctx context.Context, tenantID string) (*domain.CatalogDigest, error)
}

// LegacyExecutor adapts a tools.Tool to ports.Executor.
type LegacyExecutor struct {
	tool tools.Tool
	tmpl domain.OperationTemplate
	// tenantSchema derives the per-tenant InputSchema (nil result → keep
	// the template's static schema). Only catalog_search sets it.
	tenantSchema func(ctx context.Context, tenant domain.Tenant) map[string]any
}

var _ ports.Executor = (*LegacyExecutor)(nil)

// WrapLegacyTool wraps t under the given v5_operations template row. The
// row's Name/Description/InputSchema should come from t.Definition() so the
// LLM-facing bytes cannot drift from the legacy tool.
func WrapLegacyTool(t tools.Tool, tmpl domain.OperationTemplate) *LegacyExecutor {
	return &LegacyExecutor{tool: t, tmpl: tmpl}
}

// Passthrough marks the wrap for the registry: no input validation, no
// output validation — the legacy tool's own handling is the contract.
func (e *LegacyExecutor) Passthrough() bool { return true }

// TemplateRow returns the full seed row (§3.1) for the boot seeder.
func (e *LegacyExecutor) TemplateRow() domain.OperationTemplate { return e.tmpl }

// Template projects the seed row onto the executable spec surface.
func (e *LegacyExecutor) Template() domain.OperationSpec { return e.tmpl.Spec() }

// SpecForTenant returns the per-tenant spec: the template with its
// InputSchema swapped for the tenant-derived one when a derivation exists
// (catalog_search ← catalog digest). Fail-open to the static schema.
func (e *LegacyExecutor) SpecForTenant(ctx context.Context, tenant domain.Tenant, _ map[string]any) (domain.OperationSpec, error) {
	spec := e.tmpl.Spec()
	if e.tenantSchema != nil {
		if schema := e.tenantSchema(ctx, tenant); schema != nil {
			spec.InputSchema = schema
		}
	}
	return spec, nil
}

// Execute runs the legacy tool verbatim and maps its result.
func (e *LegacyExecutor) Execute(ctx context.Context, octx domain.OperationContext, input map[string]any) (*domain.OperationResult, error) {
	res, err := e.tool.Execute(ctx, domain.ToolContext{
		SessionID:  octx.SessionID,
		TenantSlug: octx.TenantSlug,
		TurnID:     octx.TurnID,
	}, input)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeError, "error: tool returned no result"), nil
	}
	return &domain.OperationResult{
		Operation: e.tmpl.Name,
		Kind:      e.tmpl.Kind,
		Outcome:   legacyOutcome(res),
		Summary:   res.Content, // byte-exact — prompt-cache duty C3
		Metadata:  res.Metadata,
	}, nil
}

// legacyOutcome maps a legacy ToolResult to the structured outcome. The
// "empty:" prefix check lives ONLY here.
func legacyOutcome(res *domain.ToolResult) domain.OpOutcome {
	switch {
	case res.IsError:
		return domain.OutcomeError
	case strings.HasPrefix(res.Content, "empty:"):
		return domain.OutcomeEmpty
	default:
		return domain.OutcomeOK
	}
}

// ─── the four wraps (§3.1 seed rows: legacy wrap rows come from here) ────

// WrapCatalogSearch wraps the catalog search tool (kind query, modes
// storefront+crm per R16 — onboarding never sees it: it would search the
// empty system tenant). digests powers the per-tenant schema; pass the
// catalog port.
func WrapCatalogSearch(t tools.Tool, digests DigestSource) *LegacyExecutor {
	def := t.Definition()
	ex := WrapLegacyTool(t, domain.OperationTemplate{
		Name:        def.Name,
		Kind:        domain.KindQuery,
		Title:       "Catalog search",
		Description: def.Description,
		InputSchema: def.InputSchema,
		Effects: domain.OperationEffects{
			Reads:  []string{"catalog.tenant_search_projection"},
			Writes: []string{"v5_chat_session_state"},
			Emits:  []domain.EmitDecl{},
		},
		Modes:       []domain.PipelineMode{domain.ModeStorefront, domain.ModeCRM},
		Agent:       domain.AgentData,
		MinRole:     domain.RoleVisitor,
		AutoEnabled: true,
	})
	if digests != nil {
		ex.tenantSchema = func(ctx context.Context, tenant domain.Tenant) map[string]any {
			digest, err := digests.BuildCatalogDigest(ctx, tenant.ID)
			if err != nil || digest == nil {
				return nil
			}
			return tools.CatalogSearchSchemaForDigest(digest)
		}
	}
	return ex
}

// WrapStateFilter wraps the in-memory state filter (kind internal).
func WrapStateFilter(t tools.Tool) *LegacyExecutor {
	def := t.Definition()
	return WrapLegacyTool(t, domain.OperationTemplate{
		Name:        def.Name,
		Kind:        domain.KindInternal,
		Title:       "Filter loaded state",
		Description: def.Description,
		InputSchema: def.InputSchema,
		Effects: domain.OperationEffects{
			Reads:  []string{"v5_chat_session_state"},
			Writes: []string{"v5_chat_session_state"},
			Emits:  []domain.EmitDecl{},
		},
		Modes:       []domain.PipelineMode{domain.ModeStorefront, domain.ModeCRM},
		Agent:       domain.AgentData,
		MinRole:     domain.RoleVisitor,
		AutoEnabled: true,
	})
}

// WrapHistoryLookup wraps the session-delta lookup (kind internal).
func WrapHistoryLookup(t tools.Tool) *LegacyExecutor {
	def := t.Definition()
	return WrapLegacyTool(t, domain.OperationTemplate{
		Name:        def.Name,
		Kind:        domain.KindInternal,
		Title:       "Session history lookup",
		Description: def.Description,
		InputSchema: def.InputSchema,
		Effects: domain.OperationEffects{
			Reads:  []string{"v5_chat_session_state"},
			Writes: []string{},
			Emits:  []domain.EmitDecl{},
		},
		Modes:       []domain.PipelineMode{domain.ModeStorefront, domain.ModeCRM},
		Agent:       domain.AgentData,
		MinRole:     domain.RoleVisitor,
		AutoEnabled: true,
	})
}

// WrapVisualAssembly wraps the scene-graph assembly tool (kind visual,
// modes storefront+crm+onboarding per §3.1).
func WrapVisualAssembly(t tools.Tool) *LegacyExecutor {
	def := t.Definition()
	return WrapLegacyTool(t, domain.OperationTemplate{
		Name:        def.Name,
		Kind:        domain.KindVisual,
		Title:       "Visual assembly",
		Description: def.Description,
		InputSchema: def.InputSchema,
		Effects: domain.OperationEffects{
			Reads:  []string{"v5_presets"},
			Writes: []string{"v5_chat_session_state"},
			Emits:  []domain.EmitDecl{},
		},
		Modes:       []domain.PipelineMode{domain.ModeStorefront, domain.ModeCRM, domain.ModeOnboarding},
		Agent:       domain.AgentVisual,
		MinRole:     domain.RoleVisitor,
		AutoEnabled: true,
	})
}
