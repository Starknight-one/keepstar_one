package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine"
	"keepstar_v5/internal/engine/presets"
	"keepstar_v5/internal/ports"
)

// NavigationHandler owns POST /api/v1/navigation/expand and
// /api/v1/navigation/back. The two endpoints are state-mutating
// shortcuts that don't go through the LLM — they realise the
// adjacency graph (preset → drill-target preset, hardcoded today;
// future v5_presets.metadata) directly.
//
// expand: snapshot current view → materialise drill-target preset
// against ONE entity → write template + view zones, push snapshot.
//
// back:   pop snapshot → restore its template + view zones.
//
// Both endpoints are best-effort sync writes; failure modes surface
// as 4xx (entity not found, can't go back) or 5xx (DB write failure).
type NavigationHandler struct {
	state      ports.StatePort
	presets    ports.PresetPort
	components ports.ComponentPort
	log        *slog.Logger
}

// NewNavigationHandler constructs the handler with its three deps.
func NewNavigationHandler(state ports.StatePort, presetPort ports.PresetPort, componentPort ports.ComponentPort, log *slog.Logger) *NavigationHandler {
	return &NavigationHandler{
		state:      state,
		presets:    presetPort,
		components: componentPort,
		log:        log,
	}
}

type expandRequest struct {
	SessionID  string            `json:"sessionId"`
	EntityType domain.EntityType `json:"entityType"`
	EntityID   string            `json:"entityId"`
	TurnID     string            `json:"turnId,omitempty"`
}

type backRequest struct {
	SessionID string `json:"sessionId"`
	TurnID    string `json:"turnId,omitempty"`
}

type navResponse struct {
	Success     bool                   `json:"success"`
	Document    map[string]interface{} `json:"document,omitempty"`
	ViewMode    domain.ViewMode        `json:"viewMode,omitempty"`
	Focused     *domain.EntityRef      `json:"focused,omitempty"`
	StackSize   int                    `json:"stackSize"`
	CanGoBack   bool                   `json:"canGoBack"`
	PresetInUse string                 `json:"presetInUse,omitempty"`
}

