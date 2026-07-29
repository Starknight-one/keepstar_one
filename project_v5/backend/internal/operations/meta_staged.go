package operations

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// The 9 STAGED meta-operations (RUNTIME_SPEC.md §4.3): create_tenant,
// define_entity, define_value_set, define_automation, enable_operations,
// adopt_presets, issue_ingest_door, register_user, issue_surface_urls.
//
// Execute appends (or re-proposes) a ManifestStep{status: proposed} in the
// onboarding zone and returns "staged: <op> <summary>" — nothing mutates
// the world at stage time; ids resolve at APPLY time (R4). One generic
// executor parameterised per op: the differences are the natural key
// (re-staging the same thing REPLACES the step instead of duplicating it),
// the summary line, an optional manifest hook (create_tenant proposes the
// tenant identity) and the R6 credential scrub (register_user).

// stagedOpConfig is the per-op parameterisation of StagedMetaExecutor.
type stagedOpConfig struct {
	name string
	// stepKey extracts the step's natural key from its params. Steps with
	// equal (op, key) replace each other on re-stage; nil = one step per op
	// (singleton).
	stepKey func(params map[string]any) string
	// summarize renders the LLM-facing tail of "staged: <op> <summary>".
	summarize func(params map[string]any) string
	// onStage optionally mutates the manifest beyond the step append.
	onStage func(m *domain.OnboardingManifest, params map[string]any)
	// sanitize optionally REPAIRS the staged params before anything is
	// written. It returns (note, problem): a non-empty problem rejects the
	// staging outright and nothing is persisted; a non-empty note rides on
	// the OK summary, so a repair is never silent — the model reads it on
	// its next turn and can re-stage. Staging is the cheap place to catch a
	// bad argument (V2_SPEC L11), but it is NOT a place to dead-end the
	// flow: Agent1 has no in-turn re-prompt, so "reject everything" means
	// the plan simply loses that step.
	sanitize func(deps MetaExecutorDeps, params map[string]any) (note, problem string)
	// scrub lists param keys deleted before the step persists — R6
	// belt-and-braces on top of the registry dropping undeclared input keys
	// (an LLM-staged step must never smuggle credentials into state).
	scrub []string
}

// StagedMetaExecutor stages one manifest op. Construct via
// NewStagedMetaExecutors.
type StagedMetaExecutor struct {
	deps MetaExecutorDeps
	tmpl domain.OperationTemplate
	cfg  stagedOpConfig
}

var _ ports.Executor = (*StagedMetaExecutor)(nil)

// NewStagedMetaExecutors builds the 9 staged executors in §4.3 order.
func NewStagedMetaExecutors(deps MetaExecutorDeps) []*StagedMetaExecutor {
	configs := []stagedOpConfig{
		{
			name: "create_tenant",
			summarize: func(p map[string]any) string {
				return fmt.Sprintf("%q (%s)", stringInput(p, "name"), stringInput(p, "vertical"))
			},
			onStage: func(m *domain.OnboardingManifest, p map[string]any) {
				// Propose the tenant identity (domain contract: name/vertical
				// at stage time; id/slug filled by the applier). Never touch
				// an already-provisioned identity.
				if m.Tenant.ID == "" {
					m.Tenant.Name = stringInput(p, "name")
					m.Tenant.Vertical = stringInput(p, "vertical")
				}
			},
		},
		{
			name:    "define_entity",
			stepKey: func(p map[string]any) string { return stringInput(entityParam(p), "slug") },
			summarize: func(p map[string]any) string {
				entity := entityParam(p)
				fields, _ := entity["fields"].([]any)
				return fmt.Sprintf("entity %q with %d fields", stringInput(entity, "slug"), len(fields))
			},
		},
		{
			name:    "define_value_set",
			stepKey: func(p map[string]any) string { return stringInput(p, "slug") },
			summarize: func(p map[string]any) string {
				values, _ := p["values"].([]any)
				return fmt.Sprintf("value set %q with %d values", stringInput(p, "slug"), len(values))
			},
		},
		{
			name:    "define_automation",
			stepKey: func(p map[string]any) string { return stringInput(p, "name") },
			summarize: func(p map[string]any) string {
				return fmt.Sprintf("automation %q on %s", stringInput(p, "name"), stringInput(p, "event_type"))
			},
		},
		{
			name:    "enable_operations",
			stepKey: func(p map[string]any) string { return strings.Join(operationInstanceNames(p), ",") },
			summarize: func(p map[string]any) string {
				names := operationInstanceNames(p)
				return fmt.Sprintf("%d operations: %s", len(names), strings.Join(names, ", "))
			},
		},
		{
			name:     "adopt_presets",
			stepKey:  func(p map[string]any) string { return strings.Join(sortedStrings(cfgStringSlice(p, "presets")), ",") },
			sanitize: sanitizeAdoptPresets,
			summarize: func(p map[string]any) string {
				names := cfgStringSlice(p, "presets")
				return fmt.Sprintf("%d presets: %s", len(names), strings.Join(names, ", "))
			},
		},
		{
			name: "issue_ingest_door",
			summarize: func(p map[string]any) string {
				return "accepting " + strings.Join(cfgStringSlice(p, "formats"), ", ") + " uploads"
			},
		},
		{
			name:  "register_user",
			scrub: []string{"email", "password", "name"}, // R6 belt-and-braces
			summarize: func(p map[string]any) string {
				role := stringInput(p, "role")
				if role == "" {
					role = "owner"
				}
				return role + " account — completes via the secure registration form, never through chat"
			},
		},
		{
			name: "issue_surface_urls",
			summarize: func(map[string]any) string {
				return "storefront + CRM URLs — issued after every other step applies"
			},
		},
	}
	out := make([]*StagedMetaExecutor, len(configs))
	for i, cfg := range configs {
		out[i] = &StagedMetaExecutor{deps: deps, tmpl: seedRow(cfg.name), cfg: cfg}
	}
	return out
}

