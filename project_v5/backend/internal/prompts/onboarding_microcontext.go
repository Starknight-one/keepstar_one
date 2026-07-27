package prompts

import (
	"strconv"
	"strings"

	"keepstar_v5/internal/domain"
)

// ComposeOnboardingMicrocontextFromResults is the results-aware composer
// the pipeline mode dispatch should call (drop-in for
// ComposeOnboardingMicrocontext(m, opNames(results))): it derives the turn
// ops from the structured results AND — the owner-approved exception to
// state-mediation (2026-07-28) — appends the about_keepstar content in an
// <about_keepstar> envelope when the turn loaded it, so Agent2 answers the
// visitor's "what is this?" in its own words, in the visitor's language.
func ComposeOnboardingMicrocontextFromResults(m *domain.OnboardingManifest, results []*domain.OperationResult) string {
	ops := make([]string, 0, len(results))
	about := ""
	for _, r := range results {
		if r == nil {
			continue
		}
		ops = append(ops, r.Operation)
		if r.Operation == "about_keepstar" && r.Outcome == domain.OutcomeOK {
			if s, ok := r.Output["content"].(string); ok && s != "" {
				about = s
			} else if r.Summary != "" {
				about = r.Summary
			}
		}
	}
	line := ComposeOnboardingMicrocontext(m, ops)
	if about == "" {
		return line
	}
	return line + "\n<about_keepstar>\n" + about + "\n</about_keepstar>"
}

// ComposeOnboardingMicrocontext folds the onboarding manifest into the
// one-line signal Agent2 keys its choreography off (RUNTIME_SPEC.md §4.3):
//
//	"staged: create_tenant, define_entity(lead) | applied: 3/9 | waiting: register_user | failed: adopt_presets (…)"
//
// Called from the pipeline mode dispatch instead of composeMicrocontext
// when the session's form is onboarding. lastTurnOps are the operations
// Agent1 just executed (StageEvent.Ops order) — they lead the line so
// Agent2 knows what THIS turn changed. Deterministic; no LLM involvement.
func ComposeOnboardingMicrocontext(m *domain.OnboardingManifest, lastTurnOps []string) string {
	var segments []string
	if len(lastTurnOps) > 0 {
		segments = append(segments, "turn: "+strings.Join(lastTurnOps, ", "))
	}

	if m == nil || len(m.Steps) == 0 {
		segments = append(segments, "manifest: empty")
		return strings.Join(segments, " | ")
	}

	var staged, waiting, failed []string
	applied := 0
	for i := range m.Steps {
		st := &m.Steps[i]
		switch st.Status {
		case domain.ManifestStepProposed:
			staged = append(staged, stepLabel(st))
		case domain.ManifestStepAccepted:
			waiting = append(waiting, st.Op)
		case domain.ManifestStepApplied, domain.ManifestStepSkipped:
			applied++
		case domain.ManifestStepFailed:
			label := st.Op
			if st.Error != "" {
				label += " (" + st.Error + ")"
			}
			failed = append(failed, label)
		}
	}

	if len(staged) > 0 {
		segments = append(segments, "staged: "+strings.Join(staged, ", "))
	}
	if applied > 0 {
		segments = append(segments, "applied: "+strconv.Itoa(applied)+"/"+strconv.Itoa(len(m.Steps)))
	}
	if len(waiting) > 0 {
		segments = append(segments, "waiting: "+strings.Join(waiting, ", "))
	}
	if len(failed) > 0 {
		segments = append(segments, "failed: "+strings.Join(failed, ", "))
	}
	if len(segments) == 0 {
		segments = append(segments, "manifest: empty")
	}
	return strings.Join(segments, " | ")
}

// stepLabel renders one staged step: the op, plus its natural key when the
// params carry an obvious one (entity slug / value-set slug / automation
// name / operation instance names) — "define_entity(lead)" reads better
// than a bare op ×3, and "enable_operations(book_a_visit, find_requests)"
// gives Agent2 the instance names its business-bullet list needs (owner
// flow ruling 2026-07-28, case 2).
func stepLabel(st *domain.ManifestStep) string {
	key := ""
	switch st.Op {
	case "define_entity":
		if entity, ok := st.Params["entity"].(map[string]any); ok {
			key, _ = entity["slug"].(string)
		}
	case "define_value_set":
		key, _ = st.Params["slug"].(string)
	case "define_automation":
		key, _ = st.Params["name"].(string)
	case "enable_operations":
		key = strings.Join(operationInstances(st.Params), ", ")
	}
	if key != "" {
		return st.Op + "(" + key + ")"
	}
	return st.Op
}

// operationInstances lists the instance wire names of an enable_operations
// step, tolerating both JSONB-decoded ([]any) and Go-built
// ([]map[string]any) params.
func operationInstances(params map[string]any) []string {
	var items []any
	switch v := params["operations"].(type) {
	case []any:
		items = v
	case []map[string]any:
		items = make([]any, len(v))
		for i := range v {
			items[i] = v[i]
		}
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		if op, ok := item.(map[string]any); ok {
			if name, _ := op["instance"].(string); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}
