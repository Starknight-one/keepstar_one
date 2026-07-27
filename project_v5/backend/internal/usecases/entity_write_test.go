package usecases

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// ─── fakes ───────────────────────────────────────────────────────────────

// fakeEntityPort is an in-memory ports.EntityPort for the write-path tests.
type fakeEntityPort struct {
	defs        map[string]*domain.EntityDefinition
	sets        []domain.ValueSet
	records     map[string]*domain.EntityRecord
	automations []domain.Automation

	createCalls   int
	createdRec    *domain.EntityRecord
	updatePatch   map[string]any
	transitionTo  string
	transCalls    int
	processedIDs  []int64
	nextEventID   int64
	queryRecords  []domain.EntityRecord
	queryFilter   domain.RecordFilter
	queryTotalOut int
}

var _ ports.EntityPort = (*fakeEntityPort)(nil)

func (f *fakeEntityPort) UpsertEntityDefinition(context.Context, *domain.EntityDefinition) error {
	return nil
}
func (f *fakeEntityPort) GetEntityDefinition(_ context.Context, _, slug string) (*domain.EntityDefinition, error) {
	def, ok := f.defs[slug]
	if !ok {
		return nil, &domain.Error{Code: "ENTITY_NOT_FOUND", Message: "entity definition not found"}
	}
	return def, nil
}
func (f *fakeEntityPort) ListEntityDefinitions(context.Context, string) ([]domain.EntityDefinition, error) {
	return nil, nil
}
func (f *fakeEntityPort) UpsertValueSet(context.Context, *domain.ValueSet) error { return nil }
func (f *fakeEntityPort) GetValueSet(context.Context, string, string) (*domain.ValueSet, error) {
	return nil, nil
}
func (f *fakeEntityPort) ListValueSets(context.Context, string) ([]domain.ValueSet, error) {
	return f.sets, nil
}

func (f *fakeEntityPort) event(eventType, entitySlug, recordID string, payload map[string]any) *domain.RuntimeEvent {
	f.nextEventID++
	return &domain.RuntimeEvent{
		ID: f.nextEventID, TenantID: "tnt-1", EventType: eventType,
		EntitySlug: entitySlug, RecordID: recordID, Payload: payload,
	}
}

func (f *fakeEntityPort) CreateRecord(_ context.Context, rec *domain.EntityRecord) (*domain.EntityRecord, *domain.RuntimeEvent, error) {
	f.createCalls++
	rec.ID = "rec-1"
	f.createdRec = rec
	return rec, f.event(domain.EventRecordCreated, rec.EntitySlug, rec.ID,
		map[string]any{"actor": rec.CreatedBy, "snapshot": rec.Data}), nil
}
func (f *fakeEntityPort) UpdateRecord(_ context.Context, _, recordID string, patch map[string]any) (*domain.EntityRecord, *domain.RuntimeEvent, error) {
	f.updatePatch = patch
	rec := f.records[recordID]
	if rec == nil {
		return nil, nil, &domain.Error{Code: "RECORD_NOT_FOUND", Message: "entity record not found"}
	}
	for k, v := range patch {
		rec.Data[k] = v
	}
	return rec, f.event(domain.EventRecordUpdated, rec.EntitySlug, rec.ID,
		map[string]any{"actor": rec.CreatedBy, "diff": map[string]any{"before": map[string]any{}, "after": rec.Data}}), nil
}
func (f *fakeEntityPort) TransitionStatus(_ context.Context, _, recordID, toStatus string) (*domain.EntityRecord, *domain.RuntimeEvent, error) {
	f.transCalls++
	f.transitionTo = toStatus
	rec := f.records[recordID]
	if rec == nil {
		return nil, nil, &domain.Error{Code: "RECORD_NOT_FOUND", Message: "entity record not found"}
	}
	rec.Status = toStatus
	return rec, f.event(domain.EventRecordStatusChanged, rec.EntitySlug, rec.ID,
		map[string]any{"actor": rec.CreatedBy, "diff": map[string]any{"before": map[string]any{}, "after": rec.Data}}), nil
}
func (f *fakeEntityPort) GetRecord(_ context.Context, _, recordID string) (*domain.EntityRecord, error) {
	rec, ok := f.records[recordID]
	if !ok {
		return nil, &domain.Error{Code: "RECORD_NOT_FOUND", Message: "entity record not found"}
	}
	return rec, nil
}
func (f *fakeEntityPort) QueryRecords(_ context.Context, _, _ string, filter domain.RecordFilter) ([]domain.EntityRecord, int, error) {
	f.queryFilter = filter
	return f.queryRecords, f.queryTotalOut, nil
}
func (f *fakeEntityPort) UpsertAutomation(context.Context, *domain.Automation) error { return nil }
func (f *fakeEntityPort) ListAutomations(_ context.Context, _, eventType string) ([]domain.Automation, error) {
	var out []domain.Automation
	for _, a := range f.automations {
		if a.EventType == eventType {
			out = append(out, a)
		}
	}
	return out, nil
}
func (f *fakeEntityPort) MarkEventProcessed(_ context.Context, eventID int64) error {
	f.processedIDs = append(f.processedIDs, eventID)
	return nil
}

