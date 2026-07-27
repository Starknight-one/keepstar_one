//go:build integration

// Run with: TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration ./internal/adapters/postgres/...
//
// Exercises the entity-plane adapter against a live DB: definition CRUD +
// additive-only guardrails (field removal / type change / slug rename once
// records exist), value-set round-trip incl. entry order, the record
// lifecycle with same-tx v5_events outbox rows asserted BY VALUE (real
// green: the event row is read back and matched to the record), transition
// value-set validation, filtered queries incl. the identifier gate, the
// automation upsert/list cycle, MarkEventProcessed and the notification
// write. All rows live under a random tenant UUID and are deleted in
// cleanup.

package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"keepstar_v5/internal/domain"
)

func setupEntityClient(t *testing.T) (*Client, *EntityAdapter, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	c, err := NewClient(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(c.Close)
	if err := c.RunEntityMigrations(ctx); err != nil {
		t.Fatalf("migrate entity: %v", err)
	}
	tenantID := testTenantUUID(t, c)
	t.Cleanup(func() {
		ctx := context.Background()
		for _, table := range []string{
			"v5_notifications", "v5_events", "v5_automations",
			"v5_entity_records", "v5_entity_definitions", "v5_value_sets",
		} {
			_, _ = c.pool.Exec(ctx, `DELETE FROM `+table+` WHERE tenant_id = $1::uuid`, tenantID)
		}
	})
	return c, NewEntityAdapter(c), tenantID
}

func leadPipeline(tenantID string) *domain.ValueSet {
	return &domain.ValueSet{
		TenantID: tenantID, Slug: "lead_pipeline", Name: "Lead pipeline",
		Values: []domain.ValueSetEntry{
			{Value: "new", Label: "New", Color: "#5BA4D9"},
			{Value: "contacted", Label: "Contacted"},
			{Value: "showing_booked", Label: "Showing booked"},
			{Value: "closed", Label: "Closed"},
		},
	}
}

func leadDef(tenantID string) *domain.EntityDefinition {
	return &domain.EntityDefinition{
		TenantID: tenantID, Slug: "lead", Name: "Lead", NamePlural: "Leads",
		StatusField: "status",
		Display:     domain.EntityDisplay{TitleField: "name", BadgeField: "status"},
		Fields: []domain.FieldDef{
			{Key: "name", Label: "Name", Type: domain.FieldText, Required: true},
			{Key: "phone", Label: "Phone", Type: domain.FieldPhone, Required: true},
			{Key: "status", Label: "Status", Type: domain.FieldEnum, ValueSetRef: "lead_pipeline", Default: "new"},
			{Key: "beds", Label: "Bedrooms", Type: domain.FieldNumber, Unit: domain.UnitRooms},
			{Key: "preferredTime", Label: "Preferred time", Type: domain.FieldDatetime},
			{Key: "listingId", Label: "Listing", Type: domain.FieldRef, RefTarget: "product"},
		},
	}
}

func mustUpsertLead(t *testing.T, a *EntityAdapter, tenantID string) *domain.EntityDefinition {
	t.Helper()
	ctx := context.Background()
	if err := a.UpsertValueSet(ctx, leadPipeline(tenantID)); err != nil {
		t.Fatalf("upsert value set: %v", err)
	}
	def := leadDef(tenantID)
	if err := a.UpsertEntityDefinition(ctx, def); err != nil {
		t.Fatalf("upsert definition: %v", err)
	}
	return def
}

func TestEntityDefinitionRoundTripAndGuardrails(t *testing.T) {
	_, a, tenantID := setupEntityClient(t)
	ctx := context.Background()
	def := mustUpsertLead(t, a, tenantID)
	if def.ID == "" {
		t.Fatal("upsert did not fill def.ID")
	}

	got, err := a.GetEntityDefinition(ctx, tenantID, "lead")
	if err != nil {
		t.Fatalf("get definition: %v", err)
	}
	if got.ID != def.ID || got.Name != "Lead" || got.NamePlural != "Leads" || got.StatusField != "status" {
		t.Errorf("definition round-trip broken: %+v", got)
	}
	if len(got.Fields) != 6 || got.Fields[0].Key != "name" || got.Fields[3].Unit != domain.UnitRooms {
		t.Errorf("fields order/units broken: %+v", got.Fields)
	}
	if got.Display.TitleField != "name" {
		t.Errorf("display round-trip broken: %+v", got.Display)
	}

	// Idempotent re-upsert on the natural key: same row id.
	again := leadDef(tenantID)
	if err := a.UpsertEntityDefinition(ctx, again); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if again.ID != def.ID {
		t.Fatalf("upsert not idempotent: %s != %s", again.ID, def.ID)
	}

	// Invalid shapes rejected before SQL.
	for name, bad := range map[string]*domain.EntityDefinition{
		"snake key": {TenantID: tenantID, Slug: "bad1", Name: "B", Fields: []domain.FieldDef{
			{Key: "deal_type", Label: "x", Type: domain.FieldText}}},
		"unknown type": {TenantID: tenantID, Slug: "bad2", Name: "B", Fields: []domain.FieldDef{
			{Key: "x", Label: "x", Type: "formula"}}},
		"enum without set": {TenantID: tenantID, Slug: "bad3", Name: "B", Fields: []domain.FieldDef{
			{Key: "x", Label: "x", Type: domain.FieldEnum}}},
		"unit on text": {TenantID: tenantID, Slug: "bad4", Name: "B", Fields: []domain.FieldDef{
			{Key: "x", Label: "x", Type: domain.FieldText, Unit: domain.UnitM2}}},
		"status field undeclared": {TenantID: tenantID, Slug: "bad5", Name: "B", StatusField: "nope",
			Fields: []domain.FieldDef{{Key: "x", Label: "x", Type: domain.FieldText}}},
	} {
		if err := a.UpsertEntityDefinition(ctx, bad); err == nil {
			t.Errorf("%s: accepted invalid definition", name)
		}
	}

	// Additive while no records exist: field removal is still allowed…
	shrunk := leadDef(tenantID)
	shrunk.Fields = shrunk.Fields[:5] // drop listingId — no records yet
	if err := a.UpsertEntityDefinition(ctx, shrunk); err != nil {
		t.Fatalf("pre-records field drop should pass: %v", err)
	}
	if err := a.UpsertEntityDefinition(ctx, leadDef(tenantID)); err != nil {
		t.Fatalf("restore fields: %v", err)
	}

	// …then a record arrives and the definition freezes.
	rec, _, err := a.CreateRecord(ctx, &domain.EntityRecord{
		TenantID: tenantID, EntitySlug: "lead",
		Data: map[string]any{"name": "Ana", "phone": "+5511998765432", "status": "new"},
	})
	if err != nil {
		t.Fatalf("create record: %v", err)
	}
	_ = rec

	removal := leadDef(tenantID)
	removal.Fields = removal.Fields[:5]
	if err := a.UpsertEntityDefinition(ctx, removal); err == nil {
		t.Error("field removal accepted with records present")
	}
	typeChange := leadDef(tenantID)
	typeChange.Fields[3].Type = domain.FieldText // beds number→text
	if err := a.UpsertEntityDefinition(ctx, typeChange); err == nil {
		t.Error("type change accepted with records present")
	}
	rename := leadDef(tenantID)
	rename.ID = def.ID
	rename.Slug = "prospect"
	if err := a.UpsertEntityDefinition(ctx, rename); err == nil {
		t.Error("slug rename accepted with records present")
	} else if !strings.Contains(err.Error(), "rename") {
		t.Errorf("rename rejection unclear: %v", err)
	}

	// Additive extension stays open: a NEW field is fine.
	extended := leadDef(tenantID)
	extended.Fields = append(extended.Fields, domain.FieldDef{Key: "note", Label: "Note", Type: domain.FieldText})
	if err := a.UpsertEntityDefinition(ctx, extended); err != nil {
		t.Errorf("additive field extension rejected: %v", err)
	}

	list, err := a.ListEntityDefinitions(ctx, tenantID)
	if err != nil {
		t.Fatalf("list definitions: %v", err)
	}
	if len(list) != 1 || list[0].Slug != "lead" {
		t.Errorf("list wrong: %+v", list)
	}
	if _, err := a.GetEntityDefinition(ctx, tenantID, "ghost"); !errors.Is(err, ErrEntityDefinitionNotFound) {
		t.Errorf("want ErrEntityDefinitionNotFound, got %v", err)
	}
}

func TestValueSetRoundTrip(t *testing.T) {
	_, a, tenantID := setupEntityClient(t)
	ctx := context.Background()
	vs := leadPipeline(tenantID)
	if err := a.UpsertValueSet(ctx, vs); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	firstID := vs.ID

	got, err := a.GetValueSet(ctx, tenantID, "lead_pipeline")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Values) != 4 || got.Values[0].Value != "new" || got.Values[0].Color != "#5BA4D9" ||
		got.Values[2].Value != "showing_booked" {
		t.Errorf("entry order/shape broken (R27): %+v", got.Values)
	}

	// Idempotent upsert keeps the row; entries replace wholesale.
	vs2 := leadPipeline(tenantID)
	vs2.Values = append(vs2.Values, domain.ValueSetEntry{Value: "lost", Label: "Lost"})
	if err := a.UpsertValueSet(ctx, vs2); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if vs2.ID != firstID {
		t.Errorf("upsert not idempotent: %s != %s", vs2.ID, firstID)
	}
	got, _ = a.GetValueSet(ctx, tenantID, "lead_pipeline")
	if len(got.Values) != 5 || got.Values[4].Value != "lost" {
		t.Errorf("entry update broken: %+v", got.Values)
	}

	if err := a.UpsertValueSet(ctx, &domain.ValueSet{TenantID: tenantID, Slug: "dup", Name: "D",
		Values: []domain.ValueSetEntry{{Value: "a", Label: "A"}, {Value: "a", Label: "A2"}}}); err == nil {
		t.Error("duplicate values accepted")
	}
	if _, err := a.GetValueSet(ctx, tenantID, "ghost"); !errors.Is(err, ErrValueSetNotFound) {
		t.Errorf("want ErrValueSetNotFound, got %v", err)
	}
}

