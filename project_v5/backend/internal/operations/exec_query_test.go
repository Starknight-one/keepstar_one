package operations

import (
	"context"
	"testing"
	"time"

	"keepstar_v5/internal/domain"
)

func leadQueryConfig() map[string]any {
	return map[string]any{"source": "entity", "entity": "lead"}
}

func newEntityQueryExecutor(records []domain.EntityRecord) (*QueryExecutor, *opsStatePort, *opsEntityPort) {
	state := newOpsStatePort()
	entities := &opsEntityPort{
		def:     leadTestDef(),
		sets:    []domain.ValueSet{leadTestSets()["lead_pipeline"]},
		records: records,
		total:   len(records),
	}
	ex := NewQueryExecutor(state, &opsCatalogPort{}, entities, nil)
	return ex, state, entities
}

// The entity branch: records land in the Entities zone (preserving the
// catalog zones), the delta is ENTITY_QUERY on Path "data" (R28), and the
// result carries the microcontext contract (EntityKind + Count →
// "lead_search: N leads found").
func TestQueryExecutorEntityBranchWritesEntitiesZone(t *testing.T) {
	records := []domain.EntityRecord{
		{ID: "rec-1", EntitySlug: "lead", Data: map[string]any{"name": "Ann", "status": "new"}, Status: "new"},
		{ID: "rec-2", EntitySlug: "lead", Data: map[string]any{"name": "Bob", "status": "new"}, Status: "new"},
	}
	ex, state, entities := newEntityQueryExecutor(records)
	state.state.Current.Data.Products = []domain.Product{{ID: "p1", Name: "Sea View 2BR"}}

	res, err := ex.Execute(context.Background(), domain.OperationContext{
		TenantID: "tnt-1", TenantSlug: "acme", SessionID: "sess-1", TurnID: "turn-1",
		Config: leadQueryConfig(),
	}, map[string]any{
		"status": "new",
		"limit":  5, // validator emits Go int for integer props
		"filters": map[string]any{
			"min_budget": 150000, // usd-coerced cents (int)
			"name":       "Ann",
		},
		"since": "2026-07-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeOK || res.Count != 2 || res.EntityKind != "lead" {
		t.Fatalf("result = %+v", res)
	}
	if res.Summary != "ok: found 2 leads" {
		t.Errorf("summary = %q", res.Summary)
	}

	// Filter mapping.
	f := entities.filter
	if f.Status != "new" || f.Limit != 5 {
		t.Errorf("filter = %+v", f)
	}
	if f.AttrMin["budget"] != 150000 {
		t.Errorf("money range not mapped to cents: %v", f.AttrMin)
	}
	if f.Attrs["name"] != "Ann" {
		t.Errorf("exact match not mapped: %v", f.Attrs)
	}
	if f.Since == nil || !f.Since.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("since not parsed: %v", f.Since)
	}

	// Zone write: Entities set + preserved products.
	if state.updatedData == nil {
		t.Fatal("UpdateData not called")
	}
	if len(state.updatedData.Products) != 1 {
		t.Errorf("catalog zone must be preserved, got %d products", len(state.updatedData.Products))
	}
	if len(state.updatedData.Entities) != 1 {
		t.Fatalf("entities zone = %#v", state.updatedData.Entities)
	}
	set := state.updatedData.Entities[0]
	if set.Slug != "lead" || len(set.Records) != 2 || len(set.Fields) == 0 {
		t.Errorf("entity set = %+v", set)
	}
	if set.Labels["lead_pipeline"]["new"] != "New" {
		t.Errorf("value-set labels missing: %#v", set.Labels)
	}
	if state.updatedMeta.EntityCounts["lead"] != 2 {
		t.Errorf("entity counts = %#v", state.updatedMeta.EntityCounts)
	}

	// R28 delta vocabulary.
	info := state.updatedInfo
	if info.Action.Type != domain.ActionEntityQuery || info.Path != "data" {
		t.Errorf("delta = %+v", info)
	}
	if info.TurnID != "turn-1" {
		t.Errorf("turn id not threaded: %q", info.TurnID)
	}
}

