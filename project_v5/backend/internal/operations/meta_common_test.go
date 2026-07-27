package operations

import (
	"context"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// ─── meta-executor fakes ─────────────────────────────────────────────────

// fakeOnboardingState is an in-memory ports.OnboardingStatePort.
type fakeOnboardingState struct {
	manifest *domain.OnboardingManifest
	infos    []domain.DeltaInfo
	getErr   error
}

var _ ports.OnboardingStatePort = (*fakeOnboardingState)(nil)

func (f *fakeOnboardingState) GetOnboarding(context.Context, string) (*domain.OnboardingManifest, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.manifest == nil {
		return nil, nil
	}
	cp := *f.manifest
	return &cp, nil
}

func (f *fakeOnboardingState) UpdateOnboarding(_ context.Context, _ string, m *domain.OnboardingManifest, info domain.DeltaInfo) (int, error) {
	f.manifest = m
	f.infos = append(f.infos, info)
	return len(f.infos), nil
}

// fakeLibraryStore extends the registry fakeStore with canned library hits
// and call capture.
type fakeLibraryStore struct {
	fakeStore
	hits      []domain.OperationTemplate
	lastQuery string
	lastKinds []string
	lastK     int
	gotEmb    []float32
}

func (s *fakeLibraryStore) SearchLibrary(_ context.Context, embedding []float32, query string, kinds []string, k int) ([]domain.OperationTemplate, error) {
	s.gotEmb, s.lastQuery, s.lastKinds, s.lastK = embedding, query, kinds, k
	return s.hits, nil
}

// fakeApplier is a canned operations.ManifestApplier.
type fakeApplier struct {
	manifest *domain.OnboardingManifest
	err      error
	lastUpTo string
	calls    int
}

func (f *fakeApplier) Apply(_ context.Context, _ string, upTo string) (*domain.OnboardingManifest, error) {
	f.calls++
	f.lastUpTo = upTo
	if f.err != nil {
		return nil, f.err
	}
	return f.manifest, nil
}

func metaDeps(ob *fakeOnboardingState, state *opsStatePort) MetaExecutorDeps {
	return MetaExecutorDeps{
		Onboarding: ob,
		State:      state,
		Store:      &fakeLibraryStore{},
		Log:        testLogger(),
	}
}

func onboardingOctx() domain.OperationContext {
	return domain.OperationContext{
		SessionID:  "sess-1",
		TenantSlug: "keepstar-onboarding",
		TurnID:     "turn-1",
		Mode:       domain.ModeOnboarding,
		Role:       domain.RoleVisitor,
	}
}

// stagedExecutor pulls one of the nine staged executors by wire name.
func stagedExecutor(deps MetaExecutorDeps, name string) *StagedMetaExecutor {
	for _, ex := range NewStagedMetaExecutors(deps) {
		if ex.tmpl.Name == name {
			return ex
		}
	}
	return nil
}
