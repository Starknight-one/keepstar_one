package usecases

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// fakeInbox is an in-memory InboxPort just rich enough for ApplyForTenant.
// We only implement the methods apply_v2 actually calls; the rest panic so a
// missed branch shows up loudly in tests.
type fakeInbox struct {
	items   []*domain.InboxItem
	applied map[string]bool
}

func newFakeInbox(items ...*domain.InboxItem) *fakeInbox {
	return &fakeInbox{items: items, applied: map[string]bool{}}
}

func (f *fakeInbox) Upsert(_ context.Context, _ *domain.InboxItem) (bool, error) {
	panic("fakeInbox.Upsert not implemented")
}
func (f *fakeInbox) MarkApplied(_ context.Context, id string) error {
	f.applied[id] = true
	return nil
}
func (f *fakeInbox) DeleteByTenant(_ context.Context, _ string) error {
	panic("fakeInbox.DeleteByTenant not implemented")
}
func (f *fakeInbox) GetByID(_ context.Context, id string) (*domain.InboxItem, error) {
	for _, it := range f.items {
		if it.ID == id {
			return it, nil
		}
	}
	return nil, nil
}
func (f *fakeInbox) ListUnapplied(_ context.Context, tenantID string, limit, offset int) ([]*domain.InboxItem, error) {
	out := []*domain.InboxItem{}
	for _, it := range f.items {
		if it.TenantID != tenantID {
			continue
		}
		if f.applied[it.ID] {
			continue
		}
		out = append(out, it)
	}
	if offset >= len(out) {
		return nil, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}
func (f *fakeInbox) CountTotal(_ context.Context, _ string) (int, error)        { return len(f.items), nil }
func (f *fakeInbox) ListFields(_ context.Context, _ string, _ int) ([]string, error)  { return nil, nil }
func (f *fakeInbox) SampleValues(_ context.Context, _ string, _ string, _ int) ([]string, error) {
	return nil, nil
}
func (f *fakeInbox) CountBy(_ context.Context, _ string, _ string, _ int) (map[string]int, error) {
	return nil, nil
}
func (f *fakeInbox) FieldStats(_ context.Context, _ string, _ string, _ int) (*domain.InboxFieldStats, error) {
	return nil, nil
}
func (f *fakeInbox) PeekFullRows(_ context.Context, _ string, _ int) ([]*domain.InboxItem, error) {
	return nil, nil
}

// fakeArtifact returns the same v3 artifact for every Get.
type fakeArtifact struct {
	art *domain.MappingArtifactV3
}

func (f *fakeArtifact) Save(_ context.Context, _ string, _ *domain.MappingArtifactV3) error {
	return nil
}
func (f *fakeArtifact) Get(_ context.Context, _ string) (*domain.MappingArtifactV3, *ports.MappingArtifactMeta, error) {
	return f.art, &ports.MappingArtifactMeta{Status: "active"}, nil
}
func (f *fakeArtifact) MarkStale(_ context.Context, _ string) error { return nil }

// fakeWriter implements CatalogV2WriterPort with deterministic in-memory state.
// Match cascade mirrors the adapter: SKU exact → bind, GTIN exact → bind,
// normalized_match_key exact → bind, else INSERT. Records every call so tests
// can assert apply_v2 didn't write Tier2/tier3 on bind.
type fakeWriter struct {
	bySKU      map[string]string // sku → masterID
	byGTIN     map[string]string
	byMatchKey map[string]string
	masters    map[string]*ports.MasterProductUpsert
	cosmetics  map[string]*ports.MasterCosmeticsUpsert
	tier3      map[string]map[string]any
	listings   []*ports.ListingUpsert
	aliases    map[string]string // lowercased product_type → vertical

	createCalls int
	bindCalls   int
}

func newFakeWriter() *fakeWriter {
	return &fakeWriter{
		bySKU:      map[string]string{},
		byGTIN:     map[string]string{},
		byMatchKey: map[string]string{},
		masters:    map[string]*ports.MasterProductUpsert{},
		cosmetics:  map[string]*ports.MasterCosmeticsUpsert{},
		tier3:      map[string]map[string]any{},
		aliases:    map[string]string{},
	}
}

func (w *fakeWriter) MatchOrCreateMaster(_ context.Context, mp *ports.MasterProductUpsert) (string, bool, error) {
	if id, ok := w.bySKU[mp.SKU]; ok && mp.SKU != "" {
		w.bindCalls++
		return id, false, nil
	}
	if mp.GTIN != "" {
		if id, ok := w.byGTIN[mp.GTIN]; ok {
			w.bindCalls++
			return id, false, nil
		}
	}
	if mp.NormalizedMatchKey != "" {
		if id, ok := w.byMatchKey[mp.NormalizedMatchKey]; ok {
			w.bindCalls++
			return id, false, nil
		}
	}
	w.createCalls++
	id := "master-" + mp.SKU
	w.bySKU[mp.SKU] = id
	if mp.GTIN != "" {
		w.byGTIN[mp.GTIN] = id
	}
	if mp.NormalizedMatchKey != "" {
		w.byMatchKey[mp.NormalizedMatchKey] = id
	}
	// Copy so callers can't mutate.
	cp := *mp
	w.masters[id] = &cp
	return id, true, nil
}
func (w *fakeWriter) UpsertCosmetics(_ context.Context, masterID string, f *ports.MasterCosmeticsUpsert) error {
	cp := *f
	w.cosmetics[masterID] = &cp
	return nil
}
func (w *fakeWriter) MergeTier3(_ context.Context, masterID string, patch map[string]any) error {
	if w.tier3[masterID] == nil {
		w.tier3[masterID] = map[string]any{}
	}
	for k, v := range patch {
		w.tier3[masterID][k] = v
	}
	return nil
}
func (w *fakeWriter) UpsertListing(_ context.Context, lst *ports.ListingUpsert) (string, error) {
	cp := *lst
	w.listings = append(w.listings, &cp)
	return "listing-" + lst.SourceID, nil
}
func (w *fakeWriter) LookupVertical(_ context.Context, pt string) (string, bool, error) {
	v, ok := w.aliases[strings.ToLower(strings.TrimSpace(pt))]
	return v, ok, nil
}
func (w *fakeWriter) SoftDeleteListing(_ context.Context, _, _, _ string) error { return nil }

type fakeActionLog struct {
	entries []*ports.TenantActionLogEntry
}

func (f *fakeActionLog) Log(_ context.Context, e *ports.TenantActionLogEntry) error {
	cp := *e
	f.entries = append(f.entries, &cp)
	return nil
}
func (f *fakeActionLog) List(_ context.Context, _ string, _ int, _ int) ([]*ports.TenantActionLogEntry, error) {
	return f.entries, nil
}

// --- helpers ---

func mkInbox(id, ext string, raw map[string]any) *domain.InboxItem {
	body, _ := json.Marshal(raw)
	return &domain.InboxItem{
		ID:         id,
		TenantID:   "t-test",
		SourceKind: domain.InboxSourceShopify,
		ExternalID: ext,
		Raw:        body,
		FetchedAt:  time.Now(),
	}
}

// cosmeticsArtifact has both cosmetics and unknown branches — the unknown
// branch is the safety net real discovery_v2 always commits, so tests that
// route to unknown can still apply without a mapping_miss.
func cosmeticsArtifact() *domain.MappingArtifactV3 {
	common := []domain.FieldMappingRule{
		{From: "title", To: "master.name"},
		{From: "vendor", To: "master.brand"},
		{From: "variants[0].sku", To: "master.sku"},
	}
	return &domain.MappingArtifactV3{
		Version: 3,
		Branches: []domain.VerticalBranch{
			{
				Vertical: "cosmetics",
				FieldMap: append(append([]domain.FieldMappingRule{}, common...),
					domain.FieldMappingRule{From: "skin_type", To: "cosmetics.skin_type", Transform: "split:,"},
					domain.FieldMappingRule{From: "volume", To: "cosmetics.volume_ml", Transform: "ml_from_string"},
				),
			},
			{Vertical: "unknown", FieldMap: common},
		},
	}
}

// seedCosmeticsAlias wires the writer's vertical_aliases so common cosmetic
// product_type strings classify as "cosmetics" — matches the real seed list.
func seedCosmeticsAlias(w *fakeWriter) {
	w.aliases["serum"] = "cosmetics"
	w.aliases["skincare"] = "cosmetics"
	w.aliases["cream"] = "cosmetics"
	w.aliases["toner"] = "cosmetics"
}

// multiBranchArtifact has cosmetics + electronics + unknown branches.
func multiBranchArtifact() *domain.MappingArtifactV3 {
	return &domain.MappingArtifactV3{
		Version: 3,
		Branches: []domain.VerticalBranch{
			{
				Vertical: "cosmetics",
				FieldMap: []domain.FieldMappingRule{
					{From: "title", To: "master.name"},
					{From: "vendor", To: "master.brand"},
					{From: "variants[0].sku", To: "master.sku"},
					{From: "skin_type", To: "cosmetics.skin_type", Transform: "split:,"},
				},
			},
			{
				Vertical: "electronics",
				FieldMap: []domain.FieldMappingRule{
					{From: "title", To: "master.name"},
					{From: "vendor", To: "master.brand"},
					{From: "variants[0].sku", To: "master.sku"},
					{From: "cpu", To: "tier3.cpu"},
					{From: "ram_gb", To: "tier3.ram_gb", Transform: "int"},
				},
			},
			{
				Vertical: "unknown",
				FieldMap: []domain.FieldMappingRule{
					{From: "title", To: "master.name"},
					{From: "vendor", To: "master.brand"},
					{From: "variants[0].sku", To: "master.sku"},
				},
			},
		},
		ClassifyRules: []domain.ClassifyRule{
			{When: "brand = 'Apple'", ThenVertical: "electronics"},
		},
	}
}

func mkApply(art *domain.MappingArtifactV3) (*ApplyV2UseCase, *fakeInbox, *fakeWriter, *fakeActionLog) {
	inbox := newFakeInbox()
	writer := newFakeWriter()
	actionLog := &fakeActionLog{}
	uc := NewApplyV2(
		inbox,
		&fakeArtifact{art: art},
		writer,
		actionLog,
		nil, // no discovery — tests stay synchronous
		logger.New("error"),
	)
	return uc, inbox, writer, actionLog
}

// --- tests ---

func TestApplyForTenant_HappyPathCosmetics(t *testing.T) {
	uc, inbox, writer, _ := mkApply(cosmeticsArtifact())
	seedCosmeticsAlias(writer)
	inbox.items = []*domain.InboxItem{
		mkInbox("i1", "gid://shopify/Product/1", map[string]any{
			"title":        "Hyaluronic Serum",
			"vendor":       "Ordinary",
			"product_type": "Serum",
			"variants":     []any{map[string]any{"sku": "ORD-HA-30"}},
			"skin_type":    "oily, dry",
			"volume":       "30 ml",
		}),
	}
	res, err := uc.ApplyForTenant(context.Background(), "t-test")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Applied != 1 || res.MappingMisses != 0 || res.Errors != 0 {
		t.Fatalf("res = %+v, want applied=1 misses=0 errors=0", res)
	}
	if writer.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", writer.createCalls)
	}
	mp := writer.masters["master-ORD-HA-30"]
	if mp == nil {
		t.Fatal("master not stored under expected SKU")
	}
	if mp.Vertical != "cosmetics" {
		t.Errorf("vertical = %q, want cosmetics", mp.Vertical)
	}
	cos := writer.cosmetics["master-ORD-HA-30"]
	if cos == nil {
		t.Fatal("cosmetics row not written")
	}
	if len(cos.SkinType) != 2 || cos.SkinType[0] != "oily" {
		t.Errorf("skin_type = %v, want [oily dry]", cos.SkinType)
	}
	if cos.VolumeML == nil || *cos.VolumeML != 30 {
		t.Errorf("volume_ml = %v, want 30", cos.VolumeML)
	}
	if len(writer.listings) != 1 {
		t.Fatalf("listings = %d, want 1", len(writer.listings))
	}
}

