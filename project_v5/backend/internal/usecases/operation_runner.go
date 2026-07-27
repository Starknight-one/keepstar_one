package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// OperationRunner is the automation-facing invoke side of the operation
// plane (RUNTIME_SPEC.md §4.2): a thin adapter over registry.Execute with a
// synthetic system-role context, consumed by EntityWrite.DispatchEvent.
// Going through the registry keeps automation-triggered operations on the
// same choke point as LLM turns and widget invocations: instance
// resolution, config, validation, redaction and the v5_operation_runs
// audit all apply unchanged.
type OperationRunner struct {
	registry ports.OperationRegistry
	log      *slog.Logger
}

var _ automationRunner = (*OperationRunner)(nil)

// automationOperations is the v1 automation-facing allowlist (§4.2):
// notify only. postgres_entity.validateAutomation enforces the same set at
// write time; this is the belt to that suspender.
var automationOperations = map[string]bool{"notify": true}

// NewOperationRunner wires the runner.
func NewOperationRunner(registry ports.OperationRegistry, log *slog.Logger) *OperationRunner {
	if log == nil {
		log = slog.Default()
	}
	return &OperationRunner{registry: registry, log: log}
}

// Run executes one automation-triggered operation: allowlist gate →
// {field} placeholder substitution from the event payload → registry
// Execute under Role system (mode left empty — the registry normalizes to
// its default; an automation belongs to no session form). Any outcome
// other than ok is an error to the dispatcher (logged, never fatal to the
// committed record write).
func (r *OperationRunner) Run(ctx context.Context, tenantID, tenantSlug, operationSlug string, params, eventPayload map[string]any) error {
	if !automationOperations[operationSlug] {
		return fmt.Errorf("operation %q is not automation-runnable (v1: notify only)", operationSlug)
	}

	octx := domain.OperationContext{
		TenantID:   tenantID,
		TenantSlug: tenantSlug,
		Role:       domain.RoleSystem,
		ActorID:    "system:automation",
	}
	result, err := r.registry.Execute(ctx, octx, domain.ToolCall{
		Name:  operationSlug,
		Input: substituteParams(params, substitutionFields(eventPayload)),
	})
	if err != nil {
		return fmt.Errorf("automation %s: %w", operationSlug, err)
	}
	if result.Outcome != domain.OutcomeOK {
		return fmt.Errorf("automation %s: %s", operationSlug, result.Summary)
	}
	return nil
}

// substitutionFields flattens the event payload into the {placeholder}
// vocabulary: record fields (snapshot / diff.after) plus the payload's own
// scalar keys (actor, recordId, entity).
func substitutionFields(eventPayload map[string]any) map[string]any {
	fields := map[string]any{}
	for k, v := range eventPayload {
		switch tv := v.(type) {
		case map[string]any:
			if k == "snapshot" {
				for fk, fv := range tv {
					fields[fk] = fv
				}
			}
			if k == "diff" {
				if after, ok := tv["after"].(map[string]any); ok {
					for fk, fv := range after {
						fields[fk] = fv
					}
				}
			}
		case string, bool, int, int64, float64:
			fields[k] = tv
		}
	}
	return fields
}

// substituteParams returns a copy of params with every {field} placeholder
// in its string values replaced from fields. Unknown placeholders stay
// as-is (visible = debuggable); non-scalar field values never substitute.
func substituteParams(params, fields map[string]any) map[string]any {
	out := make(map[string]any, len(params))
	for k, v := range params {
		s, isString := v.(string)
		if !isString || !strings.Contains(s, "{") {
			out[k] = v
			continue
		}
		for fk, fv := range fields {
			switch fv.(type) {
			case map[string]any, []any:
				continue
			}
			s = strings.ReplaceAll(s, "{"+fk+"}", fmt.Sprintf("%v", fv))
		}
		out[k] = s
	}
	return out
}
