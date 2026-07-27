package operations

import (
	"context"
	"fmt"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// Demo bundle — the hand-assembled realtor tenant of M2 step 7 (e2e steps
// 7-9 without onboarding): the lead entity, its pipeline value set, the
// booking automation and the four operation instances of §4.3. In the full
// flow this exact content is LIBRARY content staged by the onboarding
// agent (enable_operations / define_entity / …) — this seeder is the
// scripted equivalent for demo assembly and integration tests, idempotent
// end to end (upserts on natural keys, EnableOperation re-runs safely).

// InstanceSeed is one v5_tenant_operations row to enable: a library
// template under an instance wire name with its config.
type InstanceSeed struct {
	Template string
	Instance string
	Config   map[string]any
}

// DemoLeadInstances returns the §4.3 demo tenant instances.
//
// The notify instance is deliberately named "notify" (not the spec's
// illustrative "notify_agent"): v5_automations.operation_slug is pinned to
// exactly "notify" (validateAutomation, §4.2 allowlist) and the registry
// resolves wire names instance-first with no kind-based fallback — so the
// instance carrying the channel config must answer to the name automations
// dispatch.
func DemoLeadInstances() []InstanceSeed {
	return []InstanceSeed{
		{Template: "query", Instance: "lead_search", Config: map[string]any{
			"source": "entity", "entity": "lead",
		}},
		{Template: "schedule_slot", Instance: "book_showing", Config: map[string]any{
			"entity":         "lead",
			"datetime_field": "preferredTime",
			"link_field":     "listingId",
			"reject_past":    true,
			// float64 — the store's config validation speaks JSON numbers.
			"hours":    map[string]any{"from": float64(9), "to": float64(18)},
			"defaults": map[string]any{"status": "new"},
		}},
		{Template: "transition_status", Instance: "advance_lead", Config: map[string]any{
			"entity": "lead", "field": "status", "value_set": "lead_pipeline",
		}},
		{Template: "notify", Instance: "notify", Config: map[string]any{
			"channel": "runtime",
		}},
	}
}

// demoLeadDefinition is the realtor lead shape (§1 beat 5): contact
// fields, the preferred showing time, the catalog link to the listing and
// the pipeline status.
func demoLeadDefinition(tenantID string) *domain.EntityDefinition {
	return &domain.EntityDefinition{
		TenantID:   tenantID,
		Slug:       "lead",
		Name:       "Lead",
		NamePlural: "Leads",
		Fields: []domain.FieldDef{
			{Key: "name", Label: "Name", Type: domain.FieldText, Required: true},
			{Key: "phone", Label: "Phone", Type: domain.FieldPhone, Required: true},
			{Key: "email", Label: "Email", Type: domain.FieldEmail},
			{Key: "message", Label: "Message", Type: domain.FieldText},
			{Key: "preferredTime", Label: "Preferred time", Type: domain.FieldDatetime},
			{Key: "listingId", Label: "Listing", Type: domain.FieldRef, RefTarget: string(domain.EntityTypeProduct)},
			{Key: "status", Label: "Status", Type: domain.FieldEnum, ValueSetRef: "lead_pipeline", Default: "new"},
		},
		Display: domain.EntityDisplay{
			TitleField:    "name",
			SubtitleField: "phone",
			BadgeField:    "status",
			DefaultSort:   "createdAt",
		},
		StatusField: "status",
	}
}

// demoLeadPipeline is the §4.3 value set: new, contacted, showing_booked,
// closed. Colors on the brand ramp — no purple.
func demoLeadPipeline(tenantID string) *domain.ValueSet {
	return &domain.ValueSet{
		TenantID: tenantID,
		Slug:     "lead_pipeline",
		Name:     "Lead pipeline",
		Values: []domain.ValueSetEntry{
			{Value: "new", Label: "New", Color: "#5BA4D9"},
			{Value: "contacted", Label: "Contacted", Color: "#F0924A"},
			{Value: "showing_booked", Label: "Showing booked", Color: "#4CAF7D"},
			{Value: "closed", Label: "Closed", Color: "#8A8F98"},
		},
	}
}

// demoBookingAutomation notifies staff the moment a lead record commits.
// {field} placeholders substitute from the event snapshot (+ recordId)
// in OperationRunner.
func demoBookingAutomation(tenantID string) *domain.Automation {
	return &domain.Automation{
		TenantID:      tenantID,
		Name:          "notify-on-new-lead",
		EventType:     domain.EventRecordCreated,
		EntitySlug:    "lead",
		OperationSlug: "notify",
		OperationParams: map[string]any{
			"title": "Showing request — {refTitle}",
			"body":  "{name} · {phone} · preferred time {preferredTime}",
			"ref":   "{recordId}",
		},
		Enabled: true,
	}
}

// SeedDemoLeadBundle assembles the demo bundle on one tenant: entity +
// value set + automation via the entity plane, the four instances via the
// operation store. Callers invalidate the registry's tenant entry after
// (OperationStorePort mutator contract, §6.1).
func SeedDemoLeadBundle(ctx context.Context, entities ports.EntityPort, store ports.OperationStorePort, tenantID string) error {
	if err := entities.UpsertValueSet(ctx, demoLeadPipeline(tenantID)); err != nil {
		return fmt.Errorf("demo bundle: value set: %w", err)
	}
	if err := entities.UpsertEntityDefinition(ctx, demoLeadDefinition(tenantID)); err != nil {
		return fmt.Errorf("demo bundle: lead definition: %w", err)
	}
	if err := entities.UpsertAutomation(ctx, demoBookingAutomation(tenantID)); err != nil {
		return fmt.Errorf("demo bundle: automation: %w", err)
	}
	for _, inst := range DemoLeadInstances() {
		if _, err := store.EnableOperation(ctx, tenantID, inst.Template, inst.Instance, inst.Config); err != nil {
			return fmt.Errorf("demo bundle: enable %s: %w", inst.Instance, err)
		}
	}
	return nil
}
