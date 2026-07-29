package operations

import (
	"context"
	"fmt"
	"log/slog"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// Onboarding meta-operations (RUNTIME_SPEC.md §4.3, rulings R4/R6/R8/R22) —
// the operations of the onboarding SYSTEM FORM, whose job is to assemble
// other forms. All 12 implement ports.Executor and plug in via
// RegisterExecutor(KindMeta, …); their seed rows are the §3.1 meta templates
// (seed/templates.go), modes={onboarding}, min_role=visitor — the onboarding
// cookie + form gate does the work (R14).
//
// Three shapes:
//   - IMMEDIATE ops — results usable in the same turn: search_library
//     (meta_search_library.go; manifest libraryContext + synthetic opCard
//     EntitySet) and about_keepstar (meta_about.go; the canonical about-doc
//     content returned in the result — owner ruling 2026-07-28).
//   - 10 staged ops (meta_staged.go) — Execute appends a
//     ManifestStep{status: proposed} to the onboarding zone and returns
//     "staged: <op> <summary>". NOTHING mutates the world at stage time;
//     the deterministic ManifestApplier runs the steps at APPLY time,
//     resolving ids then (R4: no continuation LLM call).
//   - apply_manifest (meta_apply_manifest.go) — CONTROL: flips
//     proposed→accepted, runs the applier, returns a per-step digest.

// ManifestApplier is the consumer-side slice of usecases.ManifestApplier
// the apply_manifest control op drives. Declared here — operations cannot
// import usecases (the usecases in-package tests import operations, which
// would cycle the test binary); *usecases.ManifestApplier satisfies it.
type ManifestApplier interface {
	// Apply flips staged steps proposed→accepted (up to and including upTo
	// when given) and executes them in applier order. A step failure is
	// recorded on the returned manifest (nil error); a non-nil error is an
	// infrastructure failure.
	Apply(ctx context.Context, sessionID, upTo string) (*domain.OnboardingManifest, error)
}

// MetaExecutorDeps wires the 13 meta executors. Embedder may be nil
// (search_library degrades to FTS, mirroring catalog_search); Applier may be
// nil ONLY in partial wirings — apply_manifest then fails loud with an
// error outcome, never silently.
type MetaExecutorDeps struct {
	Onboarding ports.OnboardingStatePort
	State      ports.StatePort
	Store      ports.OperationStorePort
	Embedder   ports.EmbeddingPort
	Applier    ManifestApplier
	Log        *slog.Logger
}

func (d MetaExecutorDeps) logger() *slog.Logger {
	if d.Log != nil {
		return d.Log
	}
	return slog.Default()
}

// RegisterMetaExecutors plugs the 13 onboarding meta executors into the
// registry under their §3.1 template wire names (boot wiring, main.go —
// same pattern as RegisterEntityExecutors). Their seed rows ride the same
// SeedTemplates pass as everything else.
func RegisterMetaExecutors(reg ports.OperationRegistry, d MetaExecutorDeps) {
	reg.RegisterExecutor(domain.KindMeta, NewAboutKeepstarExecutor())
	reg.RegisterExecutor(domain.KindMeta, NewSearchLibraryExecutor(d))
	for _, ex := range NewStagedMetaExecutors(d) {
		reg.RegisterExecutor(domain.KindMeta, ex)
	}
	reg.RegisterExecutor(domain.KindMeta, NewApplyManifestExecutor(d))
}

// ─── shared manifest plumbing ────────────────────────────────────────────

// loadOrNewManifest returns the session's staged manifest, or a fresh one
// when the session has none yet (first meta-op of the conversation).
func loadOrNewManifest(ctx context.Context, deps MetaExecutorDeps, sessionID string) (*domain.OnboardingManifest, error) {
	m, err := deps.Onboarding.GetOnboarding(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get onboarding manifest: %w", err)
	}
	if m == nil {
		m = &domain.OnboardingManifest{Version: 1}
	}
	return m, nil
}

// persistOnboarding writes the manifest zone + its onboarding_step delta in
// one zone write. Delta params are identity-only ({stepId, op, status} — or
// the op + result count for non-step ops like search_library): step params
// NEVER enter the delta stream (R6 discipline, same as the applier).
func persistOnboarding(ctx context.Context, deps MetaExecutorDeps, octx domain.OperationContext, m *domain.OnboardingManifest, tool string, params map[string]any) error {
	actor := octx.ActorID
	if actor == "" {
		actor = "agent1"
	}
	info := domain.DeltaInfo{
		TurnID:    octx.TurnID,
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   actor,
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "onboarding",
		Action:    domain.Action{Type: ports.OnboardingStepAction, Tool: tool, Params: params},
		Result:    domain.ResultMeta{Count: len(m.Steps)},
	}
	if _, err := deps.Onboarding.UpdateOnboarding(ctx, octx.SessionID, m, info); err != nil {
		return fmt.Errorf("persist onboarding manifest: %w", err)
	}
	return nil
}

// nextStepID mints a readable, manifest-unique step id ("define_entity-3").
// The ordinal is the step's append position; steps are never removed, so
// ids never collide. Replaced (re-staged) steps KEEP their id — rendered
// widgets (the registration form's step-submit endpoint) reference it.
func nextStepID(m *domain.OnboardingManifest, op string) string {
	return fmt.Sprintf("%s-%d", op, len(m.Steps)+1)
}

// upsertEntitySet replaces (by slug) or appends a set in the Entities zone,
// preserving every other set and the Products/Services zones, and stamps
// the meta count — the shared shape of every synthetic-set write (R23).
func upsertEntitySet(data *domain.StateData, meta *domain.StateMeta, set domain.EntitySet) {
	replaced := false
	for i := range data.Entities {
		if data.Entities[i].Slug == set.Slug {
			data.Entities[i] = set
			replaced = true
			break
		}
	}
	if !replaced {
		data.Entities = append(data.Entities, set)
	}
	if meta.EntityCounts == nil {
		meta.EntityCounts = map[string]int{}
	}
	meta.EntityCounts[set.Slug] = len(set.Records)
}

// writeSyntheticSets loads session state, upserts the given synthetic sets
// into the Entities zone and persists with an ENTITY_QUERY data delta (R28
// Path "data" — synthetic sets are data-zone content, never
// v5_entity_records rows). One state write for all sets.
func writeSyntheticSets(ctx context.Context, deps MetaExecutorDeps, octx domain.OperationContext, tool string, sets ...domain.EntitySet) error {
	st, err := deps.State.GetState(ctx, octx.SessionID)
	if err != nil {
		return fmt.Errorf("get state: %w", err)
	}
	data := st.Current.Data
	meta := st.Current.Meta
	slugs := make([]string, 0, len(sets))
	for _, set := range sets {
		upsertEntitySet(&data, &meta, set)
		slugs = append(slugs, set.Slug)
	}
	actor := octx.ActorID
	if actor == "" {
		actor = "agent1"
	}
	count := 0
	if len(sets) > 0 {
		count = len(sets[0].Records)
	}
	info := domain.DeltaInfo{
		TurnID:    octx.TurnID,
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   actor,
		DeltaType: domain.DeltaTypeAdd,
		Path:      "data",
		Action: domain.Action{
			Type:   domain.ActionEntityQuery,
			Tool:   tool,
			Params: map[string]any{"synthetic": slugs},
		},
		Result: domain.ResultMeta{Count: count},
	}
	if _, err := deps.State.UpdateData(ctx, octx.SessionID, data, meta, info); err != nil {
		return fmt.Errorf("update synthetic sets: %w", err)
	}
	return nil
}

// metaOpTitle resolves the human title of a manifest op from its seed row
// (single source of the §3.1 bytes), falling back to the wire name.
func metaOpTitle(op string) string {
	if tmpl, ok := seedTemplateByName(op); ok {
		return tmpl.Title
	}
	return op
}

// cloneParams shallow-copies the registry-cleaned input before it is stored
// on a manifest step, so later mutation of the tool-call map can never
// reach persisted state.
func cloneParams(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
