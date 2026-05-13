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
// Three call shapes (chunk 9), each carries an explicit `mode` (chunk 12):
//
//	1. preset name (± ops on top) — fast path; engine Materialises
//	   preset + components and the ops layer on top.
//	2. ops only — freestyle build (mode=rebuild) or modify on existing
//	   Document (mode=modify; ops target ids from the tree_map).
//	3. multi-widget compose — preset omitted, mode=rebuild, ops insert
//	   multiple top-level frames at parent="formation" / "root" / "".
//
// Both preset and ops absent → ToolResult IsError.
// `mode` missing or out of enum → ToolResult IsError.
func (t *VisualAssemblyTool) Definition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "visual_assembly",
		Description: "Build or modify a scene-graph Document. Always pass mode=rebuild|modify. Three call shapes: (1) preset name + optional ops — cheapest, use whenever a preset matches; (2) ops only — freestyle (rebuild) or tweaks on the current view (modify, target ids from tree_map); (3) multi-widget compose — rebuild, ops insert multiple top-level frames in one call (landings, presentations, hero + grid + cta). Engine runs Materialise (when preset present) → ApplyOps → ExpandReplicates → ResolveAndInline → BindData and writes the result to current.template. Reply only by calling this tool — never output text.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"mode": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"rebuild", "modify"},
					"description": "REQUIRED. \"rebuild\" — discard the previous Document and build fresh from preset/ops (use when the user asks for new content, a different layout, drill-down, or after a search). \"modify\" — load the current Document and apply ops as deltas targeting ids from tree_map (cosmetic / structural tweaks on what's on screen). When in doubt: «keep what's on screen but ...» = modify; «show me ...» / «render ...» = rebuild.",
				},
				"preset": map[string]interface{}{
					"type":        "string",
					"description": "Published preset name for this tenant. See the PRESETS section of the system prompt for the catalog. Optional — omit for freestyle / modify / multi-widget compose.",
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
			"required": []string{"mode"},
		},
	}
}

