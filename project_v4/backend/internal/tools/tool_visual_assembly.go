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
	statePort ports.StatePort
	engine    *engine_v4.Engine
}

// NewVisualAssemblyTool creates the V4 visual assembly tool.
func NewVisualAssemblyTool(statePort ports.StatePort, eng *engine_v4.Engine) *VisualAssemblyTool {
	return &VisualAssemblyTool{
		statePort: statePort,
		engine:    eng,
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
			"type": "object",
			"properties": map[string]interface{}{
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

	// Detect build-from-scratch: first op inserts a widget
	buildFromScratch := false
	if len(engineInput.Ops) > 0 {
		firstOp := engineInput.Ops[0]
		propType, _ := firstOp.Props["type"].(string)
		if propType == "widget" || firstOp.Parent == "formation" {
			buildFromScratch = true
		}
	}

	// Load existing formation for modification ops (not build-from-scratch)
	if !buildFromScratch && len(engineInput.Ops) > 0 {
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
func ProductToMap(p domain.Product) map[string]interface{} {
	m := make(map[string]interface{})
	if p.Name != "" {
		m["name"] = p.Name
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
	return m
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