func (e *StagedMetaExecutor) Template() domain.OperationSpec        { return e.tmpl.Spec() }
func (e *StagedMetaExecutor) TemplateRow() domain.OperationTemplate { return e.tmpl }

// SpecForTenant: staged meta-ops are tenant-independent — the onboarding
// session runs under the system tenant, and the schemas are static.
func (e *StagedMetaExecutor) SpecForTenant(context.Context, domain.Tenant, map[string]any) (domain.OperationSpec, error) {
	return e.tmpl.Spec(), nil
}

// Execute stages the step. Input arrives validated+coerced by the registry
// choke point (undeclared keys already dropped).
func (e *StagedMetaExecutor) Execute(ctx context.Context, octx domain.OperationContext, input map[string]any) (*domain.OperationResult, error) {
	if octx.SessionID == "" {
		return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeError, "error: staging requires a session"), nil
	}
	m, err := loadOrNewManifest(ctx, e.deps, octx.SessionID)
	if err != nil {
		return nil, err
	}

	params := cloneParams(input)
	for _, k := range e.cfg.scrub {
		delete(params, k)
	}

	var note string
	if e.cfg.sanitize != nil {
		n, problem := e.cfg.sanitize(e.deps, params)
		if problem != "" {
			// Nothing is written: the manifest must never carry a step the
			// apply cannot honour.
			return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeInvalid, "invalid: "+problem), nil
		}
		note = n
	}

	// matchStep keys on the SANITIZED params, so a re-stage of the same
	// repaired set still replaces its predecessor instead of duplicating it.
	st := e.matchStep(m, params)
	if st != nil && st.Status == domain.ManifestStepApplied {
		// Applied steps are immutable — the world already reflects them.
		return &domain.OperationResult{
			Kind:    e.tmpl.Kind,
			Outcome: domain.OutcomeOK,
			Summary: fmt.Sprintf("ok: %s already applied — step %s not restaged", e.tmpl.Name, st.ID),
			Output:  map[string]any{"staged": false, "stepId": st.ID},
		}, nil
	}
	if st != nil {
		// Re-stage: replace the proposal in place, keep the id (rendered
		// widgets reference it), clear any previous failure.
		st.Params = params
		st.Status = domain.ManifestStepProposed
		st.Result = nil
		st.Error = ""
	} else {
		m.Steps = append(m.Steps, domain.ManifestStep{
			ID:     nextStepID(m, e.tmpl.Name),
			Op:     e.tmpl.Name,
			Params: params,
			Status: domain.ManifestStepProposed,
		})
		st = &m.Steps[len(m.Steps)-1]
	}
	if e.cfg.onStage != nil {
		e.cfg.onStage(m, params)
	}

	if err := persistOnboarding(ctx, e.deps, octx, m, e.tmpl.Name, map[string]any{
		"stepId": st.ID, "op": e.tmpl.Name, "status": st.Status,
	}); err != nil {
		return nil, err
	}

	summary := fmt.Sprintf("staged: %s %s", e.tmpl.Name, e.cfg.summarize(params))
	if note != "" {
		summary += " — " + note
	}
	return &domain.OperationResult{
		Kind:     e.tmpl.Kind,
		Outcome:  domain.OutcomeOK,
		Count:    1,
		Summary:  summary,
		Output:   map[string]any{"staged": true, "stepId": st.ID},
		Metadata: map[string]any{"stepId": st.ID, "op": e.tmpl.Name},
	}, nil
}

