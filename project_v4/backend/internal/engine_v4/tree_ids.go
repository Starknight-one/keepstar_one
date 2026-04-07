package engine_v4

import (
	"fmt"
	"strings"

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
			w := &formation.Sections[si].Widgets[wi]
			if w.ID == "" || strings.HasPrefix(w.ID, "__pending_") {
				w.ID = fmt.Sprintf("w-s%d-w%d", si, wi)
			}
			stampWidgetIDs(w, si, wi)
		}
	}

	for wi := range formation.Widgets {
		w := &formation.Widgets[wi]
		if w.ID == "" || strings.HasPrefix(w.ID, "__pending_") {
			w.ID = fmt.Sprintf("w-s0-w%d", wi)
		}
		stampWidgetIDs(w, 0, wi)
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
//
// Schema:
//
//	{
//	  "widgets": [
//	    {"kind": "literal",    "id": "w-s0-w0",    "atoms": [...], "nodes": [...]},
//	    {"kind": "replicated", "count": 6, "ids": [...], "template": {...}},
//	    ...
//	  ],
//	  "widget_count": N,
//	  "mode": "composed" | "grid" | "single" | ...,
//	  "grid": {"cols": N, "rows": N}
//	}
//
// Each entry in `widgets` represents either a literal widget (hero, explainer,
// cta — no replication) or a replicated group (e.g. 6 product cards from one
// template). For replicated entries, modify ops can target the group by
// fieldName (broadcasts to every clone) or by a specific clone ID via `ids`.
//
// Walks formation.Sections first if populated, then formation.Widgets — covers
// both the multi-section composed path and the legacy flat path produced by
// Phase 3 single-section rollback.
func BuildTreeMap(formation *domain.FormationWithData) map[string]interface{} {
	if formation == nil {
		return nil
	}

	var allWidgets []domain.Widget
	if len(formation.Sections) > 0 {
		for _, s := range formation.Sections {
			allWidgets = append(allWidgets, s.Widgets...)
		}
	} else {
		allWidgets = append(allWidgets, formation.Widgets...)
	}

	if len(allWidgets) == 0 {
		return nil
	}

	// Group consecutive widgets by ReplicateConfig.GroupID. Literals (no GroupID)
	// each become their own group. This mirrors the grouping done by
	// groupIntoSections so the tree map and the rendered formation share the
	// same logical structure.
	type widgetGroup struct {
		gid     string
		widgets []domain.Widget
	}
	var groups []widgetGroup
	currentGid := "__init__"
	for _, w := range allWidgets {
		gid := ""
		if w.ReplicateConfig != nil {
			gid = w.ReplicateConfig.GroupID
		}
		if gid == "" {
			groups = append(groups, widgetGroup{gid: "", widgets: []domain.Widget{w}})
			currentGid = ""
			continue
		}
		if gid != currentGid {
			groups = append(groups, widgetGroup{gid: gid, widgets: []domain.Widget{w}})
			currentGid = gid
			continue
		}
		groups[len(groups)-1].widgets = append(groups[len(groups)-1].widgets, w)
	}

	widgets := make([]map[string]interface{}, 0, len(groups))
	for _, g := range groups {
		if g.gid == "" {
			// Literal widget — flatten buildWidgetMap fields into the entry.
			entry := map[string]interface{}{"kind": "literal"}
			for k, v := range buildWidgetMap(g.widgets[0]) {
				entry[k] = v
			}
			widgets = append(widgets, entry)
			continue
		}
		// Replicated group — collect IDs of every clone, expose first as template.
		ids := make([]string, len(g.widgets))
		for i, w := range g.widgets {
			ids[i] = w.ID
		}
		widgets = append(widgets, map[string]interface{}{
			"kind":     "replicated",
			"count":    len(g.widgets),
			"ids":      ids,
			"template": buildWidgetMap(g.widgets[0]),
		})
	}

	result := map[string]interface{}{
		"widgets":      widgets,
		"widget_count": len(allWidgets),
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