func TestApplyForTenant_MasterImmutableOnBind(t *testing.T) {
	// Pre-seed an existing master under SKU "SHARED-1" with brand=A.
	uc, inbox, writer, _ := mkApply(cosmeticsArtifact())
	seedCosmeticsAlias(writer)
	writer.bySKU["SHARED-1"] = "master-SHARED-1"
	writer.masters["master-SHARED-1"] = &ports.MasterProductUpsert{
		SKU:      "SHARED-1",
		Name:     "Original Name",
		Brand:    "Original Brand",
		Vertical: "cosmetics",
	}

	// New tenant inbox row with the same SKU but different brand/name.
	inbox.items = []*domain.InboxItem{
		mkInbox("i1", "gid://shopify/Product/2", map[string]any{
			"title":        "RENAMED",
			"vendor":       "DIFFERENT BRAND",
			"product_type": "Serum",
			"variants":     []any{map[string]any{"sku": "SHARED-1"}},
		}),
	}
	res, err := uc.ApplyForTenant(context.Background(), "t-test")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Applied != 1 {
		t.Fatalf("applied = %d, want 1", res.Applied)
	}
	if writer.bindCalls != 1 || writer.createCalls != 0 {
		t.Fatalf("bind=%d create=%d, want bind=1 create=0", writer.bindCalls, writer.createCalls)
	}
	mp := writer.masters["master-SHARED-1"]
	if mp.Name != "Original Name" || mp.Brand != "Original Brand" {
		t.Errorf("master mutated on bind: name=%q brand=%q", mp.Name, mp.Brand)
	}
	// On bind we MUST NOT write cosmetics or tier3.
	if _, ok := writer.cosmetics["master-SHARED-1"]; ok {
		t.Errorf("cosmetics written on bind path — should be skipped")
	}
	if _, ok := writer.tier3["master-SHARED-1"]; ok {
		t.Errorf("tier3 written on bind path — should be skipped")
	}
	// Listing always upserted (tenant overlay).
	if len(writer.listings) != 1 {
		t.Fatalf("listings = %d, want 1", len(writer.listings))
	}
}

