package operations

import (
	"context"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/usecases"
)

// Compile-time proof of the cross-package contract: the real write path
// (usecases.EntityWrite) satisfies the consumer-side EntityWriter the
// executors are built on. (operations cannot import usecases in non-test
// code — the usecases in-package tests import operations.)
var _ EntityWriter = (*usecases.EntityWrite)(nil)

// ─── shared fakes for the executor tests ─────────────────────────────────

// fakeWriter is an in-memory EntityWriter capturing write calls.
type fakeWriter struct {
	def  *domain.EntityDefinition
	sets map[string]domain.ValueSet

	created struct {
		tenantID, tenantSlug, entity, createdBy string
		data                                    map[string]any
		ref                                     *domain.EntityRef
		calls                                   int
	}
	updated struct {
		recordID string
		patch    map[string]any
	}
	transitioned struct {
		recordID, toStatus string
		allowed            map[string][]string
	}
	err error // returned by every mutation when set
}

func (w *fakeWriter) LoadDefinition(_ context.Context, _, _ string) (*domain.EntityDefinition, map[string]domain.ValueSet, error) {
	if w.def == nil {
		return nil, nil, &domain.Error{Code: "ENTITY_NOT_FOUND", Message: "entity definition not found"}
	}
	return w.def, w.sets, nil
}

func (w *fakeWriter) CreateRecord(_ context.Context, tenantID, tenantSlug, entity string, data map[string]any, ref *domain.EntityRef, createdBy string) (*domain.EntityRecord, error) {
	w.created.calls++
	w.created.tenantID, w.created.tenantSlug = tenantID, tenantSlug
	w.created.entity, w.created.createdBy = entity, createdBy
	w.created.data, w.created.ref = data, ref
	if w.err != nil {
		return nil, w.err
	}
	status, _ := data["status"].(string)
	return &domain.EntityRecord{ID: "rec-1", TenantID: tenantID, EntitySlug: entity, Data: data, Status: status}, nil
}

func (w *fakeWriter) UpdateRecord(_ context.Context, tenantID, _, recordID string, patch map[string]any) (*domain.EntityRecord, error) {
	w.updated.recordID, w.updated.patch = recordID, patch
	if w.err != nil {
		return nil, w.err
	}
	return &domain.EntityRecord{ID: recordID, TenantID: tenantID, EntitySlug: "lead", Data: patch}, nil
}

func (w *fakeWriter) TransitionStatus(_ context.Context, tenantID, _, recordID, toStatus string, allowed map[string][]string) (*domain.EntityRecord, error) {
	w.transitioned.recordID, w.transitioned.toStatus = recordID, toStatus
	w.transitioned.allowed = allowed
	if w.err != nil {
		return nil, w.err
	}
	return &domain.EntityRecord{ID: recordID, TenantID: tenantID, EntitySlug: "lead", Status: toStatus}, nil
}

// leadTestDef is the realtor lead shape the executor tests run on.
func leadTestDef() *domain.EntityDefinition {
	return &domain.EntityDefinition{
		ID: "def-1", TenantID: "tnt-1", Slug: "lead", Name: "Lead", NamePlural: "Leads",
		Fields: []domain.FieldDef{
			{Key: "name", Label: "Name", Type: domain.FieldText, Required: true},
			{Key: "phone", Label: "Phone", Type: domain.FieldPhone, Required: true},
			{Key: "budget", Label: "Budget", Type: domain.FieldMoney},
			{Key: "preferredTime", Label: "Preferred time", Type: domain.FieldDatetime},
			{Key: "listingId", Label: "Listing", Type: domain.FieldRef, RefTarget: "product"},
			{Key: "status", Label: "Status", Type: domain.FieldEnum, ValueSetRef: "lead_pipeline", Default: "new"},
		},
		StatusField: "status",
	}
}