// Execute runs one tool call.
//
// `mode` (chunk 12) is REQUIRED — explicit per-turn decision:
//
//   - mode=rebuild + preset    → load preset + components → Materialise
//                                (ignores any current Document; fresh build)
//   - mode=rebuild + ops only  → start from engine.NewDocument()
//                                (freestyle / multi-widget compose)
//   - mode=modify  + ops       → load state.Current.Template; ops apply as
//                                deltas targeting tree_map ids
//
// Common pipeline (after the start-state chosen above):
//
//   ApplyOps → ExpandReplicates → ResolveAndInline → BindData →
//   InjectDefaultActions → marshal → UpdateTemplate
//
// Failure modes:
//   - mode missing / not in enum             → ToolResult IsError
//   - both preset and ops absent (rebuild)   → ToolResult IsError
//   - modify on empty Template               → ToolResult IsError
//                                              (LLM should pick rebuild)
//   - preset / component not found           → ToolResult IsError
//   - state retrieval / write fails          → returns Go error
//
// On success: ToolResult.Content reports a one-line summary including
// the call-shape mode for trace correlation.
func (t *VisualAssemblyTool) Execute(ctx context.Context, toolCtx domain.ToolContext, input map[string]interface{}) (*domain.ToolResult, error) {
	mode, _ := input["mode"].(string)
	if mode != "rebuild" && mode != "modify" {
		return errorResult("mode required: must be \"rebuild\" or \"modify\""), nil
	}

	presetName, _ := input["preset"].(string)
	replicate := readReplicate(input)

	parsedOps, err := parseOpsList(input["ops"])
	if err != nil {
		return errorResult(err.Error()), nil
	}

	if mode == "rebuild" && presetName == "" && len(parsedOps) == 0 {
		return errorResult("rebuild requires either `preset` or `ops` (or both)"), nil
	}
	if mode == "modify" && len(parsedOps) == 0 {
		return errorResult("modify requires `ops`"), nil
	}

	var (
		merged       *engine.Document
		resolveStats engine.ResolveStats
		isSystem     bool
	)

	// Load session state up front — both paths need it (catalog data;
	// modify-mode also needs the previous Document).
	state, err := t.state.GetState(ctx, toolCtx.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}
	data := productsToBindData(state.Current.Data.Products)

	switch {
	case mode == "rebuild" && presetName != "":
		// rebuild + preset — load preset + components, Materialise.
		preset, err := t.presets.GetPublishedPreset(ctx, toolCtx.TenantSlug, presetName)
		if err != nil {
			return errorResult(fmt.Sprintf("preset %q not found: %v", presetName, err)), nil
		}
		presetDoc, err := unmarshalDoc(preset.DocumentJSON)
		if err != nil {
			return nil, fmt.Errorf("unmarshal preset doc: %w", err)
		}
		isSystem = preset.IsSystem

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
		merged = engine.Materialise(presetDoc, componentDocs)

	case mode == "rebuild" && presetName == "":
		// rebuild + ops only — fresh empty Document, ops build the tree.
		merged = engine.NewDocument()

	case mode == "modify":
		// modify — load existing Document so ops target real ids.
		if len(state.Current.Template) == 0 {
			return errorResult("modify requires a current Document; pick mode=rebuild for the first turn"), nil
		}
		loaded, err := mapToDoc(state.Current.Template)
		if err != nil {
			return nil, fmt.Errorf("unmarshal current template: %w", err)
		}
		merged = loaded
	}

	slog.Info("visual_assembly",
		"mode", mode,
		"preset", presetName,
		"ops", len(parsedOps),
		"replicate", replicate,
		"system_preset", isSystem,
		"session_id", toolCtx.SessionID,
		"tenant", toolCtx.TenantSlug,
	)

	// 4. Apply ops BEFORE ExpandReplicates so the LLM addresses the
	// un-replicated tree (single set of stable ids); after replicate,
	// ids are minted fresh per clone and would diverge.
	if err := engine.ApplyOps(merged, parsedOps); err != nil {
		return errorResult(fmt.Sprintf("applyOps: %v", err)), nil
	}
	engine.ExpandReplicates(merged, replicate)

	// In freestyle mode the LLM may have inserted ref nodes pointing at
	// tenant components. Load components on demand to resolve them; in
	// preset mode Materialise already pulled them in.
	if presetName == "" && documentHasRefs(merged) {
		components, err := t.components.ListPublishedComponents(ctx, toolCtx.TenantSlug)
		if err != nil {
			return errorResult(fmt.Sprintf("list components: %v", err)), nil
		}
		// Append component children into merged.Variables for the resolver
		// to find — same shape Materialise produces.
		for _, c := range components {
			cd, err := unmarshalDoc(c.DocumentJSON)
			if err != nil {
				slog.Warn("visual_assembly: component doc unmarshal failed; skipping", "component", c.Name, "err", err)
				continue
			}
			// Append component roots into merged.Children with reusable=true
			// so resolver sees them as templates (not rendered output).
			for _, child := range cd.Children {
				child["reusable"] = true
				merged.Children = append(merged.Children, child)
			}
		}
	}
	resolveStats = engine.ResolveAndInline(merged)
	bindRes := engine.BindData(merged, data)

	// 4.6 Inject default user-actions (like + cart_add) into any empty
	// "actions" frame inside an entity-bound subtree. Skipped when the
	// data slice is empty (no entities to bind to). LLM-populated
	// actions frames pass through untouched (idempotent guard inside).
	injected := engine.InjectDefaultActions(merged, domain.EntityTypeProduct, data)

	// 5. Marshal the Document and write it to state.current.template.
	templateMap, err := docToMap(merged)
	if err != nil {
		return nil, fmt.Errorf("marshal document: %w", err)
	}
	// Stamp the active preset on the marshaled map so the pipeline
	// orchestrator can look up the matching drill-target preset for
	// the prefetch payload. Empty for freestyle / modify (no preset).
	if presetName != "" {
		templateMap[domain.TemplatePresetInUseKey] = presetName
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
		"OK mode=%s preset=%s system=%v replicate=%d resolved_refs=%d failed_refs=%d bound=%d missing=%d actions=%d",
		mode, presetName, isSystem, replicate, resolveStats.Resolved, len(resolveStats.Failed), len(bindRes.Bound), len(bindRes.Missing), injected,
	)
	return &domain.ToolResult{Content: summary}, nil
}

// documentHasRefs reports whether any node in the document tree is a Ref.
// Used in freestyle mode to decide whether to load components on demand.
func documentHasRefs(doc *engine.Document) bool {
	var walk func([]engine.Node) bool
	walk = func(nodes []engine.Node) bool {
		for _, n := range nodes {
			if engine.IsRef(n) {
				return true
			}
			if walk(engine.Children(n)) {
				return true
			}
		}
		return false
	}
	return walk(doc.Children)
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

// mapToDoc is the inverse of docToMap: parses the persisted template
// back into an engine.Document so modify-mode ops can target ids inside
// it. Used only on the no-preset path.
func mapToDoc(tpl map[string]interface{}) (*engine.Document, error) {
	raw, err := json.Marshal(tpl)
	if err != nil {
		return nil, err
	}
	var doc engine.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// errorResult is a convenience for input-shape / lookup errors that should
// land in the ToolResult (visible to the LLM next turn) rather than as a
// Go error (which would be a transport-layer failure).
func errorResult(msg string) *domain.ToolResult {
	return &domain.ToolResult{Content: msg, IsError: true}
}
