package usecases

import (
	"context"
	"testing"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// --- minimal fakes -------------------------------------------------------

// fakeCatalogPort answers GetProduct from an in-memory map. Other methods
// panic — they aren't part of the apply contract.
type fakeCatalogPort struct {
	listings map[string]domain.Product // listingID → product
}

func (f *fakeCatalogPort) GetProduct(_ context.Context, _ string, productID string) (*domain.Product, error) {
	if p, ok := f.listings[productID]; ok {
		return &p, nil
	}
	return nil, nil
}

func (f *fakeCatalogPort) GetTenantByID(context.Context, string) (*domain.Tenant, error) { return nil, nil }
func (f *fakeCatalogPort) CreateTenant(context.Context, *domain.Tenant) (*domain.Tenant, error) {
	panic("not used")
}
func (f *fakeCatalogPort) UpdateTenantSettings(context.Context, string, domain.TenantSettings) error {
	panic("not used")
}
func (f *fakeCatalogPort) ListProducts(context.Context, string, domain.AdminProductFilter) ([]domain.Product, int, error) {
	panic("not used")
}
func (f *fakeCatalogPort) UpdateProduct(context.Context, string, string, domain.ProductUpdate) error {
	panic("not used")
}
func (f *fakeCatalogPort) ListServices(context.Context, string, domain.AdminProductFilter) ([]domain.Service, int, error) {
	panic("not used")
}
func (f *fakeCatalogPort) GetService(context.Context, string, string) (*domain.Service, error) {
	panic("not used")
}
func (f *fakeCatalogPort) UpdateService(context.Context, string, string, domain.ProductUpdate) error {
	panic("not used")
}
func (f *fakeCatalogPort) GetCategories(context.Context, string) ([]domain.Category, error) {
	panic("not used")
}
func (f *fakeCatalogPort) UpsertMasterProduct(context.Context, *domain.MasterProduct) (string, error) {
	panic("not used")
}
func (f *fakeCatalogPort) UpsertProductListing(context.Context, *domain.Product) (string, error) {
	panic("not used")
}
func (f *fakeCatalogPort) UpsertMasterService(context.Context, *domain.MasterService) (string, error) {
	panic("not used")
}
func (f *fakeCatalogPort) UpsertServiceListing(context.Context, *domain.Service) (string, error) {
	panic("not used")
}
func (f *fakeCatalogPort) GetOrCreateCategory(context.Context, string, string) (string, error) {
	panic("not used")
}
func (f *fakeCatalogPort) BulkUpdateStock(context.Context, string, []domain.StockUpdate) (int, error) {
	panic("not used")
}
func (f *fakeCatalogPort) GetCategoryBySlug(context.Context, string) (*domain.Category, error) {
	panic("not used")
}
func (f *fakeCatalogPort) GetAllMasterProducts(context.Context, string) ([]domain.MasterProduct, error) {
	panic("not used")
}
func (f *fakeCatalogPort) GetUnenrichedMasterProducts(context.Context, string) ([]domain.MasterProduct, error) {
	panic("not used")
}
func (f *fakeCatalogPort) UpdateMasterProductPIM(context.Context, string, string, domain.EnrichmentOutputV2) error {
	panic("not used")
}
func (f *fakeCatalogPort) SoftDeleteProductBySource(context.Context, string, string, string) error {
	panic("not used")
}
func (f *fakeCatalogPort) UpsertListingFromSource(context.Context, *domain.ListingFromSource) (string, error) {
	panic("not used")
}
func (f *fakeCatalogPort) GetMasterProductsWithoutEmbedding(context.Context, string) ([]domain.MasterProduct, error) {
	panic("not used")
}
func (f *fakeCatalogPort) GetMasterServicesWithoutEmbedding(context.Context, string) ([]domain.MasterService, error) {
	panic("not used")
}
func (f *fakeCatalogPort) SeedEmbedding(context.Context, string, []float32) error { panic("not used") }
func (f *fakeCatalogPort) SeedServiceEmbedding(context.Context, string, []float32) error {
	panic("not used")
}
func (f *fakeCatalogPort) GenerateCatalogDigest(context.Context, string) error { panic("not used") }

// fakeReportsPort holds one report by id and lets tests inspect updates.
type fakeReportsPort struct {
	byID         map[int64]*domain.MergeReport
	updateCalls  int
	markCalls    int
	lastFinal    domain.MergeReportStatus
}

func (f *fakeReportsPort) Save(context.Context, *domain.MergeReport) (int64, error) {
	panic("not used")
}
func (f *fakeReportsPort) GetByID(_ context.Context, id int64) (*domain.MergeReport, error) {
	if r, ok := f.byID[id]; ok {
		return r, nil
	}
	return nil, nil
}
func (f *fakeReportsPort) GetLatestForTenant(context.Context, string) (*domain.MergeReport, error) {
	panic("not used")
}
func (f *fakeReportsPort) ListForTenant(context.Context, string, int) ([]domain.MergeReport, error) {
	panic("not used")
}
func (f *fakeReportsPort) UpdateStatus(_ context.Context, id int64, s domain.MergeReportStatus) error {
	if r, ok := f.byID[id]; ok {
		r.Status = s
	}
	return nil
}
func (f *fakeReportsPort) UpdateProposals(_ context.Context, id int64, p []domain.MergeProposal, _ ports.MergeReportCounters) error {
	f.updateCalls++
	if r, ok := f.byID[id]; ok {
		r.Proposals = p
	}
	return nil
}
func (f *fakeReportsPort) MarkApplied(_ context.Context, id int64, by string, final domain.MergeReportStatus) error {
	f.markCalls++
	f.lastFinal = final
	if r, ok := f.byID[id]; ok {
		r.Status = final
		r.AppliedBy = by
		now := time.Now().UTC()
		r.AppliedAt = &now
	}
	return nil
}

// fakeApplyTxPort records every call so tests can assert what the applier
// asked the writer to do.
type fakeApplyTxPort struct {
	newMasterCalls          []newMasterCall
	linkExistingCalls       []linkExistingCall
	variantOfExistingCalls  []variantOfExistingCall
	restoreCalls            []restoreCall

	// Pre-seeded responses.
	newMasterReturnMP string
	newMasterReturnMV string
	linkExistingErr   error
}

type newMasterCall struct {
	listingID string
	pm        *domain.ProposedMaster
}
type linkExistingCall struct {
	listingID, mpID, mvID string
	propagate             []domain.FieldDecision
}
type variantOfExistingCall struct {
	listingID, parentMP string
	pv                  *domain.ProposedVariant
}
type restoreCall struct {
	listingID, prevMP, prevMV string
}

func (f *fakeApplyTxPort) ApplyNewMaster(_ context.Context, listingID string, pm *domain.ProposedMaster) (string, string, error) {
	f.newMasterCalls = append(f.newMasterCalls, newMasterCall{listingID, pm})
	return f.newMasterReturnMP, f.newMasterReturnMV, nil
}
func (f *fakeApplyTxPort) ApplyLinkExisting(_ context.Context, listingID, mpID, mvID string, p []domain.FieldDecision) error {
	f.linkExistingCalls = append(f.linkExistingCalls, linkExistingCall{listingID, mpID, mvID, p})
	return f.linkExistingErr
}
func (f *fakeApplyTxPort) ApplyVariantOfExisting(_ context.Context, listingID, parentMP string, pv *domain.ProposedVariant) (string, error) {
	f.variantOfExistingCalls = append(f.variantOfExistingCalls, variantOfExistingCall{listingID, parentMP, pv})
	return "mv-new", nil
}
func (f *fakeApplyTxPort) RestoreListingLink(_ context.Context, listingID, prevMP, prevMV string) error {
	f.restoreCalls = append(f.restoreCalls, restoreCall{listingID, prevMP, prevMV})
	return nil
}

// --- helpers --------------------------------------------------------------

func mustLogger(t *testing.T) *logger.Logger {
	t.Helper()
	return logger.New("error")
}

func newTestUseCase(t *testing.T, catalog *fakeCatalogPort, reports *fakeReportsPort, tx *fakeApplyTxPort) *MergeApplyUseCase {
	t.Helper()
	uc := NewMergeApplyUseCase(catalog, nil /* schema unused for apply */, reports, nil /* cascade unused */, mustLogger(t))
	uc.WithApplyTx(tx)
	// audit intentionally nil — the use case is nil-safe and silent.
	return uc
}

func seedReport(reports *fakeReportsPort, id int64, tenantID string, proposals []domain.MergeProposal) {
	reports.byID[id] = &domain.MergeReport{
		ID: id, TenantID: tenantID,
		Status:    domain.MergeReportStatusPending,
		Proposals: proposals,
	}
}

// --- tests ---------------------------------------------------------------

func TestApply_NewMaster_CreatesAndLinks(t *testing.T) {
	ctx := context.Background()
	catalog := &fakeCatalogPort{listings: map[string]domain.Product{
		"l-1": {ID: "l-1", TenantID: "t-1"},
	}}
	reports := &fakeReportsPort{byID: map[int64]*domain.MergeReport{}}
	tx := &fakeApplyTxPort{newMasterReturnMP: "mp-new", newMasterReturnMV: "mv-new"}

	seedReport(reports, 42, "t-1", []domain.MergeProposal{{
		ID: "p-1", ListingID: "l-1", Status: domain.MergeProposalStatusPending,
		Action: domain.MergeActionNewMaster,
		ProposedMaster: &domain.ProposedMaster{Name: "Foo Cream", Brand: "Foo", Vertical: "cosmetics"},
	}})

	uc := newTestUseCase(t, catalog, reports, tx)
	res, err := uc.ApplyProposals(ctx, ApplyProposalsRequest{
		ReportID: 42, ProposalIDs: []string{"p-1"}, ActorID: "curator-1",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.AppliedCount != 1 || res.FailedCount != 0 {
		t.Fatalf("counts wrong: %+v", res)
	}
	if len(tx.newMasterCalls) != 1 {
		t.Fatalf("expected 1 ApplyNewMaster call, got %d", len(tx.newMasterCalls))
	}
	got := reports.byID[42].Proposals[0]
	if got.Status != domain.MergeProposalStatusApplied {
		t.Fatalf("proposal status = %s, want applied", got.Status)
	}
	if got.TargetMasterProductID != "mp-new" || got.TargetMasterVariantID != "mv-new" {
		t.Fatalf("target ids not stamped: mp=%q mv=%q", got.TargetMasterProductID, got.TargetMasterVariantID)
	}
	if got.RollbackData["created_master_product_id"] != "mp-new" {
		t.Fatalf("rollback data missing created mp id: %+v", got.RollbackData)
	}
	if reports.lastFinal != domain.MergeReportStatusApplied {
		t.Fatalf("final status = %s, want applied", reports.lastFinal)
	}
}

func TestApply_LinkExisting_UpdatesListingOnly(t *testing.T) {
	ctx := context.Background()
	catalog := &fakeCatalogPort{listings: map[string]domain.Product{
		"l-2": {ID: "l-2", TenantID: "t-1"},
	}}
	reports := &fakeReportsPort{byID: map[int64]*domain.MergeReport{}}
	tx := &fakeApplyTxPort{}

	seedReport(reports, 7, "t-1", []domain.MergeProposal{{
		ID: "p-2", ListingID: "l-2", Status: domain.MergeProposalStatusPending,
		Action:                domain.MergeActionLinkExisting,
		TargetMasterProductID: "mp-existing",
		TargetMasterVariantID: "mv-existing",
	}})

	uc := newTestUseCase(t, catalog, reports, tx)
	if _, err := uc.ApplyProposals(ctx, ApplyProposalsRequest{
		ReportID: 7, ProposalIDs: []string{"p-2"}, ActorID: "c",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(tx.linkExistingCalls) != 1 {
		t.Fatalf("expected 1 ApplyLinkExisting call, got %d", len(tx.linkExistingCalls))
	}
	if tx.linkExistingCalls[0].mpID != "mp-existing" {
		t.Fatalf("wrong target mp: %s", tx.linkExistingCalls[0].mpID)
	}
	if len(tx.newMasterCalls) != 0 {
		t.Fatalf("should not have created a new master")
	}
}

func TestApply_LinkExisting_WithEdits_PropagatesFieldDecisions(t *testing.T) {
	ctx := context.Background()
	catalog := &fakeCatalogPort{listings: map[string]domain.Product{
		"l-3": {ID: "l-3", TenantID: "t-1"},
	}}
	reports := &fakeReportsPort{byID: map[int64]*domain.MergeReport{}}
	tx := &fakeApplyTxPort{}

	seedReport(reports, 9, "t-1", []domain.MergeProposal{{
		ID: "p-3", ListingID: "l-3", Status: domain.MergeProposalStatusPending,
		Action:                domain.MergeActionLinkExisting,
		TargetMasterProductID: "mp-old",
	}})

	uc := newTestUseCase(t, catalog, reports, tx)
	editedDecisions := []domain.FieldDecision{
		{Field: "tier2.scent", Action: "propagate_to_master", ProposedValue: "rose"},
	}
	_, err := uc.ApplyProposals(ctx, ApplyProposalsRequest{
		ReportID: 9, ProposalIDs: []string{"p-3"}, ActorID: "c",
		Edits: map[string]ProposalEdit{
			"p-3": {
				FieldDecisions:        editedDecisions,
				TargetMasterProductID: "mp-new-target",
			},
		},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(tx.linkExistingCalls) != 1 {
		t.Fatalf("expected 1 link-existing call")
	}
	call := tx.linkExistingCalls[0]
	if call.mpID != "mp-new-target" {
		t.Fatalf("edit didn't redirect target master: got %s", call.mpID)
	}
	if len(call.propagate) != 1 || call.propagate[0].Field != "tier2.scent" {
		t.Fatalf("edited field decisions not propagated: %+v", call.propagate)
	}
}

func TestApply_AlreadyLinked_NoOp(t *testing.T) {
	ctx := context.Background()
	// Listing is already linked — the proposal action is "already_linked"
	// reflecting that fact. Apply must not call any tx writer.
	catalog := &fakeCatalogPort{listings: map[string]domain.Product{
		"l-4": {ID: "l-4", TenantID: "t-1", MasterProductID: "mp-x", MasterVariantID: "mv-x"},
	}}
	reports := &fakeReportsPort{byID: map[int64]*domain.MergeReport{}}
	tx := &fakeApplyTxPort{}

	seedReport(reports, 11, "t-1", []domain.MergeProposal{{
		ID: "p-4", ListingID: "l-4", Status: domain.MergeProposalStatusPending,
		Action: domain.MergeActionAlreadyLinked,
	}})

	uc := newTestUseCase(t, catalog, reports, tx)
	res, err := uc.ApplyProposals(ctx, ApplyProposalsRequest{
		ReportID: 11, ProposalIDs: []string{"p-4"}, ActorID: "c",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(tx.newMasterCalls)+len(tx.linkExistingCalls)+len(tx.variantOfExistingCalls) != 0 {
		t.Fatalf("already_linked must not call any tx method")
	}
	if res.SkippedCount != 1 {
		t.Fatalf("expected SkippedCount=1, got %d", res.SkippedCount)
	}
}

func TestRevert_RestoresListingLinks_KeepsCreatedMasters(t *testing.T) {
	ctx := context.Background()
	catalog := &fakeCatalogPort{listings: map[string]domain.Product{
		"l-5": {ID: "l-5", TenantID: "t-1"},
	}}
	reports := &fakeReportsPort{byID: map[int64]*domain.MergeReport{}}
	tx := &fakeApplyTxPort{newMasterReturnMP: "mp-orphan", newMasterReturnMV: "mv-orphan"}

	seedReport(reports, 22, "t-1", []domain.MergeProposal{{
		ID: "p-5", ListingID: "l-5", Status: domain.MergeProposalStatusPending,
		Action:         domain.MergeActionNewMaster,
		ProposedMaster: &domain.ProposedMaster{Name: "Foo", Brand: "X"},
	}})

	uc := newTestUseCase(t, catalog, reports, tx)

	// Apply first.
	if _, err := uc.ApplyProposals(ctx, ApplyProposalsRequest{
		ReportID: 22, ProposalIDs: []string{"p-5"}, ActorID: "c",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if reports.lastFinal != domain.MergeReportStatusApplied {
		t.Fatalf("expected applied status before revert")
	}

	// Now revert.
	if err := uc.Revert(ctx, 22, "c"); err != nil {
		t.Fatalf("revert: %v", err)
	}
	if len(tx.restoreCalls) != 1 {
		t.Fatalf("expected 1 RestoreListingLink call, got %d", len(tx.restoreCalls))
	}
	rc := tx.restoreCalls[0]
	if rc.listingID != "l-5" || rc.prevMP != "" || rc.prevMV != "" {
		t.Fatalf("revert should restore to empty FK (listing was unlinked pre-apply): %+v", rc)
	}
	// The created master is intentionally orphaned — applier never tries to
	// undo the master_product write. Curator can clean orphans later.
	if reports.lastFinal != domain.MergeReportStatusReverted {
		t.Fatalf("expected reverted status, got %s", reports.lastFinal)
	}
	if reports.byID[22].Proposals[0].Status != domain.MergeProposalStatusPending {
		t.Fatalf("proposal status should reset to pending after revert")
	}
}

// --- resolveVertical (A2) -------------------------------------------------

func TestResolveVertical_BrandMappingHintWins(t *testing.T) {
	a := &domain.MappingArtifact{
		BrandMapping: map[string]domain.BrandMappingTarget{
			"ikea":  {Action: "create_new", Vertical: "furniture"},
			"cosrx": {Action: "link_existing", MasterBrand: "COSRX", Vertical: "cosmetics"},
		},
	}
	listing := &domain.Product{}

	if got := resolveVertical(a, "ikea", listing); got != "furniture" {
		t.Errorf("create_new with vertical hint → %q, want furniture", got)
	}
	if got := resolveVertical(a, "cosrx", listing); got != "cosmetics" {
		t.Errorf("link_existing with vertical hint → %q, want cosmetics", got)
	}
}

func TestResolveVertical_NoHardcodedCosmetics(t *testing.T) {
	// Regression: link_existing without vertical hint must NOT silently return
	// "cosmetics" — that hardcode broke furniture/footwear tenants.
	a := &domain.MappingArtifact{
		BrandMapping: map[string]domain.BrandMappingTarget{
			"ikea": {Action: "link_existing", MasterBrand: "IKEA"}, // no vertical
		},
	}
	listing := &domain.Product{}

	got := resolveVertical(a, "ikea", listing)
	if got == "cosmetics" {
		t.Errorf("link_existing without vertical hint must NOT return hardcoded 'cosmetics'; got %q", got)
	}
	if got != VerticalUnknown {
		t.Errorf("with no template fallback either, expected %q; got %q", VerticalUnknown, got)
	}
}

func TestResolveVertical_FallsBackToMasterTemplateByCollection(t *testing.T) {
	a := &domain.MappingArtifact{
		BrandMapping: map[string]domain.BrandMappingTarget{
			"randomvendor": {Action: "create_new"}, // no vertical (would normally fail validation, here just to test resolution)
		},
		MasterTemplates: []domain.MasterTemplateProposal{
			{Vertical: "footwear", CategoryHints: []string{"shoes", "sneakers"}},
		},
	}
	listing := &domain.Product{
		RawAttributes: map[string]interface{}{
			"collections": []interface{}{
				map[string]interface{}{"title": "Running Sneakers"},
			},
		},
	}

	if got := resolveVertical(a, "randomvendor", listing); got != "footwear" {
		t.Errorf("collection 'Running Sneakers' should match template hint 'sneakers' → footwear, got %q", got)
	}
}

func TestResolveVertical_UnknownWhenNothingMatches(t *testing.T) {
	a := &domain.MappingArtifact{
		BrandMapping: map[string]domain.BrandMappingTarget{
			"unknownco": {Action: "create_new"},
		},
	}
	listing := &domain.Product{}
	if got := resolveVertical(a, "unknownco", listing); got != VerticalUnknown {
		t.Errorf("expected unknown, got %q", got)
	}
}

// --- lookupPath / extractTier2 metafield resolution -----------------------

func TestLookupPath_DotForm_NamespaceAndKey(t *testing.T) {
	raw := map[string]interface{}{
		"metafields": []interface{}{
			map[string]interface{}{"namespace": "custom", "key": "wood_type", "value": "oak"},
			map[string]interface{}{"namespace": "custom", "key": "frame_material", "value": "steel"},
			map[string]interface{}{"namespace": "other", "key": "wood_type", "value": "ignored"},
		},
	}
	if got := lookupPath(raw, "metafields.custom.wood_type"); got != "oak" {
		t.Errorf("metafields.custom.wood_type = %v, want oak", got)
	}
	if got := lookupPath(raw, "metafields.custom.frame_material"); got != "steel" {
		t.Errorf("metafields.custom.frame_material = %v, want steel", got)
	}
	// Different namespace must not match.
	if got := lookupPath(raw, "metafields.other.frame_material"); got != nil {
		t.Errorf("expected nil for namespace mismatch, got %v", got)
	}
}

func TestLookupPath_BracketFormStillWorks(t *testing.T) {
	raw := map[string]interface{}{
		"metafields": []interface{}{
			map[string]interface{}{"namespace": "custom", "key": "wood_type", "value": "oak"},
		},
	}
	if got := lookupPath(raw, "metafields[key=wood_type].value"); got != "oak" {
		t.Errorf("legacy bracket form lost: %v", got)
	}
}

func TestLookupPath_DirectKey(t *testing.T) {
	raw := map[string]interface{}{"vendor": "IKEA"}
	if got := lookupPath(raw, "vendor"); got != "IKEA" {
		t.Errorf("direct key broken: %v", got)
	}
}
