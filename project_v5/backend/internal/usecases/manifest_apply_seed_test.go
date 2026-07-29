package usecases

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/operations/seed"
	"keepstar_v5/internal/ports"
)

// seed_demo_data (V2_SPEC.md L6 + R3). What these tests hold in place:
// listings go through the SAME admin import real uploads use, records land
// in the entities the conversation defined, everything is flagged demo, and
// a re-run seeds nothing twice (R22).

// --- fakes ---

// seedGateway records imports and answers status from a script.
type seedGateway struct {
	mu sync.Mutex
	applierGateway
	imports     []seedImport
	statusQueue []ports.AdminImportStatus // consumed in order; last one repeats
	startErr    error
}

type seedImport struct {
	tenantSlug string
	fileName   string
	body       string
}

func (g *seedGateway) StartImport(_ context.Context, tenantSlug, fileName string, file io.Reader) (*ports.AdminImportJob, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.startErr != nil {
		return nil, g.startErr
	}
	raw, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	g.imports = append(g.imports, seedImport{tenantSlug: tenantSlug, fileName: fileName, body: string(raw)})
	return &ports.AdminImportJob{JobID: fmt.Sprintf("job-%d", len(g.imports)), Status: "pending", TotalItems: 0}, nil
}

func (g *seedGateway) ImportStatus(_ context.Context, _, jobID string) (*ports.AdminImportStatus, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.statusQueue) == 0 {
		return &ports.AdminImportStatus{JobID: jobID, Status: "completed", Processed: 17, ProjectionRows: 17, Invalidated: true}, nil
	}
	st := g.statusQueue[0]
	if len(g.statusQueue) > 1 {
		g.statusQueue = g.statusQueue[1:]
	}
	st.JobID = jobID
	return &st, nil
}

// seedEntities is an in-memory EntityPort: definitions and value sets as the
// applier's own steps write them, records with the flat attribute filter
// QueryRecords needs for the re-run exists-check.
type seedEntities struct {
	applierEntities
	defs      []domain.EntityDefinition
	valueSets map[string]*domain.ValueSet
	records   []domain.EntityRecord
	createErr error
}

func newSeedEntities() *seedEntities {
	return &seedEntities{valueSets: map[string]*domain.ValueSet{}}
}

func (e *seedEntities) UpsertEntityDefinition(_ context.Context, def *domain.EntityDefinition) error {
	for i := range e.defs {
		if e.defs[i].Slug == def.Slug {
			e.defs[i] = *def
			return nil
		}
	}
	e.defs = append(e.defs, *def)
	return nil
}

func (e *seedEntities) ListEntityDefinitions(context.Context, string) ([]domain.EntityDefinition, error) {
	return e.defs, nil
}

func (e *seedEntities) UpsertValueSet(_ context.Context, vs *domain.ValueSet) error {
	cp := *vs
	e.valueSets[vs.Slug] = &cp
	return nil
}

func (e *seedEntities) GetValueSet(_ context.Context, _, slug string) (*domain.ValueSet, error) {
	vs, ok := e.valueSets[slug]
	if !ok {
		return nil, errors.New("value set not found")
	}
	return vs, nil
}

func (e *seedEntities) CreateRecord(_ context.Context, rec *domain.EntityRecord) (*domain.EntityRecord, *domain.RuntimeEvent, error) {
	if e.createErr != nil {
		return nil, nil, e.createErr
	}
	rec.ID = fmt.Sprintf("rec-%d", len(e.records)+1)
	if rec.Status == "" {
		for _, d := range e.defs {
			if d.Slug == rec.EntitySlug && d.StatusField != "" {
				rec.Status, _ = rec.Data[d.StatusField].(string)
			}
		}
	}
	e.records = append(e.records, *rec)
	return rec, &domain.RuntimeEvent{EventType: domain.EventRecordCreated, EntitySlug: rec.EntitySlug}, nil
}

// QueryRecords answers the exists-check: entity slug + the flat Attrs
// equality the postgres adapter implements as data->>'key' = value.
func (e *seedEntities) QueryRecords(_ context.Context, _, entitySlug string, f domain.RecordFilter) ([]domain.EntityRecord, int, error) {
	var out []domain.EntityRecord
	for _, rec := range e.records {
		if rec.EntitySlug != entitySlug {
			continue
		}
		match := true
		for k, want := range f.Attrs {
			if got, _ := rec.Data[k].(string); got != want {
				match = false
				break
			}
		}
		if match {
			out = append(out, rec)
		}
	}
	return out, len(out), nil
}