func TestApplyForTenant_PerRowVerticalRouting(t *testing.T) {
	// Multi-branch artifact + classify_rule on brand=Apple → electronics.
	uc, inbox, writer, _ := mkApply(multiBranchArtifact())
	seedCosmeticsAlias(writer)
	inbox.items = []*domain.InboxItem{
		mkInbox("i1", "gid://shopify/Product/1", map[string]any{
			"title":        "Niacinamide Serum",
			"vendor":       "Ordinary",
			"product_type": "Serum",
			"variants":     []any{map[string]any{"sku": "ORD-NIA-30"}},
			"skin_type":    "oily",
		}),
		mkInbox("i2", "gid://shopify/Product/2", map[string]any{
			"title":    "MacBook Air",
			"vendor":   "Apple",
			"variants": []any{map[string]any{"sku": "MBA-1"}},
			"cpu":      "M3",
			"ram_gb":   "16",
		}),
	}
	res, err := uc.ApplyForTenant(context.Background(), "t-test")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Applied != 2 || res.MappingMisses != 0 {
		t.Fatalf("res = %+v, want applied=2 misses=0", res)
	}
	cosm := writer.masters["master-ORD-NIA-30"]
	if cosm == nil || cosm.Vertical != "cosmetics" {
		t.Errorf("cosmetics row missing or wrong vertical: %+v", cosm)
	}
	if _, ok := writer.cosmetics["master-ORD-NIA-30"]; !ok {
		t.Error("cosmetics row not written for serum")
	}
	elec := writer.masters["master-MBA-1"]
	if elec == nil || elec.Vertical != "electronics" {
		t.Errorf("electronics row missing or wrong vertical: %+v", elec)
	}
	if _, ok := writer.cosmetics["master-MBA-1"]; ok {
		t.Error("cosmetics written for electronics row — should not be")
	}
	// tier3 must have cpu+ram for laptop.
	t3 := writer.tier3["master-MBA-1"]
	if t3 == nil || t3["cpu"] != "M3" {
		t.Errorf("tier3 cpu missing: %+v", t3)
	}
	if t3["ram_gb"] != 16 {
		t.Errorf("tier3 ram_gb = %v, want 16 (int)", t3["ram_gb"])
	}
}

