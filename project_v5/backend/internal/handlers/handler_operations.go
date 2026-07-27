package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine"
	"keepstar_v5/internal/ports"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OperationsHandler owns POST /api/v1/operations/invoke (R1) — the single
// public operation endpoint. Widget `operation_invoke` actions (booking
// form submit, advance_lead buttons) land here; execution goes through THE
// registry choke point (resolve → mode+role gate → validate → execute →
// redact → audit), so this handler adds only transport concerns: the cheap
// rate bucket, tenant scoping, the session-derived OperationContext, the
// R28 record delta, and the deterministic apply[] mapping.
//
// Wire shape (RUNTIME_SPEC.md §4.8):
//
//	POST /api/v1/operations/invoke
//	  {sessionId, operation, params, entity?:{type,id}, formId?, blockId?}
//	→ 200 {status:"ok"|"error", message?, result?,
//	       apply:[ {target:"block", blockId, document}
//	             | {target:"form",  formId, status, message} ]}
//
// v1 apply targets are block|form ONLY (session_actions/document/toast are
// cut). `register_user` is NOT reachable here (R6).
type OperationsHandler struct {
	registry   ports.OperationRegistry
	state      ports.StatePort     // record delta append (best-effort); nil skips
	presets    ports.PresetPort    // success_plaque zero-LLM render; nil → form ack fallback
	components ports.ComponentPort // optional; nil → no component merge in the block render
	themes     ports.ThemePort     // optional; nil → default theme on the block document
	pool       *pgxpool.Pool       // session mode/role/tenant scoping; nil → storefront/visitor defaults (tests)
	guard      *PipelineGuard      // cheap bucket (§6.3 CHEAP_RATE_PER_MIN); nil disables
	log        *slog.Logger
}

// successPlaquePreset is the R20 seed preset the ok-apply block swap
// renders (zero-LLM). Cross-lane contract with the surfaces workstream:
// its fieldBindings are authored against resultBindData's key vocabulary.
const successPlaquePreset = "success_plaque"

// registerUserOperation is rejected at this endpoint unconditionally (R6):
// registration credentials NEVER pass through the operation registry — the
// registration form submits via form_submit to the onboarding step-submit
// endpoint instead.
const registerUserOperation = "register_user"

// NewOperationsHandler constructs the handler. registry and log are
// required; every other dependency is optional and degrades as documented
// on the struct fields.
func NewOperationsHandler(
	registry ports.OperationRegistry,
	state ports.StatePort,
	presets ports.PresetPort,
	components ports.ComponentPort,
	themes ports.ThemePort,
	pool *pgxpool.Pool,
	guard *PipelineGuard,
	log *slog.Logger,
) *OperationsHandler {
	return &OperationsHandler{
		registry:   registry,
		state:      state,
		presets:    presets,
		components: components,
		themes:     themes,
		pool:       pool,
		guard:      guard,
		log:        log,
	}
}

// NewCheapGuard builds the §6.3 cheap per-IP rate bucket (env
// CHEAP_RATE_PER_MIN, default 120/min/IP): a PipelineGuard with the daily
// spend ceiling disabled — these routes trigger no LLM spend. One shared
// instance covers /session/init, /actions, /navigation/*,
// /operations/invoke and the onboarding submit routes.
func NewCheapGuard(ratePerMin int, log *slog.Logger) *PipelineGuard {
	return NewPipelineGuard(ratePerMin, 0, log)
}

// operationInvokeRequest is the body of POST /api/v1/operations/invoke.
type operationInvokeRequest struct {
	SessionID string            `json:"sessionId"`
	Operation string            `json:"operation"`
	Params    map[string]any    `json:"params,omitempty"`
	Entity    *domain.EntityRef `json:"entity,omitempty"`
	FormID    string            `json:"formId,omitempty"`
	BlockID   string            `json:"blockId,omitempty"`
}

// operationInvokeResponse is the R1 response shape. Apply is always
// present (possibly empty) so the widget applier can iterate blindly.
type operationInvokeResponse struct {
	Status  string             `json:"status"` // "ok" | "error"
	Message string             `json:"message,omitempty"`
	Result  *invokeResult      `json:"result,omitempty"`
	Apply   []applyInstruction `json:"apply"`
}

// invokeResult is the public projection of an OperationResult: identity +
// outcome only. Output/Metadata stay server-side — the apply[] contract
// (rendered block / form status) is how results reach the surface, and
// keeping raw output out of the public wire keeps the R6 redaction
// boundary trivially true here.
type invokeResult struct {
	Operation  string `json:"operation"`
	Kind       string `json:"kind"`
	Outcome    string `json:"outcome"`
	Count      int    `json:"count,omitempty"`
	EntityKind string `json:"entityKind,omitempty"`
	RecordID   string `json:"recordId,omitempty"`
	Summary    string `json:"summary"`
}

// applyInstruction is one minimal apply[] entry — targets block|form ONLY
// in v1 (R1).
type applyInstruction struct {
	Target   string         `json:"target"` // "block" | "form"
	BlockID  string         `json:"blockId,omitempty"`
	Document map[string]any `json:"document,omitempty"`
	FormID   string         `json:"formId,omitempty"`
	Status   string         `json:"status,omitempty"` // "ok" | "error" (form target)
	Message  string         `json:"message,omitempty"`
}

