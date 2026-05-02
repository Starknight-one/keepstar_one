package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine"
	"keepstar_v5/internal/ports"
)

// VisualAssemblyTool builds (or modifies) a scene-graph Document from a
// preset + replicate count + (eventually) a list of ops. It runs the V5
// engine pipeline and writes the resulting Document to
// state.Current.Template.
//
// Pipeline order, end to end:
//
//	preset, components := PresetPort.GetPublishedPreset + ListPublishedComponents
//	doc                := engine.Materialise(preset, components)
//	                      engine.ExpandReplicates(doc, count)
//	                      engine.ResolveAndInline(doc)
//	                      engine.BindData(doc, data)
//	StatePort.UpdateTemplate(sessionID, doc, deltaInfo)
type VisualAssemblyTool struct {
	state      ports.StatePort
	presets    ports.PresetPort
	components ports.ComponentPort
}

// NewVisualAssemblyTool constructs the tool with the three ports it needs.
// All ports are required.
func NewVisualAssemblyTool(state ports.StatePort, presets ports.PresetPort, components ports.ComponentPort) *VisualAssemblyTool {
	return &VisualAssemblyTool{
		state:      state,
		presets:    presets,
		components: components,
	}
}

// Compile-time check that VisualAssemblyTool satisfies Tool.
var _ Tool = (*VisualAssemblyTool)(nil)

// Definition returns the JSON-Schema the LLM sees. Schema is intentionally
// stable across runs — anything we change here busts the prompt cache.
//
// Chunk 6b note: `ops` is declared so the LLM can reason about the
// architecture, but the implementation does NOT apply ops yet. Chunk 6c
// will wire the engine.CommandHistory translation; until then, ops are
// logged and ignored. Keeping the schema declaration today means chunk 6c
// adds applier code only — no schema drift, no prompt rewrite.
func (t *VisualAssemblyTool) Definition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "visual_assembly",
		Description: "Build or modify a scene-graph Document. Pick a published preset by name, set replicate count for fan-out across data, optionally layer ops on top. Engine runs Materialise → ExpandReplicates → ResolveAndInline → BindData and writes the result to current.template. Reply only by calling this tool — never output text.",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"preset"},
			"properties": map[string]interface{}{
				"preset": map[string]interface{}{
					"type":        "string",
					"description": "Published preset name for this tenant. See the PRESETS section of the system prompt for the catalog. Required.",
				},
				"replicate": map[string]interface{}{
					"type":        "integer",
					"description": "How many clones to fan out for the replicate-marked subtree (one per data record from state.current.data). 0 or omitted = no fan-out (single instance). Use the count of products you want to show.",
				},
				"ops": map[string]interface{}{
					"type":        "array",
					"description": "Operations to layer on top of the preset (insert/update/delete/move/override). Target nodes by id from the tree_map. Each op object: {op, target?, parent?, ref?, props?}. Optional — leave empty for preset-only rendering.",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"op":     map[string]interface{}{"type": "string", "enum": []string{"insert", "update", "delete", "move", "override"}},
							"target": map[string]interface{}{"type": "string", "description": "Node id to operate on (from tree_map.instances[].atoms[].id)."},
							"parent": map[string]interface{}{"type": "string", "description": "Parent id for insert / move ops."},
							"ref":    map[string]interface{}{"type": "string", "description": "Local binding name for chaining ops (referenced as $ref in subsequent ops)."},
							"props":  map[string]interface{}{"type": "object", "description": "Properties to set on the node. Merged in update ops. Common keys: type, fieldBinding, content, format, wrapper, textStyle, layout, slot."},
						},
						"required": []string{"op"},
					},
				},
			},
		},
	}
}

