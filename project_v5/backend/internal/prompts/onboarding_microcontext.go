package prompts

import (
	"strconv"
	"strings"

	"keepstar_v5/internal/domain"
)

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
// name) — "define_entity(lead)" reads better than a bare op ×3.
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
	}
	if key != "" {
		return st.Op + "(" + key + ")"
	}
	return st.Op
}
