package engine_v4

// System presets — literal-content widgets that do not bind to entity data.
// Agent2 is expected to override text values via ops after selecting the
// preset (e.g. {"op":"update","target":"body","props":{"value":"..."}}).
//
// Each system preset exposes refs $w and $root. Text atoms are tagged with
// fieldName= (empty) and value=placeholder so they can be targeted by a
// stable name (via the `ref` on the atom) in override ops.

// textExplainerOps — a single column widget with a title and a body paragraph.
// Used for "explain why X is better than Y" style answers. All values literal.
func textExplainerOps() []Op {
	return []Op{
		{Type: OpInsert, Ref: "w", Parent: "formation", Props: map[string]interface{}{"type": "widget", "size": "medium"}},
		{Type: OpInsert, Ref: "root", Parent: "$w", Props: map[string]interface{}{"type": "column", "gap": "md"}},
		{Type: OpInsert, Ref: "title", Parent: "$root", Props: map[string]interface{}{"type": "text", "value": "Explanation", "slot": "title", "textStyle": map[string]interface{}{"fontSize": "xl", "fontWeight": "bold"}}},
		{Type: OpInsert, Ref: "body", Parent: "$root", Props: map[string]interface{}{"type": "text", "value": "Body text goes here.", "slot": "description", "textStyle": map[string]interface{}{"fontSize": "md", "lineHeight": "relaxed"}}},
	}
}

// emptyNotFoundOps — empty state: icon + headline + subtext.
func emptyNotFoundOps() []Op {
	return []Op{
		{Type: OpInsert, Ref: "w", Parent: "formation", Props: map[string]interface{}{"type": "widget", "size": "medium"}},
		{Type: OpInsert, Ref: "root", Parent: "$w", Props: map[string]interface{}{"type": "column", "gap": "md", "align": "center"}},
		{Type: OpInsert, Parent: "$root", Props: map[string]interface{}{"type": "icon", "value": "search", "textStyle": map[string]interface{}{"fontSize": "3xl", "color": "muted"}}},
		{Type: OpInsert, Ref: "headline", Parent: "$root", Props: map[string]interface{}{"type": "text", "value": "Nothing found", "slot": "title", "textStyle": map[string]interface{}{"fontSize": "xl", "fontWeight": "semibold"}}},
		{Type: OpInsert, Ref: "subtext", Parent: "$root", Props: map[string]interface{}{"type": "text", "value": "Try a different query.", "slot": "description", "textStyle": map[string]interface{}{"color": "muted"}}},
	}
}

// errorGenericOps — error state: alert icon + headline + subtext.
func errorGenericOps() []Op {
	return []Op{
		{Type: OpInsert, Ref: "w", Parent: "formation", Props: map[string]interface{}{"type": "widget", "size": "medium"}},
		{Type: OpInsert, Ref: "root", Parent: "$w", Props: map[string]interface{}{"type": "column", "gap": "md", "align": "center"}},
		{Type: OpInsert, Parent: "$root", Props: map[string]interface{}{"type": "icon", "value": "alert", "textStyle": map[string]interface{}{"fontSize": "3xl", "color": "red"}}},
		{Type: OpInsert, Ref: "headline", Parent: "$root", Props: map[string]interface{}{"type": "text", "value": "Something went wrong", "slot": "title", "textStyle": map[string]interface{}{"fontSize": "xl", "fontWeight": "semibold"}}},
		{Type: OpInsert, Ref: "subtext", Parent: "$root", Props: map[string]interface{}{"type": "text", "value": "Please try again in a moment.", "slot": "description", "textStyle": map[string]interface{}{"color": "muted"}}},
	}
}

func init() {
	RegisterPreset(&Preset{
		Name:             "text_explainer",
		Description:      "Literal-text widget with a title and a body paragraph. Use for LLM explanations — override `title` and `body` via update ops.",
		Category:         "system",
		Refs:             []string{"w", "root", "title", "body"},
		DefaultReplicate: false,
		Build:            textExplainerOps,
	})
	RegisterPreset(&Preset{
		Name:             "empty_not_found",
		Description:      "Empty state — nothing found. Override `headline` and `subtext` for custom copy.",
		Category:         "system",
		Refs:             []string{"w", "root", "headline", "subtext"},
		DefaultReplicate: false,
		Build:            emptyNotFoundOps,
	})
	RegisterPreset(&Preset{
		Name:             "error_generic",
		Description:      "Generic error state (alert icon + headline + subtext). Override copy via update ops.",
		Category:         "system",
		Refs:             []string{"w", "root", "headline", "subtext"},
		DefaultReplicate: false,
		Build:            errorGenericOps,
	})
}