// fakeRefCatalog is a CatalogPort whose GetProduct is configurable (the
// shared fakeCatalog always misses).
type fakeRefCatalog struct {
	fakeCatalog
	product *domain.Product
}

func (c *fakeRefCatalog) GetProduct(_ context.Context, _, _ string) (*domain.Product, error) {
	if c.product == nil {
		return nil, domain.ErrProductNotFound
	}
	return c.product, nil
}

// fakeAutoRunner captures automation dispatches.
type runCall struct {
	tenantID, tenantSlug, op string
	params, payload          map[string]any
}
type fakeAutoRunner struct {
	calls []runCall
	err   error
}

func (r *fakeAutoRunner) Run(_ context.Context, tenantID, tenantSlug, op string, params, payload map[string]any) error {
	r.calls = append(r.calls, runCall{tenantID, tenantSlug, op, params, payload})
	return r.err
}

// ─── helpers ─────────────────────────────────────────────────────────────

func leadEntityPort() *fakeEntityPort {
	return &fakeEntityPort{
		defs: map[string]*domain.EntityDefinition{
			"lead": {
				ID: "def-1", TenantID: "tnt-1", Slug: "lead", Name: "Lead", NamePlural: "Leads",
				Fields: []domain.FieldDef{
					{Key: "name", Label: "Name", Type: domain.FieldText, Required: true},
					{Key: "phone", Label: "Phone", Type: domain.FieldPhone, Required: true},
					{Key: "preferredTime", Label: "Preferred time", Type: domain.FieldDatetime},
					{Key: "listingId", Label: "Listing", Type: domain.FieldRef, RefTarget: "product"},
					{Key: "status", Label: "Status", Type: domain.FieldEnum, ValueSetRef: "lead_pipeline", Default: "new"},
				},
				StatusField: "status",
			},
		},
		sets: []domain.ValueSet{{
			TenantID: "tnt-1", Slug: "lead_pipeline", Name: "Lead pipeline",
			Values: []domain.ValueSetEntry{
				{Value: "new", Label: "New"}, {Value: "contacted", Label: "Contacted"},
			},
		}},
		records: map[string]*domain.EntityRecord{},
	}
}

