package operations

import (
	"context"
	"errors"
	"fmt"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/operations/seed"
)

// Shared plumbing for the six native executors (RUNTIME_SPEC.md §4.2):
// exec_query / exec_create_record / exec_update_record /
// exec_transition_status / exec_schedule_slot / exec_notify.

// EntityWriter is the write path executors run on (R10: executors call
// usecases.EntityWrite, never ports.EntityPort). Declared consumer-side —
// operations cannot import usecases (the usecases in-package tests import
// operations, which would cycle the test binary); *usecases.EntityWrite
// satisfies it, asserted in the executor tests.
type EntityWriter interface {
	LoadDefinition(ctx context.Context, tenantID, entitySlug string) (*domain.EntityDefinition, map[string]domain.ValueSet, error)
	CreateRecord(ctx context.Context, tenantID, tenantSlug, entitySlug string, data map[string]any, ref *domain.EntityRef, createdBy string) (*domain.EntityRecord, error)
	UpdateRecord(ctx context.Context, tenantID, tenantSlug, recordID string, patch map[string]any) (*domain.EntityRecord, error)
	TransitionStatus(ctx context.Context, tenantID, tenantSlug, recordID, toStatus string, allowed map[string][]string) (*domain.EntityRecord, error)
}

// seedRow returns the §3.1 boot-seed row for one of this package's
// executor templates. The seed package is the single source of the rows
// (names, cards, schemas, gates) — executors carry no duplicate copy that
// could drift. A missing name is a build defect: fail loud at boot.
func seedRow(name string) domain.OperationTemplate {
	for _, t := range seed.Templates() {
		if t.Name == name {
			return t
		}
	}
	panic(fmt.Sprintf("operations: no seed template named %q", name))
}

// entityFailure maps the entity plane's typed errors onto structured
// outcomes: caller-fixable violations → OutcomeInvalid (IsError
// tool_result, the LLM self-corrects); tenant misconfiguration → an
// OutcomeError result (retrying can't help, but the turn survives).
// Unknown errors return (nil, err) — transport failure, agents keep their
// retry path.
func entityFailure(op string, kind domain.OperationKind, err error) (*domain.OperationResult, error) {
	var typed *domain.Error
	if !errors.As(err, &typed) {
		return nil, err
	}
	switch typed.Code {
	case "RECORD_INVALID", "INVALID_STATUS", "RECORD_NOT_FOUND":
		return failure(op, kind, domain.OutcomeInvalid, "invalid: "+typed.Message), nil
	case "ENTITY_NOT_FOUND", "VALUE_SET_NOT_FOUND", "NO_STATUS_FIELD":
		return failure(op, kind, domain.OutcomeError, "error: "+typed.Message), nil
	}
	return nil, err
}

// actorOrSession resolves the createdBy stamp: the caller-supplied actor
// id when present, else the session-anonymous visitor identity
// ("visitor:<session_id>", §3.2 created_by contract).
func actorOrSession(octx domain.OperationContext) string {
	if octx.ActorID != "" {
		return octx.ActorID
	}
	if octx.SessionID != "" {
		return "visitor:" + octx.SessionID
	}
	return ""
}

// ─── instance-config readers (v5_tenant_operations.config JSONB) ─────────

func cfgString(cfg map[string]any, key string) string {
	s, _ := cfg[key].(string)
	return s
}

// cfgBool reads a boolean config key with a default for absence.
func cfgBool(cfg map[string]any, key string, def bool) bool {
	if v, ok := cfg[key].(bool); ok {
		return v
	}
	return def
}

func cfgMap(cfg map[string]any, key string) map[string]any {
	m, _ := cfg[key].(map[string]any)
	return m
}

// cfgStringSlice tolerates []string (Go-built configs) and []any
// (JSONB-decoded configs).
func cfgStringSlice(cfg map[string]any, key string) []string {
	switch v := cfg[key].(type) {
	case []string:
		return v
	case []any:
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

// cfgInt reads a numeric config key (JSONB numbers decode float64).
func cfgInt(cfg map[string]any, key string, def int) int {
	switch v := cfg[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return def
}

// restrictToAllowlist trims a record schema's properties and required list
// to the configured field_allowlist (empty allowlist = full schema).
func restrictToAllowlist(schema map[string]any, allowlist []string) map[string]any {
	if schema == nil || len(allowlist) == 0 {
		return schema
	}
	allowed := make(map[string]bool, len(allowlist))
	for _, k := range allowlist {
		allowed[k] = true
	}
	props, _ := schema["properties"].(map[string]any)
	newProps := make(map[string]any, len(allowed))
	for k, v := range props {
		if allowed[k] {
			newProps[k] = v
		}
	}
	out := map[string]any{"type": "object", "properties": newProps}
	var required []string
	switch req := schema["required"].(type) {
	case []string:
		for _, k := range req {
			if allowed[k] {
				required = append(required, k)
			}
		}
	case []any:
		for _, raw := range req {
			if k, ok := raw.(string); ok && allowed[k] {
				required = append(required, k)
			}
		}
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

// applyDefaults fills configured default values ({"status":"new"}) for
// keys the input left unset. Input keys always win.
func applyDefaults(input, defaults map[string]any) map[string]any {
	if len(defaults) == 0 {
		return input
	}
	out := make(map[string]any, len(input)+len(defaults))
	for k, v := range input {
		out[k] = v
	}
	for k, v := range defaults {
		if _, present := out[k]; !present {
			out[k] = v
		}
	}
	return out
}

// entityDisplayName picks the plural display name for result summaries
// ("2 leads"), falling back through the definition's names to the slug.
func entityDisplayName(def *domain.EntityDefinition, plural bool) string {
	if def == nil {
		return "records"
	}
	if plural && def.NamePlural != "" {
		return def.NamePlural
	}
	if def.Name != "" {
		return def.Name
	}
	return def.Slug
}
