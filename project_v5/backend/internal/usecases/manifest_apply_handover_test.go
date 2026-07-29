package usecases

// The handover seam, end to end, on PRODUCTION parts only.
//
// The onboarding handover crosses TWO session zones: the applier writes the
// manifest zone (step Result carries the minted URLs), the renderer binds the
// DATA zone (the synthetic `surfaceLink` EntitySet). Nothing in
// ManifestApplier can reach the data zone on its own — OnboardingStatePort is
// GetOnboarding/UpdateOnboarding — so an apply that runs outside the
// apply_manifest executor mints URLs nobody can render. That is exactly what
// EnsureZeroInputStep does, and the bug it shipped with: the card went on the
// wire with zero rows.
//
// These tests therefore refuse every stand-in for the seam: the real
// *ManifestApplier, the real operations.NewSyntheticSetPublisher, the real
// tools.ComposeTurnTool, the real embedded surface_links seed. Break the
// wiring and they fail (TestHandoverWithoutThePublisherShipsEmpty pins that
// the publisher is what makes the difference, not some other path).

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine/presets"
	"keepstar_v5/internal/operations"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/tools"
)

const handoverSession = "sess-handover"

// handoverStore is one session carrying BOTH zones the handover crosses:
// applierStatePort is the manifest zone, mockStatePort the data zone the
// renderer reads. The production seam runs between them; a fake that owns
// only one of them cannot observe it (which is why the earlier gate test
// could not).
type handoverStore struct {
	*applierStatePort
	*mockStatePort
}

var (
	_ ports.OnboardingStatePort = (*handoverStore)(nil)
	_ ports.StatePort           = (*handoverStore)(nil)
)

// handoverFixture is the applier wired the way cmd/server wires it.
type handoverFixture struct {
	store  *handoverStore
	tokens *applierTokens
	ap     *ManifestApplier
}