func leadTestSets() map[string]domain.ValueSet {
	return map[string]domain.ValueSet{
		"lead_pipeline": {
			ID: "vs-1", TenantID: "tnt-1", Slug: "lead_pipeline", Name: "Lead pipeline",
			Values: []domain.ValueSetEntry{
				{Value: "new", Label: "New"},
				{Value: "contacted", Label: "Contacted"},
				{Value: "showing_booked", Label: "Showing booked"},
				{Value: "closed", Label: "Closed"},
			},
		},
	}
}

func leadWriter() *fakeWriter {
	return &fakeWriter{def: leadTestDef(), sets: leadTestSets()}
}

// opsEntityPort is the minimal ports.EntityPort the query executor reads.
type opsEntityPort struct {
	def     *domain.EntityDefinition
	sets    []domain.ValueSet
	records []domain.EntityRecord
	total   int
	filter  domain.RecordFilter // captured
}

var _ ports.EntityPort = (*opsEntityPort)(nil)

func (p *opsEntityPort) UpsertEntityDefinition(context.Context, *domain.EntityDefinition) error {
	return nil
}
func (p *opsEntityPort) GetEntityDefinition(_ context.Context, _, slug string) (*domain.EntityDefinition, error) {
	if p.def == nil || p.def.Slug != slug {
		return nil, &domain.Error{Code: "ENTITY_NOT_FOUND", Message: "entity definition not found"}
	}
	return p.def, nil
}
func (p *opsEntityPort) ListEntityDefinitions(context.Context, string) ([]domain.EntityDefinition, error) {
	return nil, nil
}
func (p *opsEntityPort) UpsertValueSet(context.Context, *domain.ValueSet) error { return nil }
func (p *opsEntityPort) GetValueSet(context.Context, string, string) (*domain.ValueSet, error) {
	return nil, nil
}
func (p *opsEntityPort) ListValueSets(context.Context, string) ([]domain.ValueSet, error) {
	return p.sets, nil
}
func (p *opsEntityPort) CreateRecord(context.Context, *domain.EntityRecord) (*domain.EntityRecord, *domain.RuntimeEvent, error) {
	return nil, nil, nil
}
func (p *opsEntityPort) UpdateRecord(context.Context, string, string, map[string]any) (*domain.EntityRecord, *domain.RuntimeEvent, error) {
	return nil, nil, nil
}
func (p *opsEntityPort) TransitionStatus(context.Context, string, string, string) (*domain.EntityRecord, *domain.RuntimeEvent, error) {
	return nil, nil, nil
}
func (p *opsEntityPort) GetRecord(context.Context, string, string) (*domain.EntityRecord, error) {
	return nil, nil
}
func (p *opsEntityPort) QueryRecords(_ context.Context, _, _ string, f domain.RecordFilter) ([]domain.EntityRecord, int, error) {
	p.filter = f
	return p.records, p.total, nil
}
func (p *opsEntityPort) UpsertAutomation(context.Context, *domain.Automation) error { return nil }
func (p *opsEntityPort) ListAutomations(context.Context, string, string) ([]domain.Automation, error) {
	return nil, nil
}
func (p *opsEntityPort) MarkEventProcessed(context.Context, int64) error { return nil }

// opsStatePort is the minimal ports.StatePort the query executor writes.
type opsStatePort struct {
	state *domain.SessionState

	updatedData *domain.StateData
	updatedMeta *domain.StateMeta
	updatedInfo *domain.DeltaInfo
	deltas      []domain.Delta
}

var _ ports.StatePort = (*opsStatePort)(nil)

func newOpsStatePort() *opsStatePort {
	return &opsStatePort{state: &domain.SessionState{
		SessionID: "sess-1",
		Current: domain.StateCurrent{
			Meta: domain.StateMeta{Aliases: map[string]string{"tenant_slug": "acme"}},
		},
	}}
}