// Invoke handles POST /api/v1/operations/invoke.
func (h *OperationsHandler) Invoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if ok, _ := h.guard.Allow(clientIP(r)); !ok {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	tenant := TenantFromContext(r.Context())
	if tenant == nil {
		http.Error(w, "tenant unresolved", http.StatusInternalServerError)
		return
	}

	var req operationInvokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(w, "sessionId is required", http.StatusBadRequest)
		return
	}
	if req.Operation == "" {
		http.Error(w, "operation is required", http.StatusBadRequest)
		return
	}
	if req.Operation == registerUserOperation {
		http.Error(w, "register_user is not invokable (R6)", http.StatusForbidden)
		return
	}

	mode, role, status, msg := h.resolveSession(r.Context(), req.SessionID, tenant.ID)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}

	// R14/§4.8: role from the session row (visitor default), mode likewise;
	// createdBy identity is the session — "visitor:<session_id>" (v1 has no
	// logged-in runtime user; CRM staff is token-granted and still
	// session-anonymous).
	octx := domain.OperationContext{
		SessionID:  req.SessionID,
		TenantSlug: tenant.Slug,
		TenantID:   tenant.ID,
		Mode:       mode,
		Role:       role,
		ActorID:    "visitor:" + req.SessionID,
	}

	result, err := h.registry.Execute(r.Context(), octx, domain.ToolCall{
		Name:  req.Operation,
		Input: req.Params,
	})
	if err != nil {
		h.log.Error("invoke: operation execute failed", "operation", req.Operation, "tenant", tenant.Slug, "err", err)
		http.Error(w, "operation failed", http.StatusInternalServerError)
		return
	}

	h.appendRecordDelta(r.Context(), req, result)
	writeJSON(w, http.StatusOK, h.buildResponse(r.Context(), tenant, req, result))
}

// resolveSession reads the session row and scopes it to the request tenant.
// Returns (mode, role, 0, "") on success; a non-zero status + message maps
// straight to http.Error. pool == nil (unit tests) degrades to the
// storefront form / visitor role — production wiring always passes a pool.
func (h *OperationsHandler) resolveSession(ctx context.Context, sessionID, tenantID string) (domain.PipelineMode, domain.Role, int, string) {
	mode, role := domain.ModeStorefront, domain.RoleVisitor
	if h.pool == nil {
		return mode, role, 0, ""
	}
	var rowTenant, m, ro string
	var killedAt *time.Time
	err := h.pool.QueryRow(ctx,
		`SELECT tenant_id::text, COALESCE(mode, ''), COALESCE(role, ''), killed_at
		 FROM v5_chat_sessions WHERE id = $1::uuid`,
		sessionID,
	).Scan(&rowTenant, &m, &ro, &killedAt)
	if err != nil {
		// Unknown id and malformed UUID both land here — same 404.
		return mode, role, http.StatusNotFound, "session not found"
	}
	if killedAt != nil {
		return mode, role, http.StatusConflict, "session killed"
	}
	if rowTenant != tenantID {
		return mode, role, http.StatusForbidden, "session does not belong to tenant"
	}
	if m != "" {
		mode = domain.PipelineMode(m)
	}
	if ro != "" {
		role = domain.Role(ro)
	}
	return mode, role, 0, ""
}

// appendRecordDelta writes the R28 session delta for entity MUTATIONS only
// (Path "records"): RECORD_CREATE for create_record + schedule_slot (R11:
// create semantics), RECORD_UPDATE, RECORD_TRANSITION. Queries are NOT
// written here — the query executor's own Entities-zone write emits
// ENTITY_QUERY (Path "data", §4.2); notify has no delta (R28). Best-effort:
// the mutation already committed, a delta failure logs and never fails the
// response. Raw req.Params are deliberately NOT persisted (R6 discipline —
// the redacted audit copy lives in v5_operation_runs).
func (h *OperationsHandler) appendRecordDelta(ctx context.Context, req operationInvokeRequest, result *domain.OperationResult) {
	if h.state == nil || result.Outcome != domain.OutcomeOK {
		return
	}
	actionType, deltaType, ok := domain.RecordDeltaShape(result.Kind)
	if !ok {
		return
	}
	params := map[string]any{"operation": result.Operation}
	if result.EntityKind != "" {
		params["entityKind"] = result.EntityKind
	}
	if result.RecordID != "" {
		params["recordId"] = result.RecordID
	}
	if req.Entity != nil && req.Entity.ID != "" {
		params["refEntityType"] = string(req.Entity.Type)
		params["refEntityId"] = req.Entity.ID
	}
	delta := domain.DeltaInfo{
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "visitor:" + req.SessionID,
		DeltaType: deltaType,
		Path:      "records",
		Action:    domain.Action{Type: actionType, Tool: result.Operation, Params: params},
		Result:    domain.ResultMeta{Count: result.Count},
	}.ToDelta()
	if _, err := h.state.AddDelta(ctx, req.SessionID, delta); err != nil {
		h.log.Warn("invoke: record delta append failed",
			"session", req.SessionID, "operation", result.Operation, "err", err)
	}
}