func newTestEntityWrite(port *fakeEntityPort, cat ports.CatalogPort, runner *fakeAutoRunner) *EntityWrite {
	uc := NewEntityWrite(port, cat, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if runner != nil {
		uc.runner = runner
	}
	return uc
}

// ─── CreateRecord ────────────────────────────────────────────────────────

// The full write path: validation with defaults, catalog-link
// denormalization, tx write, inline automation dispatch with placeholder
// substitution, processed stamp. This is the demo's booking→lead→notify
// chain minus HTTP.
func TestEntityWriteCreateRecordFullPath(t *testing.T) {
	port := leadEntityPort()
	port.automations = []domain.Automation{{
		Name: "notify-on-new-lead", EventType: domain.EventRecordCreated, EntitySlug: "lead",
		Predicate:     &domain.AutomationPredicate{Field: "status", Op: "eq", Value: "new"},
		OperationSlug: "notify",
		OperationParams: map[string]any{
			"title": "Showing request — {refTitle}",
			"body":  "{name} · {phone}",
			"ref":   "{recordId}",
		},
		Enabled: true,
	}}
	cat := &fakeRefCatalog{product: &domain.Product{ID: "lst-1", Name: "Sea View 2BR", Images: []string{"img.jpg"}}}
	runner := &fakeAutoRunner{}
	uc := newTestEntityWrite(port, cat, runner)

	rec, err := uc.CreateRecord(context.Background(), "tnt-1", "acme", "lead", map[string]any{
		"name": "Ann", "phone": "+14155550101", "listingId": "lst-1",
	}, nil, "visitor:sess-1")
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if rec.ID != "rec-1" {
		t.Fatalf("record id = %q", rec.ID)
	}

	got := port.createdRec
	if got.Status != "new" {
		t.Errorf("default status not derived: %q", got.Status)
	}
	if got.Data["refTitle"] != "Sea View 2BR" || got.Data["refImage"] != "img.jpg" {
		t.Errorf("catalog link not denormalized: %#v", got.Data)
	}
	if got.RefEntityType != "product" || got.RefEntityID != "lst-1" {
		t.Errorf("ref not derived from definition: %q/%q", got.RefEntityType, got.RefEntityID)
	}
	if got.CreatedBy != "visitor:sess-1" {
		t.Errorf("createdBy = %q", got.CreatedBy)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("automation dispatches = %d, want 1", len(runner.calls))
	}
	call := runner.calls[0]
	if call.tenantSlug != "acme" || call.op != "notify" {
		t.Errorf("dispatch = %s/%s", call.tenantSlug, call.op)
	}
	// Substitution happens in OperationRunner (tested there) — the
	// dispatcher hands over the RAW params plus the augmented payload.
	if call.params["title"] != "Showing request — {refTitle}" {
		t.Errorf("params must arrive untemplated: %v", call.params["title"])
	}
	if call.payload["recordId"] != "rec-1" {
		t.Errorf("payload not augmented with recordId: %v", call.payload)
	}
	if len(port.processedIDs) != 1 || port.processedIDs[0] != 1 {
		t.Errorf("event not marked processed: %v", port.processedIDs)
	}
}

func TestEntityWriteCreateRecordRejectsInvalidData(t *testing.T) {
	port := leadEntityPort()
	uc := newTestEntityWrite(port, &fakeRefCatalog{}, nil)

	_, err := uc.CreateRecord(context.Background(), "tnt-1", "acme", "lead",
		map[string]any{"name": "Ann"}, nil, "") // phone missing
	var typed *domain.Error
	if !errors.As(err, &typed) || typed.Code != "RECORD_INVALID" {
		t.Fatalf("want RECORD_INVALID, got %v", err)
	}
	if port.createCalls != 0 {
		t.Errorf("invalid data must never reach the port")
	}
}

// A booking against a nonexistent listing is invalid — fail-closed, never
// silently unlinked.
func TestEntityWriteCreateRecordRejectsMissingLinkedProduct(t *testing.T) {
	port := leadEntityPort()
	uc := newTestEntityWrite(port, &fakeRefCatalog{product: nil}, nil)

	_, err := uc.CreateRecord(context.Background(), "tnt-1", "acme", "lead", map[string]any{
		"name": "Ann", "phone": "+14155550101", "listingId": "ghost",
	}, nil, "")
	var typed *domain.Error
	if !errors.As(err, &typed) || typed.Code != "RECORD_INVALID" {
		t.Fatalf("want RECORD_INVALID for missing linked product, got %v", err)
	}
	if port.createCalls != 0 {
		t.Errorf("record must not be created when the link is dead")
	}
}

// ─── DispatchEvent ───────────────────────────────────────────────────────

// Entity and predicate filters gate deterministically; a runner failure on
// one automation never blocks the processed stamp (the record already
// committed — R12 inline dispatch is best-effort).
func TestEntityWriteDispatchEventFiltersAndMarksProcessed(t *testing.T) {
	port := leadEntityPort()
	port.automations = []domain.Automation{
		{Name: "wrong-entity", EventType: domain.EventRecordCreated, EntitySlug: "order",
			OperationSlug: "notify", OperationParams: map[string]any{}},
		{Name: "pred-miss", EventType: domain.EventRecordCreated, EntitySlug: "lead",
			Predicate:     &domain.AutomationPredicate{Field: "status", Op: "neq", Value: "new"},
			OperationSlug: "notify", OperationParams: map[string]any{}},
		{Name: "match", EventType: domain.EventRecordCreated, EntitySlug: "lead",
			Predicate:     &domain.AutomationPredicate{Field: "status", Op: "in", Value: []any{"new", "contacted"}},
			OperationSlug: "notify", OperationParams: map[string]any{}},
	}
	runner := &fakeAutoRunner{err: errors.New("boom")} // even the match fails
	uc := newTestEntityWrite(port, &fakeRefCatalog{}, runner)

	uc.DispatchEvent(context.Background(), "acme", &domain.RuntimeEvent{
		ID: 42, TenantID: "tnt-1", EventType: domain.EventRecordCreated,
		EntitySlug: "lead", RecordID: "rec-9",
		Payload: map[string]any{"actor": "visitor:s", "snapshot": map[string]any{"status": "new"}},
	})

	if len(runner.calls) != 1 || runner.calls[0].op != "notify" {
		t.Fatalf("exactly the matching automation must fire, got %d calls", len(runner.calls))
	}
	if len(port.processedIDs) != 1 || port.processedIDs[0] != 42 {
		t.Errorf("event must be marked processed despite the runner failure: %v", port.processedIDs)
	}
}

// ─── UpdateRecord / TransitionStatus ─────────────────────────────────────

func TestEntityWriteUpdateRecordValidatesPatch(t *testing.T) {
	port := leadEntityPort()
	port.records["rec-1"] = &domain.EntityRecord{
		ID: "rec-1", TenantID: "tnt-1", EntitySlug: "lead", Status: "new",
		Data: map[string]any{"name": "Ann", "phone": "+14155550101", "status": "new"},
	}
	uc := newTestEntityWrite(port, &fakeRefCatalog{}, nil)

	// Bad value → typed violation, no port write.
	_, err := uc.UpdateRecord(context.Background(), "tnt-1", "acme", "rec-1",
		map[string]any{"phone": "not-a-phone"})
	var typed *domain.Error
	if !errors.As(err, &typed) || typed.Code != "RECORD_INVALID" {
		t.Fatalf("want RECORD_INVALID, got %v", err)
	}
	if port.updatePatch != nil {
		t.Fatalf("invalid patch must not reach the port")
	}

	// Unknown keys drop (closed dialect), known keys coerce through.
	if _, err := uc.UpdateRecord(context.Background(), "tnt-1", "acme", "rec-1",
		map[string]any{"phone": "+15551230000", "junk": true}); err != nil {
		t.Fatalf("UpdateRecord: %v", err)
	}
	if _, ok := port.updatePatch["junk"]; ok {
		t.Errorf("undeclared key must be dropped: %#v", port.updatePatch)
	}
	if port.updatePatch["phone"] != "+15551230000" {
		t.Errorf("patch = %#v", port.updatePatch)
	}
}

func TestEntityWriteTransitionStatusHonorsTransitionMap(t *testing.T) {
	port := leadEntityPort()
	port.records["rec-1"] = &domain.EntityRecord{
		ID: "rec-1", TenantID: "tnt-1", EntitySlug: "lead", Status: "new",
		Data: map[string]any{"status": "new"},
	}
	uc := newTestEntityWrite(port, &fakeRefCatalog{}, nil)
	allowed := map[string][]string{"new": {"contacted"}}

	_, err := uc.TransitionStatus(context.Background(), "tnt-1", "acme", "rec-1", "closed", allowed)
	var typed *domain.Error
	if !errors.As(err, &typed) || typed.Code != "INVALID_STATUS" {
		t.Fatalf("want INVALID_STATUS for a disallowed transition, got %v", err)
	}
	if port.transCalls != 0 {
		t.Fatalf("disallowed transition must not reach the port")
	}

	rec, err := uc.TransitionStatus(context.Background(), "tnt-1", "acme", "rec-1", "contacted", allowed)
	if err != nil {
		t.Fatalf("TransitionStatus: %v", err)
	}
	if rec.Status != "contacted" || port.transitionTo != "contacted" {
		t.Errorf("transition not applied: %q / %q", rec.Status, port.transitionTo)
	}
}