func TestApplyForTenant_TierThreeFallbackForNonCosmeticsRuleInCosmeticsBranch(t *testing.T) {
	// Single cosmetics branch, but row is electronics (rule says so). Agent's
	// cosmetics.* rule against an electronics row → forgiving tier3 routing.
	art := &domain.MappingArtifactV3{
		Version: 3,
		Branches: []domain.VerticalBranch{
			{
				Vertical: "cosmetics",
				FieldMap: []domain.FieldMappingRule{
					{From: "title", To: "master.name"},
					{From: "vendor", To: "master.brand"},
					{From: "variants[0].sku", To: "master.sku"},
					{From: "spf", To: "cosmetics.spf", Transform: "int"},
				},
			},
			{
				Vertical: "unknown",
				FieldMap: []domain.FieldMappingRule{
					{From: "title", To: "master.name"},
					{From: "vendor", To: "master.brand"},
					{From: "variants[0].sku", To: "master.sku"},
					// Rule routed to cosmetics.* but row is unknown vertical →
					// forgiving fallback to tier3 with the stripped key.
					{From: "spf", To: "cosmetics.spf", Transform: "int"},
				},
			},
		},
	}
	uc, inbox, writer, _ := mkApply(art)
	inbox.items = []*domain.InboxItem{
		mkInbox("i1", "gid://shopify/Product/1", map[string]any{
			"title":    "Random Widget",
			"vendor":   "Anon",
			"variants": []any{map[string]any{"sku": "X-1"}},
			"spf":      "30",
		}),
	}
	res, err := uc.ApplyForTenant(context.Background(), "t-test")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Applied != 1 || res.MappingMisses != 0 {
		t.Fatalf("res = %+v", res)
	}
	mp := writer.masters["master-X-1"]
	if mp == nil || mp.Vertical != "unknown" {
		t.Errorf("vertical = %q, want unknown", mp.Vertical)
	}
	if _, ok := writer.cosmetics["master-X-1"]; ok {
		t.Error("cosmetics written for unknown-vertical row — should be tier3 fallback")
	}
	t3 := writer.tier3["master-X-1"]
	if t3 == nil || t3["spf"] != 30 {
		t.Errorf("tier3 fallback spf = %v, want 30", t3)
	}
}