// buildResponse maps the structured result onto the deterministic apply[]
// contract (§4.8):
//
//   - ok create_record/schedule_slot → {target:"block"}: success_plaque
//     rendered zero-LLM and bound from the result; render failure (preset
//     not adopted, etc.) degrades to the form ack.
//   - other ok/empty outcomes → form ack when formId present.
//   - invalid/denied/error → status "error" + {target:"form"} carrying the
//     result Summary (the LLM-free human-readable reason).
func (h *OperationsHandler) buildResponse(ctx context.Context, tenant *domain.Tenant, req operationInvokeRequest, result *domain.OperationResult) operationInvokeResponse {
	resp := operationInvokeResponse{
		Result: &invokeResult{
			Operation:  result.Operation,
			Kind:       string(result.Kind),
			Outcome:    string(result.Outcome),
			Count:      result.Count,
			EntityKind: result.EntityKind,
			RecordID:   result.RecordID,
			Summary:    result.Summary,
		},
		Apply: []applyInstruction{},
	}

	switch result.Outcome {
	case domain.OutcomeOK, domain.OutcomeEmpty:
		resp.Status = "ok"
		resp.Message = result.Summary
		if result.Outcome == domain.OutcomeOK &&
			(result.Kind == domain.KindCreateRecord || result.Kind == domain.KindScheduleSlot) {
			doc, err := h.renderSuccessBlock(ctx, tenant, result)
			if err != nil {
				h.log.Warn("invoke: success block render unavailable; form ack fallback",
					"tenant", tenant.Slug, "operation", result.Operation, "err", err)
			} else {
				resp.Apply = append(resp.Apply, applyInstruction{
					Target:   "block",
					BlockID:  req.BlockID,
					Document: doc,
				})
			}
		}
		if len(resp.Apply) == 0 && req.FormID != "" {
			resp.Apply = append(resp.Apply, applyInstruction{
				Target: "form", FormID: req.FormID, Status: "ok", Message: result.Summary,
			})
		}
	default: // invalid | denied | error
		resp.Status = "error"
		resp.Message = result.Summary
		if req.FormID != "" {
			resp.Apply = append(resp.Apply, applyInstruction{
				Target: "form", FormID: req.FormID, Status: "error", Message: result.Summary,
			})
		}
	}
	return resp
}

// renderSuccessBlock runs the zero-LLM preview chain (handler_presets.go
// Preview pattern) over the tenant's adopted success_plaque preset, bound
// with the operation result: Materialise → ExpandReplicates →
// ResolveAndInline → BindData → theme attach. No InjectDefaultActions —
// the plaque binds a result, not catalog entities.
func (h *OperationsHandler) renderSuccessBlock(ctx context.Context, tenant *domain.Tenant, result *domain.OperationResult) (map[string]any, error) {
	if h.presets == nil {
		return nil, domain.ErrPresetNotFound
	}
	preset, err := h.presets.GetPublishedPreset(ctx, tenant.Slug, successPlaquePreset)
	if err != nil {
		return nil, err
	}
	presetDoc, err := unmarshalPresetDoc(preset.DocumentJSON)
	if err != nil {
		return nil, err
	}
	var componentDocs []*engine.Document
	if h.components != nil {
		list, err := h.components.ListPublishedComponents(ctx, tenant.Slug)
		if err != nil {
			return nil, err
		}
		for _, c := range list {
			cd, err := unmarshalPresetDoc(c.DocumentJSON)
			if err != nil {
				h.log.Debug("invoke: component doc unmarshal failed; skipping", "component", c.Name, "err", err)
				continue
			}
			componentDocs = append(componentDocs, cd)
		}
	}
	doc := engine.Materialise(presetDoc, componentDocs)

	data := []map[string]any{resultBindData(result)}
	engine.ExpandReplicates(doc, len(data))
	engine.ResolveAndInline(doc)
	engine.BindData(doc, data)

	docMap, err := engineDocToHandlerMap(doc)
	if err != nil {
		return nil, err
	}
	docMap["theme"] = resolveThemeMap(ctx, h.themes, tenant.ID, h.log)
	return docMap, nil
}

// resultBindData is the binding vocabulary of the ok-apply block: the
// result's Output keys (SnakeToCamel — THE only normalizer, §6.2) plus the
// system keys operation / outcome / summary / recordId / entityKind /
// count. Cross-lane contract: success_plaque fieldBindings are authored
// against exactly these keys.
func resultBindData(result *domain.OperationResult) map[string]any {
	m := make(map[string]any, len(result.Output)+6)
	for k, v := range result.Output {
		m[engine.SnakeToCamel(k)] = v
	}
	m["operation"] = result.Operation
	m["outcome"] = string(result.Outcome)
	m["summary"] = result.Summary
	if result.RecordID != "" {
		m["recordId"] = result.RecordID
	}
	if result.EntityKind != "" {
		m["entityKind"] = result.EntityKind
	}
	if result.Count > 0 {
		m["count"] = result.Count
	}
	return m
}
