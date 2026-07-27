package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine"
	"keepstar_v5/internal/usecases"
)

// Step-submit — THE R6 PII choke point:
//
//	POST /api/v1/onboard/step/{stepId}/submit
//	  {sessionId, name?, email, password, formId?, blockId?}
//	→ 200 {status:"ok"|"error", message?, step?, apply:[…]}
//
// The registration form submits here via the widget's form_submit action —
// NEVER through /operations/invoke (which rejects register_user outright).
// The password's entire lifetime on this service: request body → this
// handler → ManifestApplier.ExecuteStep → AdminGateway.ProvisionUser →
// bcrypt on the admin side. It never enters session state, deltas, traces,
// v5_operation_runs, logs or the LLM. handler_onboard_step_test.go asserts
// the persisted JSON.

// onboardStepSubmitRequest is the submit body. Password is deliberately
// only ever decoded into this short-lived struct. Two accepted shapes:
// flat fields (scripts, e2e) or the widget FormProvider envelope
// {sessionId, formId, blockId, values:{name,email,password}} — nested
// values win when present (normalize()).
type onboardStepSubmitRequest struct {
	SessionID string `json:"sessionId"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	FormID    string `json:"formId,omitempty"`
	BlockID   string `json:"blockId,omitempty"`

	Values *onboardStepSubmitValues `json:"values,omitempty"`
}

// onboardStepSubmitValues is the widget form-values envelope (FormProvider
// posts {formId, values} — api/operations.js submitFormEndpoint).
type onboardStepSubmitValues struct {
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	Password string `json:"password,omitempty"`
}

// normalize promotes nested widget values onto the flat fields and drops
// the envelope so the password lives in exactly one place afterwards.
func (req *onboardStepSubmitRequest) normalize() {
	if req.Values == nil {
		return
	}
	if req.Values.Name != "" {
		req.Name = req.Values.Name
	}
	if req.Values.Email != "" {
		req.Email = req.Values.Email
	}
	if req.Values.Password != "" {
		req.Password = req.Values.Password
	}
	req.Values = nil
}

// StepSubmit handles POST /api/v1/onboard/step/{stepId}/submit.
func (h *OnboardHandler) StepSubmit(w http.ResponseWriter, r *http.Request) {
	if !checkOnboardCookie(w, r) {
		return
	}
	if !h.checkCheap(w, r) {
		return
	}
	if h.applier == nil {
		http.Error(w, "onboarding submit not configured", http.StatusServiceUnavailable)
		return
	}
	stepID := r.PathValue("stepId")
	if stepID == "" {
		http.Error(w, "stepId is required", http.StatusBadRequest)
		return
	}
	var req onboardStepSubmitRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	req.normalize()
	if req.SessionID == "" {
		http.Error(w, "sessionId is required", http.StatusBadRequest)
		return
	}
	if req.Email == "" || req.Password == "" {
		http.Error(w, "email and password are required", http.StatusBadRequest)
		return
	}

	step, err := h.applier.ExecuteStep(r.Context(), req.SessionID, stepID, map[string]any{
		"name":     req.Name,
		"email":    req.Email,
		"password": req.Password,
	})
	if err != nil {
		h.writeStepSubmitError(w, req, err)
		return
	}

	resp := map[string]any{
		"status": "ok",
		"step":   map[string]any{"id": step.ID, "op": step.Op, "status": step.Status},
		"apply":  []applyInstruction{},
	}
	// Zero-LLM success plaque swap (beat 3). Render failure degrades to the
	// form ack — the registration still happened.
	if doc, rerr := h.renderOnboardSuccessPlaque(r.Context(), step); rerr != nil {
		h.log.Warn("step submit: success plaque render unavailable; form ack fallback", "err", rerr)
		if req.FormID != "" {
			resp["apply"] = []applyInstruction{{Target: "form", FormID: req.FormID, Status: "ok", Message: "Account created"}}
		}
	} else {
		resp["apply"] = []applyInstruction{{Target: "block", BlockID: req.BlockID, Document: doc}}
	}
	writeJSON(w, http.StatusOK, resp)
}

// writeStepSubmitError maps applier errors onto the wire. Client-shape
// problems get transport statuses; a provisioning failure is a 200
// {status:"error"} with a form apply so the widget shows it inline and the
// user can re-submit (the step stays accepted server-side).
func (h *OnboardHandler) writeStepSubmitError(w http.ResponseWriter, req onboardStepSubmitRequest, err error) {
	switch {
	case errors.Is(err, usecases.ErrNoManifest), errors.Is(err, usecases.ErrStepNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, usecases.ErrStepNotSubmittable), errors.Is(err, usecases.ErrInvalidSubmit):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, usecases.ErrStepNotReady), errors.Is(err, usecases.ErrTenantNotProvisioned):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		// Provisioning failed upstream — retryable in-form. The message is
		// admin's response text only; the submitted credentials are never
		// echoed.
		resp := map[string]any{
			"status":  "error",
			"message": "registration failed",
			"apply":   []applyInstruction{},
		}
		if req.FormID != "" {
			resp["apply"] = []applyInstruction{{Target: "form", FormID: req.FormID, Status: "error", Message: "Registration failed — please try again"}}
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// renderOnboardSuccessPlaque runs the zero-LLM preview chain over the
// success_plaque preset (system-registry fallback under the onboarding
// system tenant — the plaque needs no adopted copy). Mirrors
// OperationsHandler.renderSuccessBlock with a step-result binding.
func (h *OnboardHandler) renderOnboardSuccessPlaque(ctx context.Context, step *domain.ManifestStep) (map[string]any, error) {
	if h.presets == nil {
		return nil, domain.ErrPresetNotFound
	}
	preset, err := h.presets.GetPublishedPreset(ctx, onboardingTenantSlug, successPlaquePreset)
	if err != nil {
		return nil, err
	}
	presetDoc, err := unmarshalPresetDoc(preset.DocumentJSON)
	if err != nil {
		return nil, err
	}
	var componentDocs []*engine.Document
	if h.components != nil {
		list, err := h.components.ListPublishedComponents(ctx, onboardingTenantSlug)
		if err == nil {
			for _, c := range list {
				if cd, cerr := unmarshalPresetDoc(c.DocumentJSON); cerr == nil {
					componentDocs = append(componentDocs, cd)
				}
			}
		}
	}
	doc := engine.Materialise(presetDoc, componentDocs)

	// Bind vocabulary: the step result keys (already camelCase: userId,
	// email, role) + the same system keys the invoke-path plaque binds.
	result := &domain.OperationResult{
		Operation: step.Op,
		Kind:      domain.KindMeta,
		Outcome:   domain.OutcomeOK,
		Summary:   "Account created",
		Output:    step.Result,
	}
	data := []map[string]any{resultBindData(result)}
	engine.ExpandReplicates(doc, len(data))
	engine.ResolveAndInline(doc)
	engine.BindData(doc, data)

	docMap, err := engineDocToHandlerMap(doc)
	if err != nil {
		return nil, err
	}
	docMap["theme"] = resolveThemeMap(ctx, h.themes, onboardingTenantSlug, h.log)
	return docMap, nil
}
