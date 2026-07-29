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
	// validate optionally rejects the staging BEFORE anything is written,
	// returning the LLM-facing reason (empty = valid). Staging is the cheap
	// place to fail: the model is still in the turn and can re-stage with
	// corrected params, instead of the apply chain quietly dropping work
	// hours later (V2_SPEC L11).
	validate func(deps MetaExecutorDeps, params map[string]any) string
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
			validate: validateAdoptPresets,
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

	if e.cfg.validate != nil {
		if problem := e.cfg.validate(e.deps, params); problem != "" {
			// Nothing is written: the manifest must never carry a step the
			// apply cannot honour.
			return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeInvalid, "invalid: "+problem), nil
		}
	}

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

	return &domain.OperationResult{
		Kind:     e.tmpl.Kind,
		Outcome:  domain.OutcomeOK,
		Count:    1,
		Summary:  fmt.Sprintf("staged: %s %s", e.tmpl.Name, e.cfg.summarize(params)),
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

// ─── stage-time validators ───────────────────────────────────────────────

// validateAdoptPresets resolves every requested name against the system
// preset library BEFORE the step is staged. The model invents names (live
// smoke: "lead_cards") and apply-time only skips them (f84e0a0) — the
// workspace then goes live missing the very surfaces the conversation
// promised, with nobody left in the loop to notice. Failing here hands the
// model the invalid names AND the candidates while it is still composing,
// so it re-stages correctly within the turn.
//
// The apply-time skip stays as belt-and-braces: a library that drifts
// between stage and apply must still not halt the chain.
func validateAdoptPresets(deps MetaExecutorDeps, params map[string]any) string {
	names := cfgStringSlice(params, "presets")
	if len(names) == 0 {
		return "adopt_presets needs presets[] — at least one preset name"
	}
	if deps.PresetLibrary == nil {
		return "" // not wired: apply-time skip-unknown is the only guard
	}
	library := deps.PresetLibrary()
	if len(library) == 0 {
		// An empty library means the seeds could not be read, not that every
		// name is wrong — rejecting the staging would dead-end the flow.
		deps.logger().Warn("adopt_presets: preset library is empty — stage-time validation skipped")
		return ""
	}
	known := make(map[string]bool, len(library))
	for _, n := range library {
		known[n] = true
	}
	var unknown []string
	for _, n := range names {
		if !known[n] {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) == 0 {
		return ""
	}
	return fmt.Sprintf("unknown preset %s: %s — the library has: %s. Re-stage adopt_presets using only these names",
		pluralPreset(len(unknown)), strings.Join(unknown, ", "), strings.Join(library, ", "))
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