func (e *seedEntities) recordsOf(entitySlug string) []domain.EntityRecord {
	var out []domain.EntityRecord
	for _, rec := range e.records {
		if rec.EntitySlug == entitySlug {
			out = append(out, rec)
		}
	}
	return out
}

// --- fixture ---

type seedFixture struct {
	state    *applierStatePort
	gateway  *seedGateway
	entities *seedEntities
	ap       *ManifestApplier
}

func newSeedFixture(m *domain.OnboardingManifest) *seedFixture {
	f := &seedFixture{
		state:    &applierStatePort{manifest: m},
		gateway:  &seedGateway{},
		entities: newSeedEntities(),
	}
	f.ap = NewManifestApplier(ManifestApplierConfig{
		State:          f.state,
		Tokens:         &applierTokens{},
		Gateway:        f.gateway,
		Entities:       f.entities,
		Ops:            &applierOps{},
		Themes:         &applierThemes{},
		Registry:       &applierRegistry{},
		SurfaceBaseURL: "https://v5.example.test",
		Log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return f
}

// realtorSeedManifest is a fresh plan for a realty agency. seed_demo_data is
// staged FIRST on purpose: the applier must run it after the data model
// exists, whatever order the model emitted (orderedStepIndices).
func realtorSeedManifest() *domain.OnboardingManifest {
	return &domain.OnboardingManifest{
		Version: 1,
		Steps: []domain.ManifestStep{
			step("s-seed", "seed_demo_data", map[string]any{}),
			step("s-tenant", "create_tenant", map[string]any{"name": "Blue Harbor Realty", "vertical": "real estate agency"}),
			step("s-vset", "define_value_set", map[string]any{
				"slug": "lead_pipeline", "name": "Lead pipeline",
				"values": []any{
					map[string]any{"value": "new", "label": "New"},
					map[string]any{"value": "contacted", "label": "Contacted"},
					map[string]any{"value": "showing_booked", "label": "Showing booked"},
					map[string]any{"value": "closed", "label": "Closed"},
				},
			}),
			step("s-lead", "define_entity", map[string]any{
				"entity": map[string]any{
					"slug": "lead", "name": "Lead",
					"fields": []any{
						map[string]any{"key": "name", "label": "Name", "type": "text"},
						map[string]any{"key": "phone", "label": "Phone", "type": "phone"},
						map[string]any{"key": "email", "label": "Email", "type": "email"},
						map[string]any{"key": "message", "label": "Message", "type": "text"},
						map[string]any{"key": "preferredTime", "label": "Preferred time", "type": "datetime"},
						map[string]any{"key": "status", "label": "Status", "type": "enum", "valueSetRef": "lead_pipeline"},
					},
				},
			}),
			step("s-contact", "define_entity", map[string]any{
				"entity": map[string]any{
					"slug": "contact", "name": "Contact",
					"fields": []any{
						map[string]any{"key": "name", "label": "Name", "type": "text"},
						map[string]any{"key": "phone", "label": "Phone", "type": "phone"},
						map[string]any{"key": "email", "label": "Email", "type": "email"},
						map[string]any{"key": "note", "label": "Note", "type": "text"},
						map[string]any{"key": "role", "label": "Role", "type": "text"},
					},
				},
			}),
			step("s-urls", "issue_surface_urls", map[string]any{}),
		},
	}
}

// realtorSeedManifestWithDoor is the same plan plus the uploader door — the
// shape the upload-first path (a visitor dropping their own CSV) runs on.
func realtorSeedManifestWithDoor() *domain.OnboardingManifest {
	m := realtorSeedManifest()
	m.Steps = append(m.Steps, step("s-door", "issue_ingest_door", map[string]any{"formats": []any{"csv", "json"}}))
	return m
}

// --- tests ---

// The whole law in one walk: a fresh manifest with entities defined seeds
// listings through the admin import door, leads and contacts through the
// entity plane, everything flagged demo — and the surfaces are only issued
// afterwards.
func TestSeedDemoDataSeedsBothPlanes(t *testing.T) {
	f := newSeedFixture(realtorSeedManifest())
	pack := seed.RealtyDemoPack()

	m, err := f.ap.Apply(context.Background(), "sess-seed", "")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	st := stepByID(t, m, "s-seed")
	if st.Status != domain.ManifestStepApplied {
		t.Fatalf("seed step status = %s (%s)", st.Status, st.Error)
	}

	// Listings: ONE import, the pack's own file, through the gateway the
	// upload path uses.
	if len(f.gateway.imports) != 1 {
		t.Fatalf("expected exactly 1 catalog import, got %d", len(f.gateway.imports))
	}
	imp := f.gateway.imports[0]
	if imp.tenantSlug != m.Tenant.Slug || imp.fileName != pack.FileName {
		t.Errorf("import addressed %s/%s, want %s/%s", imp.tenantSlug, imp.fileName, m.Tenant.Slug, pack.FileName)
	}
	rows, err := csv.NewReader(strings.NewReader(imp.body)).ReadAll()
	if err != nil {
		t.Fatalf("imported body is not a table: %v", err)
	}
	if len(rows) != len(pack.Listings)+1 {
		t.Errorf("imported %d rows, want %d listings + header", len(rows)-1, len(pack.Listings))
	}
	if st.Result["listings"] != 17 {
		t.Errorf("step result listings = %v, want the 17 rows admin reported", st.Result["listings"])
	}
	if st.Result["pack"] != pack.Name {
		t.Errorf("step result pack = %v, want %q", st.Result["pack"], pack.Name)
	}
	// R7 whole: the projection rebuild is what the storefront actually reads.
	if st.Result["projectionRows"] != 17 || st.Result["invalidated"] != true {
		t.Errorf("step result drops the honest import status: %v", st.Result)
	}

	// Records: leads AND contacts, in the entities the conversation defined.
	leads := f.entities.recordsOf("lead")
	contacts := f.entities.recordsOf("contact")
	if len(leads) != len(pack.Leads) {
		t.Errorf("seeded %d leads, want %d", len(leads), len(pack.Leads))
	}
	if len(contacts) != len(pack.Contacts) {
		t.Errorf("seeded %d contacts, want %d", len(contacts), len(pack.Contacts))
	}
	if want := len(pack.Leads) + len(pack.Contacts); st.Result["records"] != want {
		t.Errorf("step result records = %v, want %d", st.Result["records"], want)
	}

	// The demo flag is what makes L6's later purge possible — every record.
	statuses := map[string]bool{}
	for _, rec := range append(append([]domain.EntityRecord{}, leads...), contacts...) {
		if rec.Data[seed.DemoFlagKey] != true {
			t.Errorf("record %s is not flagged demo: %v", rec.ID, rec.Data)
		}
		if rec.Data[seed.DemoPackKey] != pack.Name {
			t.Errorf("record %s carries pack %v, want %q", rec.ID, rec.Data[seed.DemoPackKey], pack.Name)
		}
		if key, _ := rec.Data[seed.DemoRecordKey].(string); key == "" {
			t.Errorf("record %s has no demo key — a re-run could not skip it", rec.ID)
		}
	}
	// Leads carry the tenant's own pipeline values (mirrored onto status).
	allowed := map[string]bool{"new": true, "contacted": true, "showing_booked": true, "closed": true}
	for _, rec := range leads {
		got, _ := rec.Data["status"].(string)
		if !allowed[got] {
			t.Errorf("lead %s has status %q, outside the tenant's value set", rec.ID, got)
		}
		statuses[got] = true
		if rec.Status != got {
			t.Errorf("lead %s status mirror = %q, want %q", rec.ID, rec.Status, got)
		}
		if _, ok := rec.Data["phone"].(string); !ok {
			t.Errorf("lead %s lost its phone in mapping: %v", rec.ID, rec.Data)
		}
	}
	if len(statuses) < 3 {
		t.Errorf("seeded leads only cover %d pipeline stages — the CRM shows no funnel", len(statuses))
	}

	// Ordering (L6 + §4.3): the seed ran on a live data model, and the URLs
	// were issued only after it — the step was staged FIRST in this manifest.
	urls := stepByID(t, m, "s-urls")
	if urls.Status != domain.ManifestStepApplied {
		t.Fatalf("issue_surface_urls status = %s (%s) — it must run after the seed", urls.Status, urls.Error)
	}
}

// R22: every step is re-runnable. A second apply does no work at all, and a
// forced retry (the halt-and-retry path) re-imports the same stable rows
// without writing a single duplicate record.
func TestSeedDemoDataIsIdempotent(t *testing.T) {
	f := newSeedFixture(realtorSeedManifest())
	ctx := context.Background()

	if _, err := f.ap.Apply(ctx, "sess-seed", ""); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	firstRecords := len(f.entities.records)
	firstBody := f.gateway.imports[0].body

	// Plain re-apply: the applied step is skipped outright.
	if _, err := f.ap.Apply(ctx, "sess-seed", ""); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if len(f.gateway.imports) != 1 {
		t.Errorf("re-apply started %d imports, want 1", len(f.gateway.imports))
	}
	if len(f.entities.records) != firstRecords {
		t.Errorf("re-apply wrote %d extra records", len(f.entities.records)-firstRecords)
	}

	// Retry path: the step is re-run (as it would be after a halt further
	// down the plan). The catalog import repeats — by design, admin matches
	// on the stable SKUs — but the entity plane must not duplicate.
	f.state.mu.Lock()
	for i := range f.state.manifest.Steps {
		if f.state.manifest.Steps[i].Op == "seed_demo_data" {
			f.state.manifest.Steps[i].Status = domain.ManifestStepAccepted
		}
	}
	f.state.mu.Unlock()

	m, err := f.ap.Apply(ctx, "sess-seed", "")
	if err != nil {
		t.Fatalf("retry apply: %v", err)
	}
	st := stepByID(t, m, "s-seed")
	if st.Status != domain.ManifestStepApplied {
		t.Fatalf("retry left the seed step %s (%s)", st.Status, st.Error)
	}
	if len(f.entities.records) != firstRecords {
		t.Errorf("retry duplicated records: %d → %d", firstRecords, len(f.entities.records))
	}
	if st.Result["records"] != 0 {
		t.Errorf("retry reported %v records written, want 0 — everything was already seeded", st.Result["records"])
	}
	if len(f.gateway.imports) != 2 {
		t.Fatalf("retry should re-import the catalog (SKU-stable), got %d imports", len(f.gateway.imports))
	}
	if f.gateway.imports[1].body != firstBody {
		t.Error("retry imported a different table — SKUs would not converge")
	}
}

// A business class with no pack must not be seeded with someone else's
// world, and must not halt the assembly either: the step applies, says so,
// and touches nothing.
func TestSeedDemoDataSkipsUnknownBusinessClass(t *testing.T) {
	m := realtorSeedManifest()
	for i := range m.Steps {
		if m.Steps[i].Op == "create_tenant" {
			m.Steps[i].Params["vertical"] = "flower shop"
		}
	}
	f := newSeedFixture(m)

	applied, err := f.ap.Apply(context.Background(), "sess-seed", "")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	st := stepByID(t, applied, "s-seed")
	if st.Status != domain.ManifestStepApplied {
		t.Fatalf("step status = %s (%s), want applied", st.Status, st.Error)
	}
	if st.Result["pack"] != "none" || st.Result["listings"] != 0 {
		t.Errorf("unknown business class seeded something: %v", st.Result)
	}
	if len(f.gateway.imports) != 0 || len(f.entities.records) != 0 {
		t.Errorf("unknown business class wrote data: %d imports, %d records",
			len(f.gateway.imports), len(f.entities.records))
	}
}

// Fail loud: a failed import means a dead storefront. The step halts with
// admin's reason and the URLs are NOT issued over an empty workspace.
func TestSeedDemoDataFailsLoudWhenTheImportFails(t *testing.T) {
	f := newSeedFixture(realtorSeedManifest())
	f.gateway.statusQueue = []ports.AdminImportStatus{
		{Status: "failed", Errors: []string{"row 3: price is not a number"}},
	}

	m, err := f.ap.Apply(context.Background(), "sess-seed", "")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	st := stepByID(t, m, "s-seed")
	if st.Status != domain.ManifestStepFailed {
		t.Fatalf("step status = %s, want failed", st.Status)
	}
	if !strings.Contains(st.Error, "price is not a number") {
		t.Errorf("step error %q does not carry admin's reason", st.Error)
	}
	if urls := stepByID(t, m, "s-urls"); urls.Status == domain.ManifestStepApplied {
		t.Error("surface URLs were issued over a workspace whose demo data failed")
	}
}

// An import that never finishes must not be reported as seeded data.
func TestSeedDemoDataFailsLoudOnImportTimeout(t *testing.T) {
	prevInterval, prevTimeout := seedImportPollInterval, seedImportPollTimeout
	seedImportPollInterval, seedImportPollTimeout = time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { seedImportPollInterval, seedImportPollTimeout = prevInterval, prevTimeout })

	f := newSeedFixture(realtorSeedManifest())
	f.gateway.statusQueue = []ports.AdminImportStatus{{Status: "processing"}}

	m, err := f.ap.Apply(context.Background(), "sess-seed", "")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	st := stepByID(t, m, "s-seed")
	if st.Status != domain.ManifestStepFailed {
		t.Fatalf("step status = %s, want failed", st.Status)
	}
	if !strings.Contains(st.Error, "still processing") {
		t.Errorf("step error %q should say the import never finished", st.Error)
	}
}

// Real data beats demo data. The business already imported its own catalog
// (the ingest door completed), so the pack must NOT be poured on top of it:
// there is no purge, admin's ingest only adds, and nothing in v5 reads the
// demo flag — fake stock would stay in the storefront, the search and the
// CRM forever.
func TestSeedDemoDataSkippedWhenRealDataAlreadyArrived(t *testing.T) {
	m := realtorSeedManifestWithDoor()
	door := stepByID(t, m, "s-door")
	door.Status = domain.ManifestStepApplied
	door.Result = map[string]any{"token": "used-1", "processed": 240, "projectionRows": 240}
	f := newSeedFixture(m)

	applied, err := f.ap.Apply(context.Background(), "sess-seed", "")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	st := stepByID(t, applied, "s-seed")
	if st.Status != domain.ManifestStepSkipped {
		t.Fatalf("seed step status = %s (%s), want skipped", st.Status, st.Error)
	}
	if len(f.gateway.imports) != 0 || len(f.entities.records) != 0 {
		t.Fatalf("demo data was seeded over real data: %d imports, %d records",
			len(f.gateway.imports), len(f.entities.records))
	}
	notes, _ := st.Result["notes"].([]string)
	if len(notes) != 1 || !strings.Contains(notes[0], "240") {
		t.Errorf("result notes = %v, want an honest reason naming the real rows", st.Result["notes"])
	}
	// A skipped seed is DONE, not pending: the visitor still gets the URLs.
	if urls := stepByID(t, applied, "s-urls"); urls.Status != domain.ManifestStepApplied {
		t.Errorf("issue_surface_urls = %s — a skipped seed must not gate the handover", urls.Status)
	}
}

// The upload-first path (owner 2026-07-28: the upload IS the approval).
// ResolveIngestToken auto-applies the whole staged manifest, so without a
// guard the visitor's own CSV would trigger a demo catalog into their tenant
// seconds before their real rows land.
func TestUploadFirstNeverSeedsDemoData(t *testing.T) {
	f := newSeedFixture(realtorSeedManifestWithDoor())
	ctx := context.Background()

	tok, err := f.ap.ResolveIngestToken(ctx, "sess-seed")
	if err != nil {
		t.Fatalf("ResolveIngestToken: %v", err)
	}
	if tok.Token == "" {
		t.Fatal("no ingest token minted — the upload has no door")
	}
	if len(f.gateway.imports) != 0 {
		t.Fatalf("the auto-apply imported %d demo tables into a tenant that is uploading its own data",
			len(f.gateway.imports))
	}
	if len(f.entities.records) != 0 {
		t.Errorf("the auto-apply wrote %d demo records", len(f.entities.records))
	}

	m, _ := f.state.GetOnboarding(ctx, "sess-seed")
	st := stepByID(t, m, "s-seed")
	if st.Status != domain.ManifestStepSkipped {
		t.Fatalf("seed step = %s, want skipped (persisted before the auto-apply reloads the manifest)", st.Status)
	}
	if notes, _ := st.Result["notes"].([]any); len(notes) == 0 {
		t.Errorf("skipped seed step carries no reason: %v", st.Result)
	}

	// The real import lands afterwards: still no demo data, and re-applying
	// does not resurrect it.
	if err := f.ap.RecordUploadJob(ctx, "sess-seed", "job-real", "listings.csv"); err != nil {
		t.Fatalf("record upload job: %v", err)
	}
	if _, err := f.ap.Apply(ctx, "sess-seed", ""); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if len(f.gateway.imports) != 0 || len(f.entities.records) != 0 {
		t.Errorf("a later apply seeded demo data anyway: %d imports, %d records",
			len(f.gateway.imports), len(f.entities.records))
	}
}

// A seed step that has not run yet is stood down the moment admin accepts a
// real upload for the session — the door mutation is where v5 learns it.
func TestAcceptedUploadStandsDownAPendingSeedStep(t *testing.T) {
	m := realtorSeedManifestWithDoor()
	for i := range m.Steps {
		m.Steps[i].Status = domain.ManifestStepAccepted
	}
	m.Tenant = domain.ManifestTenant{ID: "t-1", Slug: "blue-harbor-realty", Vertical: "real estate agency"}
	f := newSeedFixture(m)
	ctx := context.Background()

	if err := f.ap.RecordUploadJob(ctx, "sess-seed", "job-real", "listings.csv"); err != nil {
		t.Fatalf("record upload job: %v", err)
	}
	stored, _ := f.state.GetOnboarding(ctx, "sess-seed")
	if st := stepByID(t, stored, "s-seed"); st.Status != domain.ManifestStepSkipped {
		t.Fatalf("seed step = %s after a real upload was accepted, want skipped", st.Status)
	}
}

// The 0-row guard: admin can answer "completed" having written nothing (a
// mapping that matched no column, a projection rebuild that wrote nothing).
// Reporting that as seeded data hands the owner an empty storefront to demo.
func TestSeedDemoDataFailsLoudOnZeroProcessedImport(t *testing.T) {
	f := newSeedFixture(realtorSeedManifest())
	f.gateway.statusQueue = []ports.AdminImportStatus{
		{Status: "completed", Processed: 0, ProjectionRows: 0},
	}

	m, err := f.ap.Apply(context.Background(), "sess-seed", "")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	st := stepByID(t, m, "s-seed")
	if st.Status != domain.ManifestStepFailed {
		t.Fatalf("step status = %s, want failed on an import that landed nothing", st.Status)
	}
	if !strings.Contains(st.Error, "0 of 17 rows") {
		t.Errorf("step error %q does not say how many rows were lost", st.Error)
	}
	if len(f.entities.records) != 0 {
		t.Errorf("records were seeded although the catalog import landed nothing")
	}
	if urls := stepByID(t, m, "s-urls"); urls.Status == domain.ManifestStepApplied {
		t.Error("surface URLs were issued over an empty storefront")
	}
}

// R7's contract is the WHOLE status, not just the processed count: a partial
// import with row errors and an empty projection is the difference between a
// storefront the owner can demo and one that renders two thirds of nothing.
func TestSeedDemoDataRecordsTheHonestImportStatus(t *testing.T) {
	f := newSeedFixture(realtorSeedManifest())
	f.gateway.statusQueue = []ports.AdminImportStatus{
		{Status: "completed", Processed: 11, ProjectionRows: 0, Invalidated: false,
			Errors: []string{"row 4: image_url unreachable"}},
	}

	m, err := f.ap.Apply(context.Background(), "sess-seed", "")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	st := stepByID(t, m, "s-seed")
	if st.Status != domain.ManifestStepApplied {
		t.Fatalf("step status = %s (%s)", st.Status, st.Error)
	}
	if st.Result["listings"] != 11 || st.Result["projectionRows"] != 0 {
		t.Errorf("step result hides the import outcome: %v", st.Result)
	}
	if _, ok := st.Result["errors"]; !ok {
		t.Errorf("admin's row errors were dropped: %v", st.Result)
	}
	notes, _ := st.Result["notes"].([]string)
	joined := strings.Join(notes, " | ")
	if !strings.Contains(joined, "11 of the pack's 17") || !strings.Contains(joined, "projection") {
		t.Errorf("notes = %q, want the shortfall AND the empty projection said out loud", joined)
	}
}

// The tenant models leads but no contacts: the leads still seed, and the
// step says out loud what it could not do (no silent half-success).
func TestSeedDemoDataNotesUnmodelledPlanes(t *testing.T) {
	m := realtorSeedManifest()
	kept := m.Steps[:0]
	for _, s := range m.Steps {
		if s.ID == "s-contact" {
			continue
		}
		kept = append(kept, s)
	}
	m.Steps = kept
	f := newSeedFixture(m)

	applied, err := f.ap.Apply(context.Background(), "sess-seed", "")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	st := stepByID(t, applied, "s-seed")
	if st.Status != domain.ManifestStepApplied {
		t.Fatalf("step status = %s (%s)", st.Status, st.Error)
	}
	if len(f.entities.recordsOf("lead")) == 0 {
		t.Error("leads were not seeded although the tenant models them")
	}
	notes, _ := st.Result["notes"].([]string)
	if len(notes) != 1 || !strings.Contains(notes[0], "contact") {
		t.Errorf("result notes = %v, want an honest note about the missing contact entity", st.Result["notes"])
	}
}
