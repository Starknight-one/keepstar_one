package operations

import (
	"context"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
)

// ─── create_record ───────────────────────────────────────────────────────

func TestCreateRecordExecutorAppliesDefaultsAndWrites(t *testing.T) {
	w := leadWriter()
	ex := NewCreateRecordExecutor(w)

	res, err := ex.Execute(context.Background(), domain.OperationContext{
		TenantID: "tnt-1", TenantSlug: "acme", SessionID: "sess-1", ActorID: "visitor:sess-1",
		Config: map[string]any{"entity": "lead", "defaults": map[string]any{"status": "new"}},
	}, map[string]any{"name": "Ann", "phone": "+14155550101"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeOK || res.RecordID != "rec-1" || res.EntityKind != "lead" {
		t.Fatalf("result = %+v", res)
	}
	if res.Output["recordId"] != "rec-1" {
		t.Errorf("output = %#v", res.Output)
	}
	if w.created.data["status"] != "new" {
		t.Errorf("defaults not applied: %#v", w.created.data)
	}
	if w.created.data["name"] != "Ann" {
		t.Errorf("input lost: %#v", w.created.data)
	}
	if w.created.createdBy != "visitor:sess-1" {
		t.Errorf("createdBy = %q", w.created.createdBy)
	}
	// Ref derivation is EntityWrite's job (from the definition) — the
	// generic create executor passes nil.
	if w.created.ref != nil {
		t.Errorf("create_record must not force a ref: %#v", w.created.ref)
	}
}

func TestCreateRecordExecutorMapsTypedErrors(t *testing.T) {
	w := leadWriter()
	w.err = &domain.Error{Code: "RECORD_INVALID", Message: "phone: not E.164"}
	ex := NewCreateRecordExecutor(w)

	res, err := ex.Execute(context.Background(), domain.OperationContext{
		TenantID: "tnt-1", Config: map[string]any{"entity": "lead"},
	}, map[string]any{"name": "Ann"})
	if err != nil {
		t.Fatalf("typed violations must not be transport errors: %v", err)
	}
	if res.Outcome != domain.OutcomeInvalid || !strings.Contains(res.Summary, "phone: not E.164") {
		t.Errorf("result = %+v", res)
	}
}

func TestCreateRecordExecutorMissingConfigIsError(t *testing.T) {
	ex := NewCreateRecordExecutor(leadWriter())
	res, err := ex.Execute(context.Background(), domain.OperationContext{TenantID: "tnt-1"}, map[string]any{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeError {
		t.Errorf("misconfiguration must be an error outcome, got %q", res.Outcome)
	}
}

// SpecForTenant restricts the derived record schema to the allowlist.
func TestCreateRecordExecutorSpecAllowlist(t *testing.T) {
	ex := NewCreateRecordExecutor(leadWriter())
	spec, err := ex.SpecForTenant(context.Background(), domain.Tenant{ID: "tnt-1"}, map[string]any{
		"entity":          "lead",
		"field_allowlist": []any{"name", "phone"},
	})
	if err != nil {
		t.Fatalf("SpecForTenant: %v", err)
	}
	props, _ := spec.InputSchema["properties"].(map[string]any)
	if len(props) != 2 {
		t.Errorf("allowlist not applied: %v", props)
	}
	if _, ok := props["status"]; ok {
		t.Errorf("status must be excluded by the allowlist")
	}
}

// ─── update_record ───────────────────────────────────────────────────────

func TestUpdateRecordExecutor(t *testing.T) {
	w := leadWriter()
	ex := NewUpdateRecordExecutor(w)
	octx := domain.OperationContext{TenantID: "tnt-1", TenantSlug: "acme", Config: map[string]any{"entity": "lead"}}

	// Missing id → invalid, no write.
	res, err := ex.Execute(context.Background(), octx, map[string]any{"patch": map[string]any{"name": "Bo"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeInvalid {
		t.Errorf("outcome = %q", res.Outcome)
	}

	// OK path.
	res, err = ex.Execute(context.Background(), octx, map[string]any{
		"id": "rec-7", "patch": map[string]any{"name": "Bo"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeOK || res.RecordID != "rec-7" {
		t.Fatalf("result = %+v", res)
	}
	if w.updated.recordID != "rec-7" || w.updated.patch["name"] != "Bo" {
		t.Errorf("write = %+v", w.updated)
	}
}

// ─── transition_status ───────────────────────────────────────────────────

func TestTransitionStatusExecutor(t *testing.T) {
	w := leadWriter()
	ex := NewTransitionStatusExecutor(w)
	octx := domain.OperationContext{TenantID: "tnt-1", TenantSlug: "acme", Config: map[string]any{
		"entity": "lead", "field": "status", "value_set": "lead_pipeline",
		// JSONB-decoded transitions map shape.
		"transitions": map[string]any{"new": []any{"contacted"}},
	}}

	res, err := ex.Execute(context.Background(), octx, map[string]any{"id": "rec-7", "to_status": "contacted"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeOK || res.Output["status"] != "contacted" {
		t.Fatalf("result = %+v", res)
	}
	if w.transitioned.toStatus != "contacted" {
		t.Errorf("write = %+v", w.transitioned)
	}
	if got := w.transitioned.allowed["new"]; len(got) != 1 || got[0] != "contacted" {
		t.Errorf("transitions map not decoded: %#v", w.transitioned.allowed)
	}
	if !strings.Contains(res.Summary, "moved to contacted") {
		t.Errorf("summary = %q", res.Summary)
	}
}

func TestTransitionStatusExecutorMapsInvalidStatus(t *testing.T) {
	w := leadWriter()
	w.err = &domain.Error{Code: "INVALID_STATUS", Message: `status "closed" is not in value set lead_pipeline`}
	ex := NewTransitionStatusExecutor(w)

	res, err := ex.Execute(context.Background(), domain.OperationContext{
		TenantID: "tnt-1", Config: map[string]any{"entity": "lead", "field": "status", "value_set": "lead_pipeline"},
	}, map[string]any{"id": "rec-7", "to_status": "closed"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeInvalid || !strings.Contains(res.Summary, "not in value set") {
		t.Errorf("result = %+v", res)
	}
}

// SpecForTenant enum-types to_status from the configured value set.
func TestTransitionStatusExecutorSpecEnum(t *testing.T) {
	ex := NewTransitionStatusExecutor(leadWriter())
	spec, err := ex.SpecForTenant(context.Background(), domain.Tenant{ID: "tnt-1"}, map[string]any{
		"entity": "lead", "field": "status", "value_set": "lead_pipeline",
	})
	if err != nil {
		t.Fatalf("SpecForTenant: %v", err)
	}
	props, _ := spec.InputSchema["properties"].(map[string]any)
	toStatus, _ := props["to_status"].(map[string]any)
	enum, _ := toStatus["enum"].([]string)
	if len(enum) != 4 || enum[1] != "contacted" {
		t.Errorf("to_status enum = %v", enum)
	}
}