// newHandoverFixture builds a session whose plan is fully applied EXCEPT the
// surface-URL step, which was never staged — the live-smoke shape: the model
// composed the handover without ever calling apply_manifest.
//
// publish=false drops the synthetic-set seam and nothing else, so the two
// tests differ by exactly the wiring under scrutiny.
func newHandoverFixture(t *testing.T, publish bool) *handoverFixture {
	t.Helper()
	ctx := context.Background()

	plan := realtorManifest()
	steps := plan.Steps[:0]
	for _, s := range plan.Steps {
		if s.Op != "issue_surface_urls" {
			steps = append(steps, s)
		}
	}
	plan.Steps = steps

	f := &handoverFixture{
		store: &handoverStore{
			applierStatePort: &applierStatePort{manifest: plan},
			mockStatePort:    newMockStatePort(),
		},
		tokens: &applierTokens{},
	}
	if _, err := f.store.CreateState(ctx, handoverSession); err != nil {
		t.Fatalf("create session state: %v", err)
	}
	cfg := ManifestApplierConfig{
		State:          f.store,
		Tokens:         f.tokens,
		Gateway:        &applierGateway{},
		Entities:       &applierEntities{},
		Ops:            &applierOps{},
		Themes:         &applierThemes{},
		Registry:       &applierRegistry{},
		SurfaceBaseURL: "https://v5.example.test",
		Log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if publish {
		cfg.PublishSyntheticSets = operations.NewSyntheticSetPublisher(f.store,
			slog.New(slog.NewTextHandler(io.Discard, nil)))
	}
	f.ap = NewManifestApplier(cfg)

	// Drive the plan to the state the handover starts from: applied, with the
	// two trigger steps completed out of band (form submit + upload).
	if _, err := f.ap.Apply(ctx, handoverSession, ""); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := f.ap.ExecuteStep(ctx, handoverSession, "s-reg", map[string]any{
		"email": "owner@acme.test", "password": "pw",
	}); err != nil {
		t.Fatalf("ExecuteStep: %v", err)
	}
	if err := f.ap.CompleteIngestStep(ctx, handoverSession, &ports.AdminImportStatus{
		Status: "completed", Processed: 10, ProjectionRows: 10, Invalidated: true,
	}); err != nil {
		t.Fatalf("CompleteIngestStep: %v", err)
	}
	if idx := findStepIndexByOp(f.store.manifest, "issue_surface_urls"); idx >= 0 {
		t.Fatalf("precondition: issue_surface_urls must NOT be staged yet")
	}
	return f
}

// issuedURLs reads the addresses the applier actually minted, from the
// manifest zone. Asserting the render against THESE (not constants) is what
// makes the test a contract: the rows must carry what the apply produced.
func (f *handoverFixture) issuedURLs(t *testing.T) (storefront, crm string) {
	t.Helper()
	idx := findStepIndexByOp(f.store.manifest, "issue_surface_urls")
	if idx < 0 {
		t.Fatal("issue_surface_urls was never staged")
	}
	st := f.store.manifest.Steps[idx]
	if st.Status != domain.ManifestStepApplied {
		t.Fatalf("issue_surface_urls status = %q, want applied (err %q)", st.Status, st.Error)
	}
	storefront, _ = st.Result["storefrontUrl"].(string)
	crm, _ = st.Result["crmUrl"].(string)
	if storefront == "" || crm == "" {
		t.Fatalf("apply produced no URLs: %v", st.Result)
	}
	return storefront, crm
}

// dataZoneSet returns the synthetic EntitySet the renderer would bind, or nil.
func (f *handoverFixture) dataZoneSet(t *testing.T, slug string) *domain.EntitySet {
	t.Helper()
	st, err := f.store.GetState(context.Background(), handoverSession)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	for i := range st.Current.Data.Entities {
		if st.Current.Data.Entities[i].Slug == slug {
			return &st.Current.Data.Entities[i]
		}
	}
	return nil
}

// TestEnsureZeroInputStepPublishesTheRenderableSet — the seam itself.
// EnsureZeroInputStep applies issue_surface_urls; the URLs it mints must land
// in the DATA zone, because that is the only zone compose_turn binds. Before
// the publisher was injected this assertion failed while every other applier
// test stayed green: the step said `applied`, the card said nothing.
func TestEnsureZeroInputStepPublishesTheRenderableSet(t *testing.T) {
	f := newHandoverFixture(t, true)

	changed, err := f.ap.EnsureZeroInputStep(context.Background(), handoverSession, "issue_surface_urls")
	if err != nil {
		t.Fatalf("EnsureZeroInputStep: %v", err)
	}
	if !changed {
		t.Fatal("changed = false — the caller would bind the pre-apply snapshot")
	}
	storefront, crm := f.issuedURLs(t)

	set := f.dataZoneSet(t, "surfaceLink")
	if set == nil {
		t.Fatal("no surfaceLink EntitySet in the data zone — the applied step's URLs " +
			"live only inside the manifest, so the handover card renders zero rows")
	}
	if !set.Synthetic {
		t.Error("surfaceLink set is not marked synthetic")
	}
	if len(set.Records) != 2 {
		t.Fatalf("surfaceLink records = %d, want 2 (storefront + CRM)", len(set.Records))
	}
	if got, _ := set.Records[0].Data["url"].(string); got != storefront {
		t.Errorf("record 0 url = %q, want the minted storefront %q", got, storefront)
	}
	if got, _ := set.Records[1].Data["url"].(string); got != crm {
		t.Errorf("record 1 url = %q, want the minted CRM address %q", got, crm)
	}
	// The manifest_summary set rides along on the same write — the digest
	// preset renders in this very turn.
	if steps := f.dataZoneSet(t, "manifestStep"); steps == nil {
		t.Error("no manifestStep EntitySet — the manifest_summary preset would render empty")
	}
}

// TestComposeTurnHandoverShipsTheIssuedURLs — the whole tail on production
// parts: compose_turn (real tool) → gate (real *ManifestApplier) → apply →
// publisher (real) → data zone → re-read → assembly against the real embedded
// surface_links seed. The model composed the handover WITHOUT staging
// issue_surface_urls and WITHOUT calling apply_manifest; the block must still
// reach the user carrying the addresses the apply minted.
func TestComposeTurnHandoverShipsTheIssuedURLs(t *testing.T) {
	f := newHandoverFixture(t, true)

	tool := tools.NewComposeTurnTool(f.store, handoverPresets{}, handoverComponents{})
	tool.SetManifestGate(f.ap)
	sink := domain.NewTurnBlockCollector(nil)
	ctx := domain.WithTurnBlockCollector(context.Background(), sink)

	res, err := tool.Execute(ctx,
		domain.ToolContext{SessionID: handoverSession, TenantSlug: "acme-realty"},
		map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"kind": "text", "text": "Everything is assembled."},
			map[string]interface{}{"kind": "render", "preset": "surface_links", "replicate": "surfaceLink"},
		}})
	if err != nil {
		t.Fatalf("compose_turn: %v", err)
	}
	if res.IsError {
		t.Fatalf("compose_turn IsError: %s", res.Content)
	}
	storefront, crm := f.issuedURLs(t)

	blocks := sink.Blocks()
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	urls := boundContents(blocks[1].Document, "url")
	if len(urls) != 2 || urls[0] != storefront || urls[1] != crm {
		t.Fatalf("rendered urls = %v, want [%s %s] — the handover reached the user "+
			"without the addresses the apply produced", urls, storefront, crm)
	}
	if labels := boundContents(blocks[1].Document, "label"); len(labels) != 2 {
		t.Errorf("rendered labels = %v, want two rows", labels)
	}
}