// Expand handles POST /api/v1/navigation/expand.
func (h *NavigationHandler) Expand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req expandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SessionID == "" || req.EntityID == "" {
		http.Error(w, "sessionId and entityId are required", http.StatusBadRequest)
		return
	}
	if req.EntityType == "" {
		req.EntityType = domain.EntityTypeProduct
	}

	tenant := TenantFromContext(r.Context())
	if tenant == nil {
		http.Error(w, "tenant unresolved", http.StatusInternalServerError)
		return
	}

	state, err := h.state.GetState(r.Context(), req.SessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Locate the entity inside the data zone.
	dataMap, ok := findEntityMap(state, req.EntityType, req.EntityID)
	if !ok {
		http.Error(w, "entity not found in current data zone", http.StatusBadRequest)
		return
	}

	// Look up adjacency: source preset → drill target. Source comes from
	// the synthetic top-level marker stamped by visual_assembly.
	sourcePreset := readPresetInUseMap(state.Current.Template)
	target := presets.AdjacentPreset(sourcePreset)
	if target == "" {
		http.Error(w, "no drill target registered for current preset", http.StatusBadRequest)
		return
	}

	// Materialise the drill-target preset against the single entity.
	doc, err := h.materialise(r.Context(), tenant.Slug, target, dataMap)
	if err != nil {
		http.Error(w, "drill render failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	templateMap, err := engineDocToHandlerMap(doc)
	if err != nil {
		http.Error(w, "marshal document: "+err.Error(), http.StatusInternalServerError)
		return
	}
	templateMap[domain.TemplatePresetInUseKey] = target

	// Push the prior view (mode + focused + step + template snapshot)
	// onto the stack so /back can restore it without re-rendering.
	snapshot := &domain.ViewSnapshot{
		Mode:        state.View.Mode,
		Focused:     state.View.Focused,
		Refs:        buildEntityRefs(state.Current.Data),
		Step:        state.Step,
		Template:    state.Current.Template,
		PresetInUse: sourcePreset,
		CreatedAt:   time.Now(),
	}
	if err := h.state.PushView(r.Context(), req.SessionID, snapshot); err != nil {
		http.Error(w, "push view: "+err.Error(), http.StatusInternalServerError)
		return
	}

	stack, _ := h.state.GetViewStack(r.Context(), req.SessionID)
	newView := domain.ViewState{
		Mode:    domain.ViewModeDetail,
		Focused: &domain.EntityRef{Type: req.EntityType, ID: req.EntityID},
	}
	viewInfo := domain.DeltaInfo{
		TurnID:    req.TurnID,
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_expand",
		DeltaType: domain.DeltaTypePush,
		Path:      "view",
	}
	if _, err := h.state.UpdateView(r.Context(), req.SessionID, newView, stack, viewInfo); err != nil {
		http.Error(w, "update view: "+err.Error(), http.StatusInternalServerError)
		return
	}

	templateInfo := domain.DeltaInfo{
		TurnID:    req.TurnID,
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_expand",
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "current.template",
	}
	if _, err := h.state.UpdateTemplate(r.Context(), req.SessionID, templateMap, templateInfo); err != nil {
		http.Error(w, "update template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, navResponse{
		Success:     true,
		Document:    templateMap,
		ViewMode:    newView.Mode,
		Focused:     newView.Focused,
		StackSize:   len(stack),
		CanGoBack:   len(stack) > 0,
		PresetInUse: target,
	})
}

// Back handles POST /api/v1/navigation/back.
func (h *NavigationHandler) Back(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req backRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	snap, err := h.state.PopView(r.Context(), req.SessionID)
	if err != nil {
		http.Error(w, "pop view: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if snap == nil {
		http.Error(w, "cannot go back: stack is empty", http.StatusBadRequest)
		return
	}

	stack, _ := h.state.GetViewStack(r.Context(), req.SessionID)
	restoredView := domain.ViewState{
		Mode:    snap.Mode,
		Focused: snap.Focused,
	}
	viewInfo := domain.DeltaInfo{
		TurnID:    req.TurnID,
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_back",
		DeltaType: domain.DeltaTypePop,
		Path:      "view",
	}
	if _, err := h.state.UpdateView(r.Context(), req.SessionID, restoredView, stack, viewInfo); err != nil {
		http.Error(w, "update view: "+err.Error(), http.StatusInternalServerError)
		return
	}

	templateMap := snap.Template
	if templateMap == nil {
		templateMap = map[string]interface{}{}
	}
	templateInfo := domain.DeltaInfo{
		TurnID:    req.TurnID,
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_back",
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "current.template",
	}
	if _, err := h.state.UpdateTemplate(r.Context(), req.SessionID, templateMap, templateInfo); err != nil {
		http.Error(w, "update template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, navResponse{
		Success:     true,
		Document:    templateMap,
		ViewMode:    restoredView.Mode,
		Focused:     restoredView.Focused,
		StackSize:   len(stack),
		CanGoBack:   len(stack) > 0,
		PresetInUse: snap.PresetInUse,
	})
}

// materialise loads target preset + components and runs the V5 engine
// pipeline (Materialise → ResolveAndInline → BindData → InjectDefaultActions)
// against ONE entity. No replicate fan-out — detail views render a
// single instance.
func (h *NavigationHandler) materialise(ctx context.Context, tenantSlug string, target string, entity map[string]interface{}) (*engine.Document, error) {
	preset, err := h.presets.GetPublishedPreset(ctx, tenantSlug, target)
	if err != nil {
		return nil, fmt.Errorf("load preset %q: %w", target, err)
	}
	presetDoc, err := unmarshalPresetDoc(preset.DocumentJSON)
	if err != nil {
		return nil, fmt.Errorf("parse preset doc: %w", err)
	}

	componentList, err := h.components.ListPublishedComponents(ctx, tenantSlug)
	if err != nil {
		return nil, fmt.Errorf("list components: %w", err)
	}
	componentDocs := make([]*engine.Document, 0, len(componentList))
	for _, c := range componentList {
		cd, err := unmarshalPresetDoc(c.DocumentJSON)
		if err != nil {
			h.log.Debug("nav: component doc unmarshal failed; skipping", "component", c.Name, "err", err)
			continue
		}
		componentDocs = append(componentDocs, cd)
	}
	merged := engine.Materialise(presetDoc, componentDocs)
	engine.ResolveAndInline(merged)
	engine.BindData(merged, []map[string]any{entity})
	engine.InjectDefaultActions(merged, domain.EntityTypeProduct, []map[string]any{entity})
	return merged, nil
}

// unmarshalPresetDoc parses raw doc bytes into engine.Document.
func unmarshalPresetDoc(raw json.RawMessage) (*engine.Document, error) {
	var doc engine.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// engineDocToHandlerMap round-trips engine.Document → JSON →
// map[string]interface{}.
func engineDocToHandlerMap(doc *engine.Document) (map[string]interface{}, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// findEntityMap returns the data-map shape (engine.ProductToMap) of
// the named entity from the current data zone, or false if not found.
func findEntityMap(state *domain.SessionState, et domain.EntityType, id string) (map[string]any, bool) {
	switch et {
	case domain.EntityTypeProduct:
		for _, p := range state.Current.Data.Products {
			if p.ID == id {
				return engine.ProductToMap(p), true
			}
		}
	}
	return nil, false
}

// buildEntityRefs flattens the data zone into EntityRef list (for
// snapshots).
func buildEntityRefs(data domain.StateData) []domain.EntityRef {
	out := make([]domain.EntityRef, 0, len(data.Products)+len(data.Services))
	for _, p := range data.Products {
		out = append(out, domain.EntityRef{Type: domain.EntityTypeProduct, ID: p.ID})
	}
	for _, s := range data.Services {
		out = append(out, domain.EntityRef{Type: domain.EntityTypeService, ID: s.ID})
	}
	return out
}

// readPresetInUseMap reads the stamped __presetInUse marker from a
// template map. Empty when freestyle / modify (no preset to drill into).
func readPresetInUseMap(tpl map[string]interface{}) string {
	if tpl == nil {
		return ""
	}
	v, _ := tpl[domain.TemplatePresetInUseKey].(string)
	return v
}