// matchStep finds the step this staging replaces: same op AND same natural
// key (nil stepKey = singleton per op). Returns nil when the staging is new.
func (e *StagedMetaExecutor) matchStep(m *domain.OnboardingManifest, params map[string]any) *domain.ManifestStep {
	var key string
	if e.cfg.stepKey != nil {
		key = e.cfg.stepKey(params)
	}
	for i := range m.Steps {
		st := &m.Steps[i]
		if st.Op != e.tmpl.Name {
			continue
		}
		if e.cfg.stepKey == nil || e.cfg.stepKey(st.Params) == key {
			return st
		}
	}
	return nil
}

// ─── stage-time sanitizers ───────────────────────────────────────────────

// sanitizeAdoptPresets resolves every requested name against the system
// preset library BEFORE the step is staged, DROPS the names that do not
// exist and stages the remainder. The model invents names (live smoke:
// "lead_cards").
//
// Why strip rather than reject: there is no in-turn re-prompt. Agent1 calls
// the model once (pipeline_execute.go), runs the emitted tool batch and
// returns — a tool result reaches the model only on a LATER user turn, and
// the auto-apply triggers (registration submit, file upload) fire before
// that. An all-or-nothing rejection therefore does not buy a corrected
// re-stage; it leaves the manifest with NO adopt_presets step at all and the
// tenant applies with nothing adopted. Stripping keeps every real preset,
// and the dropped names ride on the OK summary so the miss is visible to the
// model (and in the log) instead of silent.
//
// Nothing valid left IS rejected: an empty presets[] is a step the apply
// cannot honour.
//
// This is the same law the applier already encodes one file over
// (manifest_apply.go applyAdoptPresets: "never halt the whole environment
// over a cosmetic miss"), moved one stage earlier — and the apply-time skip
// stays as belt-and-braces for a library that drifts between stage and
// apply.
func sanitizeAdoptPresets(deps MetaExecutorDeps, params map[string]any) (note, problem string) {
	names := cfgStringSlice(params, "presets")
	if len(names) == 0 {
		return "", "adopt_presets needs presets[] — at least one preset name"
	}
	if deps.PresetLibrary == nil {
		return "", "" // not wired: apply-time skip-unknown is the only guard
	}
	library := deps.PresetLibrary()
	if len(library) == 0 {
		// An empty library means the seeds could not be read, not that every
		// name is wrong — dropping them all would dead-end the flow.
		deps.logger().Warn("adopt_presets: preset library is empty — stage-time validation skipped")
		return "", ""
	}
	known := make(map[string]bool, len(library))
	for _, n := range library {
		known[n] = true
	}
	valid := make([]any, 0, len(names))
	var unknown []string
	for _, n := range names {
		if known[n] {
			valid = append(valid, n)
			continue
		}
		unknown = append(unknown, n)
	}
	if len(unknown) == 0 {
		return "", ""
	}
	if len(valid) == 0 {
		return "", fmt.Sprintf("no such preset %s: %s — the library has: %s. Re-stage adopt_presets using only these names",
			pluralPreset(len(unknown)), strings.Join(unknown, ", "), strings.Join(library, ", "))
	}
	// []any, not []string: params go to JSONB and come back as []any, and the
	// step's natural key must read the same before and after a round-trip.
	params["presets"] = valid
	deps.logger().Warn("adopt_presets: unknown preset names dropped at stage time",
		"dropped", unknown, "staged", valid)
	return fmt.Sprintf("dropped unknown %s %s (the library has: %s) — re-stage adopt_presets if you meant something in it",
		pluralPreset(len(unknown)), strings.Join(unknown, ", "), strings.Join(library, ", ")), ""
}

func pluralPreset(n int) string {
	if n == 1 {
		return "name"
	}
	return "names"
}

// ─── param readers ───────────────────────────────────────────────────────

func entityParam(p map[string]any) map[string]any {
	entity, _ := p["entity"].(map[string]any)
	return entity
}

// operationInstanceNames lists the instance wire names of an
// enable_operations params payload, sorted (stable natural key + summary).
func operationInstanceNames(p map[string]any) []string {
	items, _ := p["operations"].([]any)
	names := make([]string, 0, len(items))
	for _, raw := range items {
		if item, ok := raw.(map[string]any); ok {
			if name := stringInput(item, "instance"); name != "" {
				names = append(names, name)
			}
		}
	}
	return sortedStrings(names)
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