// Execute runs one tool call.
//
// Failure modes:
//   - missing/invalid `preset`            → ToolResult{IsError:true, Content: "..."}
//   - preset / component not found in DB  → ToolResult{IsError:true, Content: "..."}
//   - state retrieval / write fails       → returns Go error (transport-layer)
//
// On success: ToolResult.Content reports a one-line summary
// (preset name, replicate count, bind diagnostics counts).
func (t *VisualAssemblyTool) Execute(ctx context.Context, toolCtx domain.ToolContext, input map[string]interface{}) (*domain.ToolResult, error) {
	presetName, ok := input["preset"].(string)
	if !ok || presetName == "" {
		return errorResult("preset is required (string)"), nil
	}

	replicate := readReplicate(input)

	// Parse ops upfront so we can run them after Materialise but BEFORE
	// ExpandReplicates — see the pipeline ordering comment below.
	parsedOps, err := parseOpsList(input["ops"])
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// 1. Load preset.
	preset, err := t.presets.GetPublishedPreset(ctx, toolCtx.TenantSlug, presetName)
	if err != nil {
		return errorResult(fmt.Sprintf("preset %q not found: %v", presetName, err)), nil
	}
	presetDoc, err := unmarshalDoc(preset.DocumentJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal preset doc: %w", err)
	}

	// 2. Load all published components for the tenant. Materialise picks
	// the ones the preset references by id; extras are harmless.
	components, err := t.components.ListPublishedComponents(ctx, toolCtx.TenantSlug)
	if err != nil {
		return errorResult(fmt.Sprintf("list components: %v", err)), nil
	}
	componentDocs := make([]*engine.Document, 0, len(components))
	for _, c := range components {
		cd, err := unmarshalDoc(c.DocumentJSON)
		if err != nil {
			slog.Warn("visual_assembly: component doc unmarshal failed; skipping", "component", c.Name, "err", err)
			continue
		}
		componentDocs = append(componentDocs, cd)
	}

	// 3. Load session state for catalog data.
	state, err := t.state.GetState(ctx, toolCtx.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}
	data := productsToBindData(state.Current.Data.Products)

	// 4. Run the V5 engine pipeline.
	//
	// Pipeline order matters:
	//   Materialise(preset, components)  — pulls preset + reusable defs
	//   ApplyOps(merged, ops)            — LLM tweaks BEFORE replication
	//                                       so the same edit lands on
	//                                       every clone (V4 broadcast).
	//   ExpandReplicates(merged, count)  — fan-out + reID + dataIndex
	//   ResolveAndInline(merged)         — Ref → resolved subtree
	//   BindData(merged, data)           — fill atoms from data records
	//
	// Putting ApplyOps before ExpandReplicates means the LLM addresses
	// the un-replicated tree (single set of stable ids); after replicate,
	// ids are minted fresh per clone and would diverge.
	merged := engine.Materialise(presetDoc, componentDocs)
	if err := engine.ApplyOps(merged, parsedOps); err != nil {
		return errorResult(fmt.Sprintf("applyOps: %v", err)), nil
	}
	engine.ExpandReplicates(merged, replicate)
	resolveStats := engine.ResolveAndInline(merged)
	bindRes := engine.BindData(merged, data)

	// 5. Marshal the Document and write it to state.current.template.
	templateMap, err := docToMap(merged)
	if err != nil {
		return nil, fmt.Errorf("marshal document: %w", err)
	}
	deltaInfo := domain.DeltaInfo{
		TurnID:    toolCtx.TurnID,
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   "agent2",
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "current.template",
		Action:    domain.Action{Tool: "visual_assembly", Params: input},
	}
	if _, err := t.state.UpdateTemplate(ctx, toolCtx.SessionID, templateMap, deltaInfo); err != nil {
		return nil, fmt.Errorf("update template: %w", err)
	}

	summary := fmt.Sprintf(
		"OK preset=%s replicate=%d resolved_refs=%d failed_refs=%d bound=%d missing=%d",
		presetName, replicate, resolveStats.Resolved, len(resolveStats.Failed), len(bindRes.Bound), len(bindRes.Missing),
	)
	return &domain.ToolResult{Content: summary}, nil
}

// parseOpsList coerces the raw `ops` input value into the
// []map[string]any shape engine.ApplyOps expects. Tolerates the two JSON
// shapes the LLM might emit:
//
//   - []interface{} of map[string]interface{}  (json.Unmarshal default)
//   - []map[string]interface{}                  (already-typed)
//
// nil / missing → nil (no ops). Anything malformed → typed error so the
// caller surfaces it as a ToolResult IsError.
func parseOpsList(raw any) ([]map[string]any, error) {
	if raw == nil {
		return nil, nil
	}
	switch v := raw.(type) {
	case []map[string]any:
		return v, nil
	case []interface{}:
		out := make([]map[string]any, 0, len(v))
		for i, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("ops[%d]: expected object, got %T", i, item)
			}
			out = append(out, m)
		}
		return out, nil
	}
	return nil, fmt.Errorf("ops: expected array, got %T", raw)
}

// readReplicate accepts integer-shaped values from the LLM tool_use input.
// JSON numbers come through as float64 by default; tolerate that + ints.
// Negative or absurd values clamp to 0 (no fan-out) — let the engine
// produce a single instance rather than crashing.
func readReplicate(input map[string]interface{}) int {
	raw, ok := input["replicate"]
	if !ok {
		return 0
	}
	switch v := raw.(type) {
	case int:
		if v < 0 {
			return 0
		}
		return v
	case int64:
		if v < 0 {
			return 0
		}
		return int(v)
	case float64:
		if v < 0 {
			return 0
		}
		return int(v)
	}
	return 0
}

// productsToBindData flattens the products zone of state.current.data into
// the []map[string]any shape engine.BindData expects.
func productsToBindData(products []domain.Product) []map[string]any {
	out := make([]map[string]any, len(products))
	for i, p := range products {
		out[i] = engine.ProductToMap(p)
	}
	return out
}

// unmarshalDoc parses a domain.Preset.DocumentJSON / domain.Component.DocumentJSON
// (raw bytes, kept that way to keep domain layer engine-free) into an
// engine.Document.
func unmarshalDoc(raw json.RawMessage) (*engine.Document, error) {
	var doc engine.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// docToMap round-trips an engine.Document through JSON into the
// map[string]interface{} shape state.current.template stores.
func docToMap(doc *engine.Document) (map[string]interface{}, error) {
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

// errorResult is a convenience for input-shape / lookup errors that should
// land in the ToolResult (visible to the LLM next turn) rather than as a
// Go error (which would be a transport-layer failure).
func errorResult(msg string) *domain.ToolResult {
	return &domain.ToolResult{Content: msg, IsError: true}
}
