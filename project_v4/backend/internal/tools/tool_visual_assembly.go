package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"keepstar_v4/internal/domain"
	engine_v4 "keepstar_v4/internal/engine_v4"
	"keepstar_v4/internal/ports"
)

// VisualAssemblyTool renders entities using the V4 Pencil-based engine.
// ONE mode: preset (macro) + ops (micro). No auto/ops/build split.
type VisualAssemblyTool struct {
	statePort     ports.StatePort
	engine        *engine_v4.Engine
	tenantPresets *engine_v4.TenantPresetLoader // may be nil — loader handles nil
}

// NewVisualAssemblyTool creates the V4 visual assembly tool.
// tenantPresets may be nil — the tool then serves only the global engine_v4
// registry, which is the boot-without-DB fallback.
func NewVisualAssemblyTool(statePort ports.StatePort, eng *engine_v4.Engine, tenantPresets *engine_v4.TenantPresetLoader) *VisualAssemblyTool {
	return &VisualAssemblyTool{
		statePort:     statePort,
		engine:        eng,
		tenantPresets: tenantPresets,
	}
}

// Definition returns the tool definition for LLM.
// Key difference from V2: no mode field, no atoms override map, no show/hide/order.
// Ops-only: build from scratch or modify existing formation.
func (t *VisualAssemblyTool) Definition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "visual_assembly",
		Description: "Build or modify visual formation using ops. Insert widgets, atoms, and layout nodes to build any UI.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"mode"},
			"properties": map[string]interface{}{
				"mode": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"rebuild", "modify"},
					"description": "REQUIRED. \"rebuild\" discards the previous formation and builds a fresh one from ops/preset — use when the user asks for something new (new data, detail view, empty state, different layout). \"modify\" loads the existing formation and applies ops as deltas targeting atoms by fieldName or tree ID — use for style tweaks, removals, additions on the current view (\"make price red\", \"remove rating\", \"2 columns\"). No default: pick per turn based on user intent.",
				},
				"ops": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"op": map[string]interface{}{
								"type": "string",
								"enum": []string{"insert", "update", "delete", "move"},
							},
							"target": map[string]interface{}{
								"type":        "string",
								"description": "Atom/node field name (applies to all widgets) or specific ID from formation_tree.",
							},
							"ref": map[string]interface{}{
								"type":        "string",
								"description": "Binding name — subsequent ops can reference via $ref.",
							},
							"parent": map[string]interface{}{
								"type":        "string",
								"description": "Parent ID or $ref for insert/move.",
							},
							"after": map[string]interface{}{
								"type":        "string",
								"description": "Insert/move after this element.",
							},
							"props": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"type": map[string]interface{}{
										"type":        "string",
										"description": "Atom: text, number, image, icon. Layout: row, column, flow, span. Container: widget.",
									},
									"value":     map[string]interface{}{"description": "Literal value for freestyle atoms."},
									"fieldName": map[string]interface{}{"type": "string", "description": "Field name for data-bound atoms."},
									"textStyle": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"fontSize":       map[string]interface{}{"type": "string", "enum": []string{"xs", "sm", "md", "lg", "xl", "2xl", "3xl"}},
											"fontWeight":     map[string]interface{}{"type": "string", "enum": []string{"light", "normal", "medium", "semibold", "bold"}},
											"color":          map[string]interface{}{"type": "string", "description": "Semantic token or hex: red, green, blue, muted, #FF0000"},
											"textDecoration": map[string]interface{}{"type": "string", "enum": []string{"none", "underline", "line-through"}},
											"textTransform":  map[string]interface{}{"type": "string", "enum": []string{"none", "uppercase", "lowercase", "capitalize"}},
											"lineClamp":      map[string]interface{}{"type": "integer", "description": "Max visible lines"},
											"lineHeight":     map[string]interface{}{"type": "string", "enum": []string{"tight", "normal", "relaxed", "loose"}},
											"letterSpacing":  map[string]interface{}{"type": "string", "enum": []string{"tight", "normal", "wide"}},
										},
										"description": "Text styling. Color, fontSize, fontWeight MUST be inside this object.",
									},
									"wrapper": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"type":    map[string]interface{}{"type": "string", "enum": []string{"none", "badge", "tag", "pill", "avatar", "tooltip", "alert", "link", "progress", "button"}},
											"variant": map[string]interface{}{"type": "string", "enum": []string{"primary", "secondary", "success", "error", "warning", "outline"}},
										},
									},
									"mediaStyle": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"aspectRatio": map[string]interface{}{"type": "string", "enum": []string{"1:1", "4:3", "16:9", "auto"}},
											"objectFit":   map[string]interface{}{"type": "string", "enum": []string{"cover", "contain", "fill"}},
										},
									},
									"format": map[string]interface{}{
										"type": "string",
										"enum": []string{"currency", "stars", "stars-compact", "stars-text", "percent", "number", "date", "text"},
									},
									"slot": map[string]interface{}{
										"type": "string",
										"enum": []string{"hero", "title", "price", "primary", "secondary", "badge", "tags", "description"},
									},
								},
							},
						},
						"required": []string{"op"},
					},
					"description": "Operations on the formation tree. Use WITH preset for overrides, or WITHOUT for freestyle builds.",
				},
				"layout": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"grid", "list", "single", "carousel"},
					"description": "Formation layout type.",
				},
				"columns": map[string]interface{}{
					"type":        "integer",
					"description": "Number of columns for grid layout.",
				},
				"size": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"tiny", "small", "medium", "large"},
					"description": "Widget size.",
				},
				"replicate": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, replicate the single widget template across data items from state (one widget per product/service). Default false. Use for grids/lists of search results. Leave false for freestyle compositions, detail views, or widgets built from literal values.",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Max number of data items to use (0 or omitted = all). Caps replication count. Ignored when replicate=false.",
				},
				"preset": map[string]interface{}{
					"type": "string",
					"description": "Named preset — expands to a prebuilt ops batch. " +
						"Global fallbacks: product_card, product_card_compact, " +
						"product_card_horizontal, product_card_list_row, product_detail, " +
						"product_detail_horizontal, text_explainer, empty_not_found, " +
						"error_generic, catalog_category_card, liked_grid, cart_grid. " +
						"Tenant-scoped presets (and overrides of the fallbacks above) are " +
						"listed in the <tenant_design_context> block of the system prompt — " +
						"pass those names verbatim. Pair with `ops` to apply overrides on " +
						"top (update/delete/insert targeting preset refs like $w, $root, " +
						"$info, $meta). Each preset has a DefaultReplicate baked in; " +
						"passing `replicate` explicitly overrides it.",
				},
			},
		},
	}
}