func TestApplyForTenant_RejectsNoSKUNoName(t *testing.T) {
	uc, inbox, _, _ := mkApply(cosmeticsArtifact())
	inbox.items = []*domain.InboxItem{
		mkInbox("i1", "gid://shopify/Product/junk", map[string]any{
			// No title, no SKU — apply_v2 raises mapping_miss.
			"vendor": "Mystery",
		}),
	}
	res, err := uc.ApplyForTenant(context.Background(), "t-test")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.MappingMisses != 1 || res.Applied != 0 {
		t.Fatalf("res = %+v, want misses=1 applied=0", res)
	}
}

func TestApplyForTenant_SynthesizesSKUWhenNameOnlyPresent(t *testing.T) {
	uc, inbox, writer, _ := mkApply(cosmeticsArtifact())
	seedCosmeticsAlias(writer)
	inbox.items = []*domain.InboxItem{
		mkInbox("i1", "gid://shopify/Product/9", map[string]any{
			"title":        "Toner Without SKU",
			"vendor":       "Pioneer",
			"product_type": "Toner",
			// no variants — sku stays empty after rule walk.
		}),
	}
	res, err := uc.ApplyForTenant(context.Background(), "t-test")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Applied != 1 || res.MappingMisses != 0 {
		t.Fatalf("res = %+v, want applied=1", res)
	}
	// Synthetic SKU pattern: auto-<source>-<external_id>.
	wantSKU := "auto-shopify-gid://shopify/Product/9"
	if _, ok := writer.bySKU[wantSKU]; !ok {
		t.Errorf("expected synthetic sku %q in bySKU map, have %v", wantSKU, writer.bySKU)
	}
}