// Zero hits are the truth ("any new leads?" → none): the empty set still
// writes, the outcome is empty.
func TestQueryExecutorEntityBranchEmpty(t *testing.T) {
	ex, state, _ := newEntityQueryExecutor(nil)

	res, err := ex.Execute(context.Background(), domain.OperationContext{
		TenantID: "tnt-1", TenantSlug: "acme", SessionID: "sess-1",
		Config: leadQueryConfig(),
	}, map[string]any{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeEmpty {
		t.Fatalf("outcome = %q", res.Outcome)
	}
	if res.Summary != "empty: no leads found" {
		t.Errorf("summary = %q", res.Summary)
	}
	if state.updatedData == nil || len(state.updatedData.Entities) != 1 || len(state.updatedData.Entities[0].Records) != 0 {
		t.Errorf("empty set must still write the zone: %#v", state.updatedData)
	}
	if state.updatedMeta.EntityCounts["lead"] != 0 {
		t.Errorf("entity counts = %#v", state.updatedMeta.EntityCounts)
	}
}

// The catalog branch reuses the proven catalog search flow: same Products
// zone write, same Summary bytes, EntityKind deliberately empty (the
// legacy "new_search" microcontext + stampStateCounts contract).
func TestQueryExecutorCatalogBranch(t *testing.T) {
	state := newOpsStatePort()
	catalog := &opsCatalogPort{
		tenant:   &domain.Tenant{ID: "tnt-1", Slug: "acme"},
		products: []domain.Product{{ID: "p1", Name: "Sea View 2BR", Price: 250000000}},
	}
	ex := NewQueryExecutor(state, catalog, &opsEntityPort{}, nil)

	res, err := ex.Execute(context.Background(), domain.OperationContext{
		TenantID: "tnt-1", TenantSlug: "acme", SessionID: "sess-1",
		Config: map[string]any{"source": "catalog"},
	}, map[string]any{
		"vector_query": "2 bedroom apartments",
		"limit":        10, // int from the validator — must not break the legacy float64 decode
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeOK {
		t.Fatalf("outcome = %q (%s)", res.Outcome, res.Summary)
	}
	if res.Summary != "ok: found 1 products" {
		t.Errorf("summary = %q (byte contract with the legacy tool)", res.Summary)
	}
	if res.EntityKind != "" {
		t.Errorf("catalog results must keep EntityKind empty, got %q", res.EntityKind)
	}
	if state.updatedData == nil || len(state.updatedData.Products) != 1 {
		t.Errorf("products zone not written: %#v", state.updatedData)
	}
}

// SpecForTenant: catalog config derives the digest schema; entity config
// derives the definition schema.
func TestQueryExecutorSpecForTenant(t *testing.T) {
	state := newOpsStatePort()
	catalog := &opsCatalogPort{digest: &domain.CatalogDigest{TotalProducts: 5, PriceMin: 100, PriceMax: 900}}
	entities := &opsEntityPort{def: leadTestDef(), sets: []domain.ValueSet{leadTestSets()["lead_pipeline"]}}
	ex := NewQueryExecutor(state, catalog, entities, nil)
	tenant := domain.Tenant{ID: "tnt-1", Slug: "acme"}

	catSpec, err := ex.SpecForTenant(context.Background(), tenant, map[string]any{"source": "catalog"})
	if err != nil {
		t.Fatalf("SpecForTenant(catalog): %v", err)
	}
	props, _ := catSpec.InputSchema["properties"].(map[string]any)
	if _, ok := props["vector_query"]; !ok {
		t.Errorf("catalog schema must be CatalogSearchSchemaForDigest, got %v", props)
	}

	entSpec, err := ex.SpecForTenant(context.Background(), tenant, leadQueryConfig())
	if err != nil {
		t.Fatalf("SpecForTenant(entity): %v", err)
	}
	eProps, _ := entSpec.InputSchema["properties"].(map[string]any)
	status, _ := eProps["status"].(map[string]any)
	if status == nil {
		t.Fatalf("entity schema missing status: %v", eProps)
	}
	enum, _ := status["enum"].([]string)
	if len(enum) != 4 || enum[0] != "new" {
		t.Errorf("status enum from value set = %v", enum)
	}
	if _, ok := eProps["ref_id"]; !ok {
		t.Errorf("entity schema missing ref_id (definition links the catalog)")
	}
	filters, _ := eProps["filters"].(map[string]any)
	fProps, _ := filters["properties"].(map[string]any)
	minBudget, _ := fProps["min_budget"].(map[string]any)
	if minBudget == nil || minBudget[domain.SchemaKeyUnit] != string(domain.UnitUSD) {
		t.Errorf("money range must be usd-annotated for cents coercion: %v", fProps)
	}
}