// Execute runs the visual assembly tool.
func (t *VisualAssemblyTool) Execute(ctx context.Context, toolCtx ToolContext, params map[string]interface{}) (*domain.ToolResult, error) {
	// Load current state
	state, err := t.statePort.GetState(ctx, toolCtx.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// Mode — explicit, no default. Agent2 MUST decide per turn:
	//   "rebuild" → discard existing formation, build fresh from ops/preset
	//   "modify"  → load existing formation, apply ops as deltas (target by fieldName / tree id)
	mode, _ := params["mode"].(string)
	if mode != "rebuild" && mode != "modify" {
		return &domain.ToolResult{Content: fmt.Sprintf("invalid mode: %q — must be \"rebuild\" or \"modify\"", mode)}, nil
	}

	// Build engine input
	engineInput := engine_v4.ExecuteInput{}

	// Ops
	if opsRaw, ok := params["ops"].([]interface{}); ok && len(opsRaw) > 0 {
		ops, parseErr := engine_v4.ParseOps(opsRaw)
		if parseErr != nil {
			return &domain.ToolResult{Content: fmt.Sprintf("error parsing ops: %v", parseErr)}, nil
		}
		engineInput.Ops = ops
	}

	// Layout settings
	if layout, ok := params["layout"].(string); ok {
		engineInput.Layout = layout
	}
	if cols, ok := params["columns"].(float64); ok {
		engineInput.Columns = int(cols)
	}
	if size, ok := params["size"].(string); ok {
		engineInput.Size = size
	}
	_, replicatePassed := params["replicate"]
	if rep, ok := params["replicate"].(bool); ok {
		engineInput.Replicate = rep
	}
	if lim, ok := params["limit"].(float64); ok {
		engineInput.Limit = int(lim)
	}

	// Tenant-aware resolver (KeepstarCanvas Phase 2). Captured once per turn
	// so both the top-level preset lookup below and inline per-widget preset
	// expansion inside ApplyOps honour the tenant's published library. When
	// the loader is nil (no DB) ResolverFor returns nil and the engine falls
	// back to the global engine_v4 registry — same path as pre-Phase-2.
	resolver := t.tenantPresets.ResolverFor(ctx, toolCtx.TenantSlug)
	engineInput.PresetResolver = resolver

	// Preset expansion (B2). Prepend preset ops to any user-supplied overrides
	// so $ref bindings created by the preset are visible to override ops in
	// the same ApplyOps batch. Inherit the preset's DefaultReplicate only when
	// the caller did not pass `replicate` explicitly.
	if presetName, ok := params["preset"].(string); ok && presetName != "" {
		if mode != "rebuild" {
			return &domain.ToolResult{Content: "preset can only be used with mode=\"rebuild\""}, nil
		}
		if errMsg := validatePresetWithUserOps(presetName, engineInput.Ops); errMsg != "" {
			return &domain.ToolResult{Content: errMsg}, nil
		}
		// Tenant-scoped published preset wins when present; otherwise fall
		// back to the global Go registry via resolvePreset. This is the
		// primary Phase 2 integration point — a tenant that publishes
		// `product_card` via the admin canvas CRUD endpoints will serve
		// their version starting on the next Agent2 turn (modulo the
		// loader's 60s TTL cache).
		p, found := t.resolvePresetForTenant(ctx, toolCtx.TenantSlug, presetName)
		if !found {
			return &domain.ToolResult{Content: fmt.Sprintf("unknown preset: %q", presetName)}, nil
		}
		presetOps := p.Build()
		engineInput.Ops = append(presetOps, engineInput.Ops...)
		if !replicatePassed {
			engineInput.Replicate = p.DefaultReplicate
		}
	}

	// Entity data from state
	if len(state.Current.Data.Products) > 0 || len(state.Current.Data.Services) > 0 {
		entityType := "product"
		if len(state.Current.Data.Services) > 0 && len(state.Current.Data.Products) == 0 {
			entityType = "service"
		}
		engineInput.EntityType = entityType

		for _, p := range state.Current.Data.Products {
			engineInput.Data = append(engineInput.Data, ProductToMap(p))
		}
		for _, s := range state.Current.Data.Services {
			engineInput.Data = append(engineInput.Data, ServiceToMap(s))
		}
	}

	// Mode routing — explicit per-turn decision from Agent2.
	//   rebuild → formation starts empty, previous turn is discarded
	//   modify  → load existing formation, ops apply as deltas
	if mode == "modify" && len(engineInput.Ops) > 0 {
		if state.Current.Template != nil {
			if fData, ok := state.Current.Template["formation"]; ok {
				engineInput.Formation = convertToFormation(fData)
			}
		}
		if engineInput.Formation != nil {
			engineInput.Ops = engine_v4.ExpandWildcardOps(engineInput.Formation, engineInput.Ops)
		}
	}

	// Execute engine — ONE pipeline
	output := t.engine.Execute(engineInput)

	if output.Formation == nil {
		msg := "no formation generated"
		if len(output.Warnings) > 0 {
			msg += ": " + output.Warnings[0]
		}
		return &domain.ToolResult{Content: msg}, nil
	}

	// Write formation to state
	templateMap := map[string]interface{}{"formation": output.Formation}
	if output.TreeMap != nil {
		templateMap["formation_tree"] = output.TreeMap
	}

	info := domain.DeltaInfo{
		TurnID:  toolCtx.TurnID,
		Trigger: domain.TriggerUserQuery,
		Source:  domain.SourceLLM,
		ActorID: toolCtx.ActorID,
	}
	_, writeErr := t.statePort.UpdateTemplate(ctx, toolCtx.SessionID, templateMap, info)
	if writeErr != nil {
		return nil, fmt.Errorf("write template: %w", writeErr)
	}

	// Build result message
	widgetCount := len(output.Formation.Widgets)
	for _, s := range output.Formation.Sections {
		widgetCount += len(s.Widgets)
	}

	result := fmt.Sprintf("Formation rendered: %d widgets, layout=%s", widgetCount, output.Formation.Mode)
	if len(output.Warnings) > 0 {
		result += fmt.Sprintf(" (%d warnings)", len(output.Warnings))
	}

	return &domain.ToolResult{Content: result}, nil
}

// resolvePresetForTenant consults the tenant preset loader first, then the
// global engine_v4 registry. A nil loader means the V4 server is running
// without the admin DB wired in — in that case this collapses to the legacy
// global-only behaviour. Safe to call with an empty tenantSlug.
func (t *VisualAssemblyTool) resolvePresetForTenant(ctx context.Context, tenantSlug, name string) (*engine_v4.Preset, bool) {
	if t.tenantPresets != nil {
		if p, ok := t.tenantPresets.Resolve(ctx, tenantSlug, name); ok {
			return p, true
		}
	}
	return engine_v4.GetPreset(name)
}

// convertToFormation converts a map[string]interface{} to FormationWithData via JSON roundtrip.
func convertToFormation(data interface{}) *domain.FormationWithData {
	if f, ok := data.(*domain.FormationWithData); ok {
		return f
	}
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var formation domain.FormationWithData
	if err := json.Unmarshal(jsonBytes, &formation); err != nil {
		return nil
	}
	return &formation
}

// ProductToMap converts a Product to a flat map for data binding.
// Exported so usecases/handlers can prepare data for engine_v4.
//
// Tenant-specific fields stored in Product.Extra (e.g. model/manufacturer/cpu
// for an electronics catalog, title/author/isbn for books) are spread into the
// map alongside the typed hey-babes-shape fields. Typed fields win on key
// collision so a tenant can't accidentally override a well-known field by
// putting the same key in Extra.
func ProductToMap(p domain.Product) map[string]interface{} {
	m := make(map[string]interface{})
	if p.Name != "" {
		m["name"] = p.Name
	}
	if p.OriginalName != "" {
		m["originalName"] = p.OriginalName
	}
	if p.Price > 0 {
		m["price"] = p.Price
	}
	if p.Currency != "" {
		m["currency"] = p.Currency
	}
	if p.Rating > 0 {
		m["rating"] = p.Rating
	}
	if p.Brand != "" {
		m["brand"] = p.Brand
	}
	if p.Category != "" {
		m["category"] = p.Category
	}
	if p.Description != "" {
		m["description"] = p.Description
	}
	if len(p.Images) > 0 {
		m["images"] = p.Images[0]
	}
	if len(p.Tags) > 0 {
		m["tags"] = p.Tags
	}
	if p.SKU != "" {
		m["sku"] = p.SKU
	}
	if len(p.GTINs) > 0 {
		m["gtins"] = p.GTINs
	}
	if p.Size != "" {
		m["size"] = p.Size
	}
	if p.Color != "" {
		m["color"] = p.Color
	}
	if p.WeightG != nil {
		m["weightG"] = *p.WeightG
	}
	if p.VolumeML != nil {
		m["volumeMl"] = *p.VolumeML
	}
	// Spread tenant-specific extension fields. Typed fields above already
	// claimed the hey-babes-shape keys, so Extra only fills in what's missing.
	for k, v := range p.Extra {
		if _, exists := m[k]; !exists {
			m[k] = v
		}
	}
	return m
}

// validatePresetWithUserOps enforces that top-level `preset` is mutually
// exclusive with multi-widget composition. The intent: top-level preset means
// "build one widget template from this preset" (legacy single-grid/detail
// flow). Multi-widget compositions use per-widget preset in props instead.
//
// Rules:
//   - Top-level preset + >1 widget insert in user ops → error.
//     The user is composing multiple widgets but using the legacy preset slot
//     instead of per-widget preset. Force them to migrate.
//   - Top-level preset + ANY widget insert with props.preset → error.
//     Mixing the two preset paradigms is ambiguous.
//
// Returns "" when valid, an error message otherwise.
func validatePresetWithUserOps(presetName string, ops []engine_v4.Op) string {
	if presetName == "" {
		return ""
	}
	widgetInsertCount := 0
	for _, op := range ops {
		if op.Type != engine_v4.OpInsert || op.Props == nil {
			continue
		}
		opType, _ := op.Props["type"].(string)
		if opType != "widget" {
			continue
		}
		widgetInsertCount++
		if perWidgetPreset, _ := op.Props["preset"].(string); perWidgetPreset != "" {
			return fmt.Sprintf("top-level preset %q cannot be combined with per-widget preset %q in props; pick one", presetName, perWidgetPreset)
		}
	}
	if widgetInsertCount > 1 {
		return fmt.Sprintf("top-level preset %q cannot be combined with %d widget insert ops; use per-widget preset in props for multi-widget composition", presetName, widgetInsertCount)
	}
	return ""
}

// ServiceToMap converts a Service to a flat map for data binding.
// Exported so usecases/handlers can prepare data for engine_v4.
func ServiceToMap(s domain.Service) map[string]interface{} {
	m := make(map[string]interface{})
	if s.Name != "" {
		m["name"] = s.Name
	}
	if s.Price > 0 {
		m["price"] = s.Price
	}
	if s.Rating > 0 {
		m["rating"] = s.Rating
	}
	if s.Description != "" {
		m["description"] = s.Description
	}
	if s.Duration != "" {
		m["duration"] = s.Duration
	}
	if s.Provider != "" {
		m["provider"] = s.Provider
	}
	if len(s.Images) > 0 {
		m["images"] = s.Images[0]
	}
	return m
}