func TestApplyForTenant_MapsGTINOnMasterAndStripsDigits(t *testing.T) {
	art := &domain.MappingArtifactV3{
		Version: 3,
		Branches: []domain.VerticalBranch{{
			Vertical: "cosmetics",
			FieldMap: []domain.FieldMappingRule{
				{From: "title", To: "master.name"},
				{From: "vendor", To: "master.brand"},
				{From: "variants[0].sku", To: "master.sku"},
				{From: "variants[0].barcode", To: "master.gtin"},
			},
		}},
	}
	uc, inbox, writer, _ := mkApply(art)
	seedCosmeticsAlias(writer)
	inbox.items = []*domain.InboxItem{
		mkInbox("i1", "gid://shopify/Product/1", map[string]any{
			"title":        "X",
			"vendor":       "Y",
			"product_type": "Cream",
			"variants": []any{map[string]any{
				"sku":     "Z-1",
				"barcode": "  8-800001 000001 ",
			}},
		}),
	}
	if _, err := uc.ApplyForTenant(context.Background(), "t-test"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	mp := writer.masters["master-Z-1"]
	if mp == nil {
		t.Fatal("master missing")
	}
	if mp.GTIN != "8800001000001" {
		t.Errorf("gtin = %q, want 8800001000001 (digits only)", mp.GTIN)
	}
}

func TestApplyForTenant_ListingFieldsWritten(t *testing.T) {
	art := &domain.MappingArtifactV3{
		Version: 3,
		Branches: []domain.VerticalBranch{{
			Vertical: "cosmetics",
			FieldMap: []domain.FieldMappingRule{
				{From: "title", To: "master.name"},
				{From: "vendor", To: "master.brand"},
				{From: "variants[0].sku", To: "master.sku"},
				{From: "variants[0].price", To: "listing.price_cents", Transform: "int"},
				{From: "variants[0].currency", To: "listing.currency"},
				{From: "variants[0].stock", To: "listing.stock", Transform: "int"},
			},
		}},
	}
	uc, inbox, writer, _ := mkApply(art)
	seedCosmeticsAlias(writer)
	inbox.items = []*domain.InboxItem{
		mkInbox("i1", "gid://shopify/Product/1", map[string]any{
			"title":        "Cream",
			"vendor":       "Brand",
			"product_type": "Cream",
			"variants": []any{map[string]any{
				"sku":      "C-1",
				"price":    "1499",
				"currency": "USD",
				"stock":    "12",
			}},
		}),
	}
	if _, err := uc.ApplyForTenant(context.Background(), "t-test"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(writer.listings) != 1 {
		t.Fatalf("listings = %d, want 1", len(writer.listings))
	}
	lst := writer.listings[0]
	if lst.Price != 1499 || lst.Currency != "USD" || lst.Stock != 12 {
		t.Errorf("listing = %+v", lst)
	}
}

func TestApplyForTenant_AliasLookupBeatsClassifyRule(t *testing.T) {
	uc, inbox, writer, _ := mkApply(multiBranchArtifact())
	// vertical_aliases says product_type=Skincare → cosmetics. classify_rule
	// says brand=Apple → electronics. Apple-branded skincare row hits the
	// alias first and routes to cosmetics.
	writer.aliases["skincare"] = "cosmetics"
	inbox.items = []*domain.InboxItem{
		mkInbox("i1", "gid://shopify/Product/9", map[string]any{
			"title":        "Apple-branded Toner",
			"vendor":       "Apple",
			"product_type": "Skincare",
			"variants":     []any{map[string]any{"sku": "AT-1"}},
		}),
	}
	if _, err := uc.ApplyForTenant(context.Background(), "t-test"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	mp := writer.masters["master-AT-1"]
	if mp.Vertical != "cosmetics" {
		t.Errorf("alias should beat rule: vertical = %q, want cosmetics", mp.Vertical)
	}
}

func TestApplyForTenant_ActionLogEmittedWithStatus(t *testing.T) {
	uc, inbox, writer, log := mkApply(cosmeticsArtifact())
	seedCosmeticsAlias(writer)
	inbox.items = []*domain.InboxItem{
		mkInbox("i1", "gid://shopify/Product/1", map[string]any{
			"title":        "Cream",
			"vendor":       "Brand",
			"product_type": "Cream",
			"variants":     []any{map[string]any{"sku": "C-1"}},
		}),
		// Junk row — no title and no sku → mapping_miss.
		mkInbox("i2", "gid://shopify/Product/2", map[string]any{
			"vendor":       "Other",
			"product_type": "Cream",
		}),
	}
	res, err := uc.ApplyForTenant(context.Background(), "t-test")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Applied != 1 || res.MappingMisses != 1 {
		t.Fatalf("res = %+v", res)
	}
	// Expect one "apply" log entry with status=warning (mix of applied + misses).
	var applyEntries []*ports.TenantActionLogEntry
	for _, e := range log.entries {
		if e.Action == "apply" {
			applyEntries = append(applyEntries, e)
		}
	}
	if len(applyEntries) != 1 {
		t.Fatalf("apply entries = %d, want 1", len(applyEntries))
	}
	if applyEntries[0].Status != "warning" {
		t.Errorf("status = %q, want warning", applyEntries[0].Status)
	}
}

func TestApplyForTenant_MarksItemsApplied(t *testing.T) {
	uc, inbox, writer, _ := mkApply(cosmeticsArtifact())
	seedCosmeticsAlias(writer)
	inbox.items = []*domain.InboxItem{
		mkInbox("i1", "gid://shopify/Product/1", map[string]any{
			"title":        "Cream",
			"vendor":       "Brand",
			"product_type": "Cream",
			"variants":     []any{map[string]any{"sku": "C-1"}},
		}),
	}
	if _, err := uc.ApplyForTenant(context.Background(), "t-test"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !inbox.applied["i1"] {
		t.Error("i1 should be marked applied")
	}
}
