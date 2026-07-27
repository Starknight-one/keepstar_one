package operations

import (
	"context"
	"strings"
	"testing"
	"time"

	"keepstar_v5/internal/domain"
)

// fixedNow anchors the R11 guards for determinism.
var fixedNow = time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)

func newScheduleExecutor(w *fakeWriter) *ScheduleSlotExecutor {
	ex := NewScheduleSlotExecutor(w)
	ex.now = func() time.Time { return fixedNow }
	return ex
}

func bookingConfig() map[string]any {
	return map[string]any{
		"entity":         "lead",
		"datetime_field": "preferredTime",
		"link_field":     "listingId",
		"reject_past":    true,
		// JSONB-decoded shape: numbers arrive as float64.
		"hours":    map[string]any{"from": float64(9), "to": float64(18)},
		"defaults": map[string]any{"status": "new"},
	}
}

func bookingInput(preferredTime string) map[string]any {
	return map[string]any{
		"name":          "Ann",
		"phone":         "+14155550101",
		"preferredTime": preferredTime,
		"listingId":     "lst-1",
	}
}

// The R11 deterministic guards, table-driven (spec M2.2): past and
// out-of-hours datetimes are invalid with a human-readable reason and
// never reach the write path; valid slots create the record.
func TestScheduleSlotDatetimeGuards(t *testing.T) {
	cases := []struct {
		name        string
		config      map[string]any
		input       map[string]any
		wantOutcome domain.OpOutcome
		wantInSum   string
		wantWrites  int
	}{
		{
			name:        "past datetime rejected",
			config:      bookingConfig(),
			input:       bookingInput("2020-01-01T10:00:00Z"),
			wantOutcome: domain.OutcomeInvalid,
			wantInSum:   "in the past",
			wantWrites:  0,
		},
		{
			name:        "past rejected by default when reject_past absent",
			config:      map[string]any{"entity": "lead", "datetime_field": "preferredTime"},
			input:       bookingInput("2020-01-01T10:00:00Z"),
			wantOutcome: domain.OutcomeInvalid,
			wantInSum:   "in the past",
			wantWrites:  0,
		},
		{
			name:        "before opening hour rejected",
			config:      bookingConfig(),
			input:       bookingInput("2026-08-03T08:59:00Z"),
			wantOutcome: domain.OutcomeInvalid,
			wantInSum:   "outside business hours (09:00-18:00)",
			wantWrites:  0,
		},
		{
			name:        "at closing hour rejected",
			config:      bookingConfig(),
			input:       bookingInput("2026-08-03T18:00:00Z"),
			wantOutcome: domain.OutcomeInvalid,
			wantInSum:   "outside business hours",
			wantWrites:  0,
		},
		{
			name:        "valid slot books",
			config:      bookingConfig(),
			input:       bookingInput("2026-08-03T15:00:00Z"),
			wantOutcome: domain.OutcomeOK,
			wantInSum:   "ok: booked lead rec-1",
			wantWrites:  1,
		},
		{
			name:        "no hours window accepts evenings",
			config:      map[string]any{"entity": "lead", "datetime_field": "preferredTime"},
			input:       bookingInput("2026-08-03T21:30:00Z"),
			wantOutcome: domain.OutcomeOK,
			wantInSum:   "ok: booked",
			wantWrites:  1,
		},
		{
			name:        "missing datetime invalid",
			config:      bookingConfig(),
			input:       map[string]any{"name": "Ann", "phone": "+14155550101", "listingId": "lst-1"},
			wantOutcome: domain.OutcomeInvalid,
			wantInSum:   "preferredTime is required",
			wantWrites:  0,
		},
		{
			name:        "unparseable datetime invalid",
			config:      bookingConfig(),
			input:       bookingInput("tomorrow at 3"),
			wantOutcome: domain.OutcomeInvalid,
			wantInSum:   "not an ISO-8601 datetime",
			wantWrites:  0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := leadWriter()
			ex := newScheduleExecutor(w)
			res, err := ex.Execute(context.Background(),
				domain.OperationContext{TenantID: "tnt-1", TenantSlug: "acme", SessionID: "sess-1", Config: tc.config},
				tc.input)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if res.Outcome != tc.wantOutcome {
				t.Fatalf("outcome = %q (%s), want %q", res.Outcome, res.Summary, tc.wantOutcome)
			}
			if !strings.Contains(res.Summary, tc.wantInSum) {
				t.Errorf("summary %q does not contain %q", res.Summary, tc.wantInSum)
			}
			if w.created.calls != tc.wantWrites {
				t.Errorf("writer calls = %d, want %d", w.created.calls, tc.wantWrites)
			}
		})
	}
}

// A successful booking = create_record semantics: defaults applied, the
// catalog link derived from link_field, the session-anonymous actor
// stamped, the record id surfaced for the success plaque.
func TestScheduleSlotCreatesLinkedRecord(t *testing.T) {
	w := leadWriter()
	ex := newScheduleExecutor(w)

	res, err := ex.Execute(context.Background(),
		domain.OperationContext{TenantID: "tnt-1", TenantSlug: "acme", SessionID: "sess-1", Config: bookingConfig()},
		bookingInput("2026-08-03T15:00:00Z"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeOK || res.RecordID != "rec-1" || res.EntityKind != "lead" {
		t.Fatalf("result = %+v", res)
	}
	if res.Output["recordId"] != "rec-1" {
		t.Errorf("output = %#v", res.Output)
	}

	if w.created.entity != "lead" || w.created.tenantSlug != "acme" {
		t.Errorf("write target = %s/%s", w.created.tenantSlug, w.created.entity)
	}
	if w.created.data["status"] != "new" {
		t.Errorf("config default not applied: %#v", w.created.data)
	}
	if w.created.ref == nil || w.created.ref.ID != "lst-1" || w.created.ref.Type != domain.EntityTypeProduct {
		t.Errorf("catalog link not derived from link_field: %#v", w.created.ref)
	}
	if w.created.createdBy != "visitor:sess-1" {
		t.Errorf("createdBy = %q", w.created.createdBy)
	}
}

// SpecForTenant derives the booking schema from the definition and forces
// the datetime + link fields required.
func TestScheduleSlotSpecForTenantForcesRequired(t *testing.T) {
	w := leadWriter()
	ex := newScheduleExecutor(w)

	spec, err := ex.SpecForTenant(context.Background(), domain.Tenant{ID: "tnt-1", Slug: "acme"}, bookingConfig())
	if err != nil {
		t.Fatalf("SpecForTenant: %v", err)
	}
	req, _ := spec.InputSchema["required"].([]string)
	want := map[string]bool{"name": false, "phone": false, "preferredTime": false, "listingId": false}
	for _, k := range req {
		if _, ok := want[k]; ok {
			want[k] = true
		}
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("required must include %q, got %v", k, req)
		}
	}
	props, _ := spec.InputSchema["properties"].(map[string]any)
	if _, ok := props["preferredTime"]; !ok {
		t.Errorf("schema lost the datetime field: %v", props)
	}
}
