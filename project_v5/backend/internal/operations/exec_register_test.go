package operations

import (
	"context"
	"testing"
	"time"

	"keepstar_v5/internal/domain"
)

// End-to-end through THE choke point: the six executors registered, seed
// rows seeded, demo instances enabled — book_showing resolves, its
// per-tenant schema validates and unit-coerces the input (dollars→cents),
// the guard passes and the write path fires. Plus the R14 ladder: a
// visitor calling the staff-gated advance_lead is denied.
func TestRegistryRunsDemoInstancesEndToEnd(t *testing.T) {
	store := &fakeStore{}
	reg := NewRegistry(RegistryConfig{Store: store, Tenants: &fakeTenants{}, Log: testLogger()})

	w := leadWriter()
	RegisterEntityExecutors(reg, EntityExecutorDeps{
		State:         newOpsStatePort(),
		Catalog:       &opsCatalogPort{},
		Entities:      &opsEntityPort{def: leadTestDef(), sets: []domain.ValueSet{leadTestSets()["lead_pipeline"]}},
		Writer:        w,
		Notifications: &fakeNotifStore{},
	})
	if err := reg.SeedTemplates(context.Background(), nil); err != nil {
		t.Fatalf("SeedTemplates: %v", err)
	}

	instances := DemoLeadInstances()
	store.instances = nil
	for _, inst := range instances {
		store.instances = append(store.instances, domain.TenantOperation{
			ID: "inst-" + inst.Instance, TenantID: "tnt-1", OperationID: "id-" + inst.Template,
			Name: inst.Instance, Enabled: true, Config: inst.Config,
		})
	}

	// The schedule executor's clock is registry-internal here — pin it so
	// the "future" input below cannot rot.
	// (RegisterExecutor keeps the concrete executor; re-register a pinned one.)
	sched := NewScheduleSlotExecutor(w)
	sched.now = func() time.Time { return fixedNow }
	reg.RegisterExecutor(domain.KindScheduleSlot, sched)

	// Visible to the storefront visitor: book_showing and lead_search
	// (instances of visitor-gated templates), not advance_lead's write.
	defs := reg.DefinitionsFor(context.Background(), "acme", domain.ModeStorefront, domain.AgentData, domain.RoleVisitor)
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names["book_showing"] || !names["lead_search"] {
		t.Fatalf("instances not visible: %v", names)
	}
	if names["advance_lead"] {
		t.Errorf("staff-gated instance must be invisible to visitors: %v", names)
	}

	// Booking: dollars in, cents at the write path (R18), guard satisfied.
	res, err := reg.Execute(context.Background(),
		domain.OperationContext{TenantSlug: "acme", SessionID: "sess-1", Mode: domain.ModeStorefront, Role: domain.RoleVisitor},
		domain.ToolCall{Name: "book_showing", Input: map[string]any{
			"name":          "Ann",
			"phone":         "+14155550101",
			"budget":        float64(1500), // LLM speaks dollars
			"preferredTime": "2026-08-03T15:00:00Z",
			"listingId":     "lst-1",
		}})
	if err != nil {
		t.Fatalf("Execute(book_showing): %v", err)
	}
	if res.Outcome != domain.OutcomeOK {
		t.Fatalf("book_showing failed: %q (%s)", res.Outcome, res.Summary)
	}
	if w.created.data["budget"] != 150000 {
		t.Errorf("usd not coerced to cents at the choke point: %v", w.created.data["budget"])
	}
	if w.created.tenantID != "tnt-1" {
		t.Errorf("tenant id not resolved into the write: %q", w.created.tenantID)
	}

	// R14: visitor vs the staff-gated transition.
	res, err = reg.Execute(context.Background(),
		domain.OperationContext{TenantSlug: "acme", Mode: domain.ModeCRM, Role: domain.RoleVisitor},
		domain.ToolCall{Name: "advance_lead", Input: map[string]any{"id": "rec-1", "to_status": "contacted"}})
	if err != nil {
		t.Fatalf("Execute(advance_lead): %v", err)
	}
	if res.Outcome != domain.OutcomeDenied {
		t.Fatalf("visitor must be denied the staff op, got %q", res.Outcome)
	}

	// Staff passes.
	res, err = reg.Execute(context.Background(),
		domain.OperationContext{TenantSlug: "acme", Mode: domain.ModeCRM, Role: domain.RoleStaff},
		domain.ToolCall{Name: "advance_lead", Input: map[string]any{"id": "rec-1", "to_status": "contacted"}})
	if err != nil {
		t.Fatalf("Execute(advance_lead as staff): %v", err)
	}
	if res.Outcome != domain.OutcomeOK {
		t.Fatalf("staff transition failed: %q (%s)", res.Outcome, res.Summary)
	}
}

// The demo instance configs must satisfy their templates' config schemas —
// the exact validation EnableOperation runs at seed time.
func TestDemoLeadInstancesConfigsMatchTemplates(t *testing.T) {
	for _, inst := range DemoLeadInstances() {
		tmpl := seedRow(inst.Template)
		if _, violations := ValidateInput(tmpl.ConfigSchema, inst.Config); len(violations) > 0 {
			t.Errorf("%s config invalid against %s schema: %v", inst.Instance, inst.Template, violations)
		}
	}
}

// The demo automation's operation must be automation-runnable AND resolve
// as a wire name: the notify instance is named "notify" because
// v5_automations.operation_slug is pinned to exactly that.
func TestDemoBundleNotifyInstanceAnswersToAutomationSlug(t *testing.T) {
	found := false
	for _, inst := range DemoLeadInstances() {
		if inst.Instance == "notify" && inst.Template == "notify" {
			found = true
		}
	}
	if !found {
		t.Fatal("demo bundle must enable the notify template under the instance name 'notify' (automation dispatch resolves that exact wire name)")
	}
	if demoBookingAutomation("tnt-1").OperationSlug != "notify" {
		t.Fatal("demo automation must dispatch 'notify'")
	}
}
