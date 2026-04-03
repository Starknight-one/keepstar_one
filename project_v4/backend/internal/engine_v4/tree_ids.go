package engine_v4

import (
	"fmt"

	"keepstar_v4/internal/domain"
)

// StampTreeIDs assigns stable, readable IDs to every atom and layout node.
// Idempotent: skips nodes that already have IDs.
func StampTreeIDs(formation *domain.FormationWithData) {
	if formation == nil {
		return
	}

	for si := range formation.Sections {
		for wi := range formation.Sections[si].Widgets {
			stampWidgetIDs(&formation.Sections[si].Widgets[wi], si, wi)
		}
	}

	for wi := range formation.Widgets {
		stampWidgetIDs(&formation.Widgets[wi], 0, wi)
	}
}

func stampWidgetIDs(w *domain.Widget, sectionIdx, widgetIdx int) {
	prefix := fmt.Sprintf("s%d-w%d", sectionIdx, widgetIdx)

	for i := range w.Atoms {
		if w.Atoms[i].ID != "" {
			continue
		}
		if w.Atoms[i].FieldName != "" {
			w.Atoms[i].ID = fmt.Sprintf("a-%s-%s", prefix, w.Atoms[i].FieldName)
		} else {
			w.Atoms[i].ID = fmt.Sprintf("a-%s-%d", prefix, i)
		}
	}

	if w.Layout != nil {
		nodeCounter := 0
		stampLayoutNodeIDs(w.Layout, prefix, &nodeCounter)
	}
}

func stampLayoutNodeIDs(node *domain.LayoutNode, prefix string, counter *int) {
	if node == nil {
		return
	}
	if node.ID == "" {
		if node.Name != "" {
			node.ID = fmt.Sprintf("n-%s-%s", prefix, node.Name)
		} else {
			node.ID = fmt.Sprintf("n-%s-%d", prefix, *counter)
		}
	}
	*counter++

	for i := range node.Children {
		if node.Children[i].Node != nil {
			stampLayoutNodeIDs(node.Children[i].Node, prefix, counter)
		}
	}
}

// BuildTreeMap creates a compact summary of the formation tree for Agent2 context.
func BuildTreeMap(formation *domain.FormationWithData) map[string]interface{} {
	if formation == nil {
		return nil
	}

	var allWidgets []domain.Widget
	allWidgets = append(allWidgets, formation.Widgets...)
	for _, s := range formation.Sections {
		allWidgets = append(allWidgets, s.Widgets...)
	}

	if len(allWidgets) == 0 {
		return nil
	}

	template := buildWidgetMap(allWidgets[0])

	result := map[string]interface{}{
		"widget_template": template,
		"widget_count":    len(allWidgets),
	}

	if formation.Grid != nil {
		result["grid"] = map[string]int{
			"cols": formation.Grid.Cols,
			"rows": formation.Grid.Rows,
		}
	}
	if formation.Mode != "" {
		result["mode"] = string(formation.Mode)
	}

	return result
}

func buildWidgetMap(w domain.Widget) map[string]interface{} {
	wm := map[string]interface{}{
		"id": w.ID,
	}

	var atoms []map[string]string
	for _, a := range w.Atoms {
		am := map[string]string{
			"id":   a.ID,
			"type": string(a.Type),
		}
		if a.FieldName != "" {
			am["field"] = a.FieldName
		}
		atoms = append(atoms, am)
	}
	if len(atoms) > 0 {
		wm["atoms"] = atoms
	}

	if w.Layout != nil {
		var nodes []map[string]string
		collectLayoutNodes(w.Layout, &nodes)
		if len(nodes) > 0 {
			wm["nodes"] = nodes
		}
	}

	return wm
}

func collectLayoutNodes(node *domain.LayoutNode, out *[]map[string]string) {
	if node == nil {
		return
	}
	nm := map[string]string{
		"id":   node.ID,
		"type": string(node.Type),
	}
	if node.Name != "" {
		nm["name"] = node.Name
	}
	*out = append(*out, nm)

	for _, ch := range node.Children {
		if ch.Node != nil {
			collectLayoutNodes(ch.Node, out)
		}
	}
}

// IsTreeID checks if a string is a specific tree ID (not a field/node name).
// Uses prefix check instead of length heuristic.
func IsTreeID(s string) bool {
	if len(s) < 3 {
		return false
	}
	return (s[0] == 'a' || s[0] == 'n' || s[0] == 'w') && s[1] == '-'
}