func (s *opsStatePort) CreateState(context.Context, string) (*domain.SessionState, error) {
	return s.state, nil
}
func (s *opsStatePort) GetState(context.Context, string) (*domain.SessionState, error) {
	cp := *s.state
	return &cp, nil
}
func (s *opsStatePort) UpdateState(context.Context, *domain.SessionState) error { return nil }
func (s *opsStatePort) AddDelta(_ context.Context, _ string, d *domain.Delta) (int, error) {
	s.deltas = append(s.deltas, *d)
	return len(s.deltas), nil
}
func (s *opsStatePort) GetDeltas(context.Context, string) ([]domain.Delta, error) { return nil, nil }
func (s *opsStatePort) GetDeltasSince(context.Context, string, int) ([]domain.Delta, error) {
	return nil, nil
}
func (s *opsStatePort) GetDeltasUntil(context.Context, string, int) ([]domain.Delta, error) {
	return nil, nil
}
func (s *opsStatePort) UpdateData(_ context.Context, _ string, data domain.StateData, meta domain.StateMeta, info domain.DeltaInfo) (int, error) {
	s.updatedData, s.updatedMeta, s.updatedInfo = &data, &meta, &info
	s.state.Current.Data = data
	s.state.Current.Meta = meta
	return 1, nil
}
func (s *opsStatePort) UpdateTemplate(context.Context, string, map[string]interface{}, domain.DeltaInfo) (int, error) {
	return 0, nil
}
func (s *opsStatePort) UpdateView(context.Context, string, domain.ViewState, []domain.ViewSnapshot, domain.DeltaInfo) (int, error) {
	return 0, nil
}
func (s *opsStatePort) UpdateActions(context.Context, string, domain.StateActions, domain.DeltaInfo) (int, error) {
	return 0, nil
}
func (s *opsStatePort) AppendConversation(context.Context, string, []domain.LLMMessage) error {
	return nil
}
func (s *opsStatePort) AppendAgent2History(context.Context, string, []domain.LLMMessage) error {
	return nil
}
func (s *opsStatePort) PushView(context.Context, string, *domain.ViewSnapshot) error { return nil }
func (s *opsStatePort) PopView(context.Context, string) (*domain.ViewSnapshot, error) {
	return nil, nil
}
func (s *opsStatePort) GetViewStack(context.Context, string) ([]domain.ViewSnapshot, error) {
	return nil, nil
}

// opsCatalogPort backs the catalog branch of the query executor.
type opsCatalogPort struct {
	tenant   *domain.Tenant
	products []domain.Product
	digest   *domain.CatalogDigest
}

var _ ports.CatalogPort = (*opsCatalogPort)(nil)

func (c *opsCatalogPort) GetTenantBySlug(_ context.Context, slug string) (*domain.Tenant, error) {
	if c.tenant != nil {
		return c.tenant, nil
	}
	return &domain.Tenant{ID: "tnt-1", Slug: slug}, nil
}
func (c *opsCatalogPort) ListProducts(context.Context, string, ports.ProductFilter) ([]domain.Product, int, error) {
	return c.products, len(c.products), nil
}
func (c *opsCatalogPort) GetProduct(context.Context, string, string) (*domain.Product, error) {
	return nil, domain.ErrProductNotFound
}
func (c *opsCatalogPort) VectorSearch(context.Context, string, []float32, int, *ports.VectorFilter) ([]domain.Product, error) {
	return c.products, nil
}
func (c *opsCatalogPort) SearchProjection(context.Context, string, []float32, ports.ProductFilter, int) ([]domain.Product, error) {
	return c.products, nil
}
func (c *opsCatalogPort) BuildCatalogDigest(context.Context, string) (*domain.CatalogDigest, error) {
	return c.digest, nil
}

// fakeNotifStore captures notifications and assigns ids.
type fakeNotifStore struct {
	stored []*domain.Notification
	err    error
}

func (s *fakeNotifStore) CreateNotification(_ context.Context, n *domain.Notification) error {
	if s.err != nil {
		return s.err
	}
	n.ID = "ntf-1"
	if n.Audience == "" {
		n.Audience = "crm"
	}
	s.stored = append(s.stored, n)
	return nil
}