func TestRecordLifecycleWithOutbox(t *testing.T) {
	c, a, tenantID := setupEntityClient(t)
	ctx := context.Background()
	mustUpsertLead(t, a, tenantID)

	rec, ev, err := a.CreateRecord(ctx, &domain.EntityRecord{
		TenantID: tenantID, EntitySlug: "lead",
		Data: map[string]any{"name": "Ana", "phone": "+5511998765432", "status": "new",
			"beds": float64(2)},
		RefEntityType: "product",
		CreatedBy:     "visitor:sess-1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if rec.ID == "" || rec.EntityDefinitionID == "" {
		t.Fatalf("record ids not filled: %+v", rec)
	}
	if rec.Status != "new" {
		t.Errorf("status mirror not derived from data: %q", rec.Status)
	}

	// Outbox row written in the SAME tx — read it back by value.
	if ev == nil || ev.ID == 0 {
		t.Fatal("no runtime event returned")
	}
	var (
		evType, evSlug, evRecordID string
		processed                  *time.Time
	)
	err = c.pool.QueryRow(ctx, `
		SELECT event_type, COALESCE(entity_slug, ''), COALESCE(record_id::text, ''), processed_at
		FROM v5_events WHERE id = $1
	`, ev.ID).Scan(&evType, &evSlug, &evRecordID, &processed)
	if err != nil {
		t.Fatalf("event row missing: %v", err)
	}
	if evType != domain.EventRecordCreated || evSlug != "lead" || evRecordID != rec.ID {
		t.Errorf("event row wrong: %s %s %s", evType, evSlug, evRecordID)
	}
	if processed != nil {
		t.Error("fresh event must be unprocessed")
	}
	snapshot, _ := ev.Payload["snapshot"].(map[string]any)
	if snapshot == nil || snapshot["name"] != "Ana" {
		t.Errorf("payload snapshot wrong: %+v", ev.Payload)
	}

	// Update merges the patch, re-mirrors status, emits record.updated.
	updated, upEv, err := a.UpdateRecord(ctx, tenantID, rec.ID, map[string]any{
		"beds": float64(3), "status": "contacted",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Status != "contacted" {
		t.Errorf("status mirror not updated: %q", updated.Status)
	}
	if updated.Data["name"] != "Ana" || updated.Data["beds"] != float64(3) {
		t.Errorf("merge broken: %+v", updated.Data)
	}
	if upEv.EventType != domain.EventRecordUpdated {
		t.Errorf("update event type: %s", upEv.EventType)
	}
	diff, _ := upEv.Payload["diff"].(map[string]any)
	if diff == nil {
		t.Fatalf("update payload missing diff: %+v", upEv.Payload)
	}

	// Transition: value-set validated. Invalid value → typed error, NO event.
	var eventsBefore int
	_ = c.pool.QueryRow(ctx, `SELECT COUNT(*) FROM v5_events WHERE tenant_id = $1::uuid`, tenantID).Scan(&eventsBefore)
	if _, _, err := a.TransitionStatus(ctx, tenantID, rec.ID, "vaporized"); err == nil {
		t.Fatal("out-of-set status accepted")
	} else {
		var de *domain.Error
		if !errors.As(err, &de) || de.Code != "INVALID_STATUS" {
			t.Errorf("want INVALID_STATUS, got %v", err)
		}
		if !strings.Contains(err.Error(), "showing_booked") {
			t.Errorf("error must list allowed values: %v", err)
		}
	}
	var eventsAfter int
	_ = c.pool.QueryRow(ctx, `SELECT COUNT(*) FROM v5_events WHERE tenant_id = $1::uuid`, tenantID).Scan(&eventsAfter)
	if eventsAfter != eventsBefore {
		t.Error("rejected transition still wrote an event")
	}

	moved, trEv, err := a.TransitionStatus(ctx, tenantID, rec.ID, "showing_booked")
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if moved.Status != "showing_booked" || moved.Data["status"] != "showing_booked" {
		t.Errorf("status column and data mirror diverge: %q vs %v", moved.Status, moved.Data["status"])
	}
	if trEv.EventType != domain.EventRecordStatusChanged {
		t.Errorf("transition event type: %s", trEv.EventType)
	}

	// MarkEventProcessed stamps once, idempotently.
	if err := a.MarkEventProcessed(ctx, trEv.ID); err != nil {
		t.Fatalf("mark processed: %v", err)
	}
	if err := a.MarkEventProcessed(ctx, trEv.ID); err != nil {
		t.Fatalf("second mark must be a no-op: %v", err)
	}
	err = c.pool.QueryRow(ctx, `SELECT processed_at FROM v5_events WHERE id = $1`, trEv.ID).Scan(&processed)
	if err != nil || processed == nil {
		t.Errorf("processed_at not stamped: %v %v", err, processed)
	}

	if _, err := a.GetRecord(ctx, tenantID, "not-a-uuid"); !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("junk id must map to ErrRecordNotFound, got %v", err)
	}
}

func TestQueryRecordsFilters(t *testing.T) {
	c, a, tenantID := setupEntityClient(t)
	ctx := context.Background()
	mustUpsertLead(t, a, tenantID)

	listing := testTenantUUID(t, c) // any UUID works as a ref target here
	seed := []struct {
		name, status string
		beds         float64
		ref          string
	}{
		{"Ana", "new", 2, listing},
		{"Bruno", "new", 3, ""},
		{"Clara", "contacted", 2, ""},
		{"Diego", "closed", 4, ""},
	}
	for _, s := range seed {
		rec := &domain.EntityRecord{
			TenantID: tenantID, EntitySlug: "lead",
			Data: map[string]any{"name": s.name, "phone": "+5511998765432",
				"status": s.status, "beds": s.beds},
		}
		if s.ref != "" {
			rec.RefEntityType, rec.RefEntityID = "product", s.ref
		}
		if _, _, err := a.CreateRecord(ctx, rec); err != nil {
			t.Fatalf("seed %s: %v", s.name, err)
		}
	}

	// Status filter.
	recs, total, err := a.QueryRecords(ctx, tenantID, "lead", domain.RecordFilter{Status: "new"})
	if err != nil {
		t.Fatalf("status query: %v", err)
	}
	if total != 2 || len(recs) != 2 {
		t.Errorf("status=new: total %d rows %d", total, len(recs))
	}

	// Generic attr equality (identifier-gated) + numeric bound.
	recs, total, err = a.QueryRecords(ctx, tenantID, "lead", domain.RecordFilter{
		Attrs:   map[string]string{"name": "Ana", "bad;key": "x"},
		AttrMin: map[string]float64{"beds": 2},
	})
	if err != nil {
		t.Fatalf("attr query: %v", err)
	}
	if total != 1 || len(recs) != 1 || recs[0].Data["name"] != "Ana" {
		t.Errorf("attr query wrong: total %d %+v", total, recs)
	}

	// Numeric range excludes.
	_, total, err = a.QueryRecords(ctx, tenantID, "lead", domain.RecordFilter{
		AttrMin: map[string]float64{"beds": 3}, AttrMax: map[string]float64{"beds": 4},
	})
	if err != nil || total != 2 {
		t.Errorf("beds 3-4: total %d err %v", total, err)
	}

	// Ref filter.
	recs, total, err = a.QueryRecords(ctx, tenantID, "lead", domain.RecordFilter{RefEntityID: listing})
	if err != nil || total != 1 || recs[0].RefEntityID != listing {
		t.Errorf("ref filter: total %d err %v", total, err)
	}
	if _, _, err := a.QueryRecords(ctx, tenantID, "lead", domain.RecordFilter{RefEntityID: "junk"}); err == nil {
		t.Error("junk refEntityId accepted")
	}

	// Sort on a data key + paging; total stays the full filter count.
	recs, total, err = a.QueryRecords(ctx, tenantID, "lead", domain.RecordFilter{
		SortField: "name", SortOrder: "asc", Limit: 2, Offset: 2,
	})
	if err != nil {
		t.Fatalf("paged query: %v", err)
	}
	if total != 4 || len(recs) != 2 || recs[0].Data["name"] != "Clara" {
		t.Errorf("paging wrong: total %d first %v", total, recs[0].Data["name"])
	}

	// Wrong slug isolates.
	_, total, err = a.QueryRecords(ctx, tenantID, "ghost", domain.RecordFilter{})
	if err != nil || total != 0 {
		t.Errorf("ghost entity: total %d err %v", total, err)
	}
}

func TestAutomationUpsertAndList(t *testing.T) {
	_, a, tenantID := setupEntityClient(t)
	ctx := context.Background()

	au := &domain.Automation{
		TenantID: tenantID, Name: "notify_agent_on_lead",
		EventType: domain.EventRecordCreated, EntitySlug: "lead",
		Predicate:     &domain.AutomationPredicate{Field: "status", Op: "eq", Value: "new"},
		OperationSlug: "notify",
		OperationParams: map[string]any{
			"title": "Showing request — {refTitle}", "body": "{name}, {phone}",
		},
		Enabled: true,
	}
	if err := a.UpsertAutomation(ctx, au); err != nil {
		t.Fatalf("insert: %v", err)
	}
	firstID := au.ID

	// Natural-key upsert: same name updates in place.
	au2 := *au
	au2.ID = ""
	au2.Predicate = nil
	if err := a.UpsertAutomation(ctx, &au2); err != nil {
		t.Fatalf("update: %v", err)
	}
	if au2.ID != firstID {
		t.Errorf("upsert not idempotent: %s != %s", au2.ID, firstID)
	}

	list, err := a.ListAutomations(ctx, tenantID, domain.EventRecordCreated)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Predicate != nil || list[0].OperationParams["title"] != "Showing request — {refTitle}" {
		t.Errorf("list wrong: %+v", list)
	}
	if l, _ := a.ListAutomations(ctx, tenantID, domain.EventRecordStatusChanged); len(l) != 0 {
		t.Errorf("event-type filter broken: %+v", l)
	}

	// Guardrails: closed trigger set, notify-only allowlist, flat predicate ops.
	for name, bad := range map[string]*domain.Automation{
		"updated trigger": {TenantID: tenantID, Name: "x", EventType: domain.EventRecordUpdated, OperationSlug: "notify"},
		"non-notify":      {TenantID: tenantID, Name: "x", EventType: domain.EventRecordCreated, OperationSlug: "send_email"},
		"bad predicate op": {TenantID: tenantID, Name: "x", EventType: domain.EventRecordCreated, OperationSlug: "notify",
			Predicate: &domain.AutomationPredicate{Field: "status", Op: "gt", Value: 1}},
	} {
		if err := a.UpsertAutomation(ctx, bad); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	// Disabled drops out of the dispatch list.
	au3 := *au
	au3.ID, au3.Enabled = "", false
	if err := a.UpsertAutomation(ctx, &au3); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if l, _ := a.ListAutomations(ctx, tenantID, domain.EventRecordCreated); len(l) != 0 {
		t.Errorf("disabled automation still listed: %+v", l)
	}
}

func TestNotificationCreate(t *testing.T) {
	c, _, tenantID := setupEntityClient(t)
	ctx := context.Background()
	store := NewNotificationAdapter(c)

	n := &domain.Notification{
		TenantID: tenantID,
		Title:    "Showing request — Sunset Loft",
		Body:     "Ana, +5511998765432",
	}
	if err := store.CreateNotification(ctx, n); err != nil {
		t.Fatalf("create: %v", err)
	}
	if n.ID == "" || n.Audience != "crm" {
		t.Errorf("defaults not applied: id %q audience %q", n.ID, n.Audience)
	}
	var title string
	if err := c.pool.QueryRow(ctx, `
		SELECT title FROM v5_notifications WHERE id = $1::uuid
	`, n.ID).Scan(&title); err != nil || title != n.Title {
		t.Errorf("row not written: %v %q", err, title)
	}

	if err := store.CreateNotification(ctx, &domain.Notification{TenantID: tenantID}); err == nil {
		t.Error("titleless notification accepted")
	}
}