// TestHandoverWithoutThePublisherShipsEmpty pins the defect the injected
// publisher exists to prevent, so the test above cannot pass for some other
// reason. Identical fixture minus PublishSyntheticSets: the step still applies
// and the URLs are still minted into the manifest — and the card still ships
// with zero rows, because the manifest zone is not the zone the renderer
// binds. If this ever starts rendering URLs, the applier grew a second path
// into the data zone and the wiring above needs re-reading, not deleting.
func TestHandoverWithoutThePublisherShipsEmpty(t *testing.T) {
	f := newHandoverFixture(t, false)

	tool := tools.NewComposeTurnTool(f.store, handoverPresets{}, handoverComponents{})
	tool.SetManifestGate(f.ap)
	sink := domain.NewTurnBlockCollector(nil)
	ctx := domain.WithTurnBlockCollector(context.Background(), sink)

	if _, err := tool.Execute(ctx,
		domain.ToolContext{SessionID: handoverSession, TenantSlug: "acme-realty"},
		map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"kind": "render", "preset": "surface_links", "replicate": "surfaceLink"},
		}}); err != nil {
		t.Fatalf("compose_turn: %v", err)
	}
	// The apply DID run and DID mint — the gap is purely the zone.
	if storefront, _ := f.issuedURLs(t); !strings.HasPrefix(storefront, "https://v5.example.test/s/") {
		t.Fatalf("precondition: the step must still apply, got %q", storefront)
	}
	if set := f.dataZoneSet(t, "surfaceLink"); set != nil {
		t.Fatalf("data zone carries surfaceLink without the publisher wired (%d records) — "+
			"the applier found another way in; re-check the seam", len(set.Records))
	}
	if urls := boundContents(sink.Blocks()[0].Document, "url"); len(urls) != 0 {
		t.Fatalf("urls = %v — this test documents the empty-card defect; if the card "+
			"now renders, the production seam moved", urls)
	}
}

// --- production-parts test doubles (ports only; no logic) ---

// handoverPresets serves the REAL embedded seeds — the document the tenant
// actually renders. A hand-written stand-in would prove nothing about the
// preset the handover ships.
type handoverPresets struct{}

func (handoverPresets) GetPublishedPreset(_ context.Context, _, name string) (*domain.Preset, error) {
	raw, ok := presets.SystemPresetSeeds[name]
	if !ok {
		return nil, domain.ErrPresetNotFound
	}
	return &domain.Preset{
		ID: "pr-" + name, TenantID: "t-1", Name: name,
		Status: domain.PresetStatusPublished, DocumentJSON: raw, IsSystem: true,
	}, nil
}

func (handoverPresets) ListPublishedPresets(context.Context, string) ([]domain.Preset, error) {
	return nil, nil
}

type handoverComponents struct{}

func (handoverComponents) GetPublishedComponent(context.Context, string, string) (*domain.Component, error) {
	return nil, nil
}

func (handoverComponents) ListPublishedComponents(context.Context, string) ([]domain.Component, error) {
	return nil, nil
}

// boundContents collects the bound `content` of every node carrying the given
// fieldBinding, in document order.
func boundContents(doc map[string]interface{}, binding string) []string {
	var out []string
	var walk func(any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]interface{}:
			if fb, _ := v["fieldBinding"].(string); fb == binding {
				if c, ok := v["content"].(string); ok {
					out = append(out, c)
				}
			}
			for _, child := range v {
				walk(child)
			}
		case []interface{}:
			for _, item := range v {
				walk(item)
			}
		}
	}
	walk(doc)
	return out
}
