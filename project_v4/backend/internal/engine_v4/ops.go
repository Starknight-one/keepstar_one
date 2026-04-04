package engine_v4

import (
	"encoding/json"
	"fmt"

	"keepstar_v4/internal/domain"
)

// ParseOps parses raw JSON ops array into typed Op slice.
func ParseOps(raw []interface{}) ([]Op, error) {
	var ops []Op
	for i, item := range raw {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("op[%d]: expected object, got %T", i, item)
		}

		opType, _ := itemMap["op"].(string)
		target, _ := itemMap["target"].(string)
		ref, _ := itemMap["ref"].(string)
		parent, _ := itemMap["parent"].(string)
		after, _ := itemMap["after"].(string)
		props, _ := itemMap["props"].(map[string]interface{})

		if opType == "" {
			return nil, fmt.Errorf("op[%d]: missing 'op' field", i)
		}
		// insert requires parent (not target), update/delete/move require target
		if opType == "insert" {
			if parent == "" {
				return nil, fmt.Errorf("op[%d]: insert requires 'parent' field", i)
			}
		} else {
			if target == "" {
				return nil, fmt.Errorf("op[%d]: missing 'target' field", i)
			}
		}

		ops = append(ops, Op{
			Type:   OpType(opType),
			Target: target,
			Ref:    ref,
			Parent: parent,
			After:  after,
			Props:  props,
		})
	}
	return ops, nil
}

// atomRef is a pointer to an atom within a widget, used for the ID index.
type atomRef struct {
	widget    *domain.Widget
	atomIndex int
}

// nodeRef is a pointer to a layout node and its parent context.
type nodeRef struct {
	node   *domain.LayoutNode
	parent *domain.LayoutNode // nil for root
	widget *domain.Widget     // owning widget (for dedup tracking)
}

// idIndex maps IDs to their targets in the formation tree.
type idIndex struct {
	atoms     map[string]atomRef
	nodes     map[string]nodeRef
	widgets   map[string]*domain.Widget
	formation *domain.FormationWithData
}

// buildIndex walks the formation and creates an ID -> target mapping.
func buildIndex(formation *domain.FormationWithData) idIndex {
	idx := idIndex{
		atoms:     make(map[string]atomRef),
		nodes:     make(map[string]nodeRef),
		widgets:   make(map[string]*domain.Widget),
		formation: formation,
	}

	indexWidgets := func(widgets []domain.Widget) {
		for wi := range widgets {
			w := &widgets[wi]
			if w.ID != "" {
				idx.widgets[w.ID] = w
			}
			for ai := range w.Atoms {
				if w.Atoms[ai].ID != "" {
					idx.atoms[w.Atoms[ai].ID] = atomRef{widget: w, atomIndex: ai}
				}
			}
			if w.Layout != nil {
				indexLayoutNode(w.Layout, nil, w, &idx)
			}
		}
	}

	indexWidgets(formation.Widgets)
	for si := range formation.Sections {
		indexWidgets(formation.Sections[si].Widgets)
	}

	return idx
}

func indexLayoutNode(node *domain.LayoutNode, parent *domain.LayoutNode, w *domain.Widget, idx *idIndex) {
	if node.ID != "" {
		idx.nodes[node.ID] = nodeRef{node: node, parent: parent, widget: w}
	}
	for i := range node.Children {
		if node.Children[i].Node != nil {
			indexLayoutNode(node.Children[i].Node, node, w, idx)
		}
	}
}

// ApplyOps applies a batch of operations to an existing formation.
// Returns warnings only — the engine pipeline handles errors.
func ApplyOps(formation *domain.FormationWithData, ops []Op) []string {
	if formation == nil {
		return []string{"no existing formation to modify"}
	}

	idx := buildIndex(formation)
	refs := make(map[string]string) // ref alias → assigned ID (for insert chaining)
	var warnings []string

	for i, op := range ops {
		// Resolve ref aliases in target/parent/after fields
		op = resolveRefs(op, refs)

		switch op.Type {
		case OpUpdate:
			w := applyUpdate(op, &idx)
			if w != "" {
				warnings = append(warnings, fmt.Sprintf("op[%d]: %s", i, w))
			}
		case OpDelete:
			w := applyDelete(op, &idx)
			if w != "" {
				warnings = append(warnings, fmt.Sprintf("op[%d]: %s", i, w))
			}
		case OpInsert:
			id, w := applyInsert(op, &idx, formation)
			if w != "" {
				warnings = append(warnings, fmt.Sprintf("op[%d]: %s", i, w))
			}
			if op.Ref != "" && id != "" {
				refs[op.Ref] = id
			}
		case OpMove:
			w := applyMove(op, &idx)
			if w != "" {
				warnings = append(warnings, fmt.Sprintf("op[%d]: %s", i, w))
			}
		default:
			warnings = append(warnings, fmt.Sprintf("op[%d]: unknown op type %q", i, op.Type))
		}
	}

	return warnings
}

// resolveRefs replaces ref aliases ($refName) in target/parent/after with real IDs.
func resolveRefs(op Op, refs map[string]string) Op {
	resolve := func(s string) string {
		if len(s) > 1 && s[0] == '$' {
			if id, ok := refs[s[1:]]; ok {
				return id
			}
		}
		return s
	}
	op.Target = resolve(op.Target)
	op.Parent = resolve(op.Parent)
	op.After = resolve(op.After)
	return op
}

// applyUpdate merges props into the target atom or layout node.
func applyUpdate(op Op, idx *idIndex) string {
	// Try atom first
	if ref, ok := idx.atoms[op.Target]; ok {
		atom := &ref.widget.Atoms[ref.atomIndex]
		mergeAtomProps(atom, op.Props)
		// Auto-lock: agent explicitly modified this atom
		if atom.Rigidity != domain.RigidityLocked {
			atom.Rigidity = domain.RigidityLocked
		}
		return ""
	}

	// Try layout node
	if ref, ok := idx.nodes[op.Target]; ok {
		mergeNodeProps(ref.node, op.Props)
		return ""
	}

	// Try widget
	if w, ok := idx.widgets[op.Target]; ok {
		mergeWidgetProps(w, op.Props)
		return ""
	}

	// Formation-level target: columns, gap, mode
	if op.Target == "formation" || op.Target == "grid" {
		if idx.formation != nil {
			mergeFormationProps(idx.formation, op.Props)
		}
		return ""
	}

	return fmt.Sprintf("target %q not found", op.Target)
}

// applyDelete removes an atom or layout node from the tree.
func applyDelete(op Op, idx *idIndex) string {
	// Try atom
	if ref, ok := idx.atoms[op.Target]; ok {
		w := ref.widget
		atomIdx := ref.atomIndex

		// Remove from Atoms
		w.Atoms = append(w.Atoms[:atomIdx], w.Atoms[atomIdx+1:]...)

		// Remove matching LayoutChild and fix remaining atomIndex references
		if w.Layout != nil {
			removeAtomFromLayout(w.Layout, atomIdx)
			fixAtomIndices(w.Layout, atomIdx)
		}

		delete(idx.atoms, op.Target)
		return ""
	}

	// Try layout node
	if ref, ok := idx.nodes[op.Target]; ok {
		if ref.parent != nil {
			removeChildNode(ref.parent, ref.node)
		}
		delete(idx.nodes, op.Target)
		return ""
	}

	return fmt.Sprintf("target %q not found", op.Target)
}

// applyMove moves an atom or layout node to a new parent/position.
func applyMove(op Op, idx *idIndex) string {
	if op.Parent == "" {
		return "move requires 'parent' field"
	}

	// Try atom
	if ref, ok := idx.atoms[op.Target]; ok {
		w := ref.widget
		atomIdx := ref.atomIndex

		// Remove from current layout position
		if w.Layout != nil {
			removeAtomFromLayout(w.Layout, atomIdx)
			fixAtomIndices(w.Layout, atomIdx)
		}

		// Find target parent node
		var parentNode *domain.LayoutNode
		if nRef, ok := idx.nodes[op.Parent]; ok {
			parentNode = nRef.node
		} else if pw, ok := idx.widgets[op.Parent]; ok && pw.Layout != nil {
			parentNode = pw.Layout
		}
		if parentNode == nil {
			return fmt.Sprintf("move parent %q not found", op.Parent)
		}

		// Re-insert into new parent
		child := domain.NewAtomChild(atomIdx)
		insertChildIntoNode(parentNode, child, op.After, idx)
		return ""
	}

	// Try layout node
	if ref, ok := idx.nodes[op.Target]; ok {
		// Remove from current parent
		if ref.parent != nil {
			removeChildNode(ref.parent, ref.node)
		}

		// Find new parent
		var parentNode *domain.LayoutNode
		if nRef, ok := idx.nodes[op.Parent]; ok {
			parentNode = nRef.node
		} else if pw, ok := idx.widgets[op.Parent]; ok && pw.Layout != nil {
			parentNode = pw.Layout
		}
		if parentNode == nil {
			return fmt.Sprintf("move parent %q not found", op.Parent)
		}

		child := domain.NewNodeChild(ref.node)
		insertChildIntoNode(parentNode, child, op.After, idx)

		// Update parent reference in index
		idx.nodes[op.Target] = nodeRef{node: ref.node, parent: parentNode, widget: ref.widget}
		return ""
	}

	return fmt.Sprintf("target %q not found", op.Target)
}

// applyInsert creates a new atom or layout node and adds it to a parent.
// Returns the new element's ID (assigned by StampTreeIDs later) and a warning.
func applyInsert(op Op, idx *idIndex, formation *domain.FormationWithData) (string, string) {
	if op.Props == nil {
		return "", "insert requires props"
	}

	propType, _ := op.Props["type"].(string)

	// Widget insert: type:"widget" or parent:"formation"
	if propType == "widget" || op.Parent == "formation" {
		return insertWidget(op, idx, formation)
	}

	// Layout node insert
	if propType == "row" || propType == "column" || propType == "flow" || propType == "span" {
		return insertLayoutNode(op, idx)
	}

	// Atom insert
	return insertAtom(op, idx, formation)
}

// insertAtom creates a new Atom and appends it to the parent widget.
func insertAtom(op Op, idx *idIndex, formation *domain.FormationWithData) (string, string) {
	// Build atom from props
	atom := parseAtomFromProps(op.Props)
	atom.Rigidity = domain.RigidityLocked // freestyle atoms are locked

	// Find parent widget — either by widget ID or by finding the widget that owns a layout node
	var targetWidget *domain.Widget
	if w, ok := idx.widgets[op.Parent]; ok {
		targetWidget = w
	} else if ref, ok := idx.nodes[op.Parent]; ok {
		// Find widget that owns this layout node by walking formation
		targetWidget = findWidgetForNode(formation, ref.node)
	}

	if targetWidget == nil {
		// Default: first widget
		if len(formation.Widgets) > 0 {
			targetWidget = &formation.Widgets[0]
		} else if len(formation.Sections) > 0 && len(formation.Sections[0].Widgets) > 0 {
			targetWidget = &formation.Sections[0].Widgets[0]
		}
	}
	if targetWidget == nil {
		return "", fmt.Sprintf("parent %q not found for insert", op.Parent)
	}

	// Append atom
	newIdx := len(targetWidget.Atoms)
	targetWidget.Atoms = append(targetWidget.Atoms, atom)

	// Add to layout tree
	if targetWidget.Layout != nil {
		child := domain.NewAtomChild(newIdx)
		if parentNode, ok := idx.nodes[op.Parent]; ok {
			insertChildIntoNode(parentNode.node, child, op.After, idx)
		} else {
			// Append to root layout
			insertChildIntoNode(targetWidget.Layout, child, op.After, idx)
		}
	}

	// Re-index this widget's atoms (indices changed)
	reindexWidget(targetWidget, idx)

	// ID will be assigned by StampTreeIDs in the engine pipeline
	return fmt.Sprintf("__pending_atom_%d", newIdx), ""
}

// insertLayoutNode creates a new LayoutNode and inserts it into a parent node.
func insertLayoutNode(op Op, idx *idIndex) (string, string) {
	parentRef, ok := idx.nodes[op.Parent]
	if !ok {
		// Try widget — insert as child of root layout
		if w, ok := idx.widgets[op.Parent]; ok && w.Layout != nil {
			parentRef = nodeRef{node: w.Layout, widget: w}
		} else {
			return "", fmt.Sprintf("parent node %q not found for insert", op.Parent)
		}
	}

	node := parseLayoutNodeFromProps(op.Props)
	child := domain.NewNodeChild(node)
	insertChildIntoNode(parentRef.node, child, op.After, idx)

	// Register pending ID so subsequent ops can reference via $ref chaining
	pendingID := fmt.Sprintf("__pending_node_%p", node)
	idx.nodes[pendingID] = nodeRef{node: node, parent: parentRef.node, widget: parentRef.widget}

	// Also index by real ID if present
	if node.ID != "" {
		idx.nodes[node.ID] = nodeRef{node: node, parent: parentRef.node, widget: parentRef.widget}
	}

	return pendingID, ""
}

// insertWidget creates a new empty Widget and adds it to the formation.
// Subsequent ops can insert atoms/nodes into this widget via $ref chaining.
func insertWidget(op Op, idx *idIndex, formation *domain.FormationWithData) (string, string) {
	size := domain.WidgetSizeMedium
	if v, ok := op.Props["size"].(string); ok && v != "" {
		size = domain.WidgetSize(v)
	}

	w := domain.Widget{
		Size:   size,
		Layout: &domain.LayoutNode{Type: domain.LayoutNodeColumn, Name: "root", Children: []domain.LayoutChild{}},
		Atoms:  []domain.Atom{},
	}

	formation.Widgets = append(formation.Widgets, w)
	ptr := &formation.Widgets[len(formation.Widgets)-1]

	tempID := fmt.Sprintf("__pending_widget_%d", len(formation.Widgets)-1)
	ptr.ID = tempID

	idx.widgets[tempID] = ptr
	// Also index root layout node under widget's ID so $ref chaining works:
	// parent:"$w" resolves to tempID → insertLayoutNode finds it in idx.nodes → uses widget.Layout
	idx.nodes[tempID] = nodeRef{node: ptr.Layout, parent: nil, widget: ptr}

	return tempID, ""
}

// insertChildIntoNode inserts a LayoutChild into a node, optionally after a specific ID.
func insertChildIntoNode(parent *domain.LayoutNode, child domain.LayoutChild, afterID string, idx *idIndex) {
	if afterID == "" {
		parent.Children = append(parent.Children, child)
		return
	}

	// Find position of afterID
	insertPos := -1
	for i, ch := range parent.Children {
		if ch.AtomIndex != nil {
			// Check if this atom has the afterID
			for _, ref := range idx.atoms {
				if ref.atomIndex == *ch.AtomIndex && ref.widget != nil {
					if ref.widget.Atoms[ref.atomIndex].ID == afterID {
						insertPos = i + 1
						break
					}
				}
			}
		}
		if ch.Node != nil && ch.Node.ID == afterID {
			insertPos = i + 1
		}
		if insertPos >= 0 {
			break
		}
	}

	if insertPos < 0 {
		// afterID not found in this node — append
		parent.Children = append(parent.Children, child)
		return
	}

	// Insert at position
	parent.Children = append(parent.Children, domain.LayoutChild{})
	copy(parent.Children[insertPos+1:], parent.Children[insertPos:])
	parent.Children[insertPos] = child
}

// parseAtomFromProps creates an Atom from raw props map.
func parseAtomFromProps(props map[string]interface{}) domain.Atom {
	atom := domain.Atom{}

	if v, ok := props["type"].(string); ok {
		atom.Type = domain.AtomType(v)
	}
	if atom.Type == "" {
		atom.Type = domain.AtomTypeText
	}
	if v, ok := props["subtype"].(string); ok {
		atom.Subtype = domain.AtomSubtype(v)
	}
	if v, ok := props["value"]; ok {
		atom.Value = v
	}
	if v, ok := props["label"].(string); ok {
		atom.Label = v
	}
	if v, ok := props["format"].(string); ok {
		atom.Format = domain.AtomFormat(v)
	}
	if v, ok := props["slot"].(string); ok {
		atom.Slot = domain.AtomSlot(v)
	}
	if v, ok := props["fieldName"].(string); ok {
		atom.FieldName = v
	}

	// Styled props
	if tsRaw, ok := props["textStyle"]; ok {
		atom.TextStyle = parseTextStyle(tsRaw)
	}
	if wRaw, ok := props["wrapper"]; ok {
		atom.Wrapper = parseWrapper(wRaw)
	}
	if msRaw, ok := props["mediaStyle"]; ok {
		atom.MediaStyle = parseMediaStyle(msRaw)
	}
	if isRaw, ok := props["iconStyle"]; ok {
		atom.IconStyle = parseIconStyle(isRaw)
	}

	return atom
}

// parseLayoutNodeFromProps creates a LayoutNode from raw props.
func parseLayoutNodeFromProps(props map[string]interface{}) *domain.LayoutNode {
	node := &domain.LayoutNode{}
	if v, ok := props["type"].(string); ok {
		node.Type = domain.LayoutNodeType(v)
	}
	if v, ok := props["name"].(string); ok {
		node.Name = v
	}
	if v, ok := props["gap"].(string); ok {
		node.Gap = v
	}
	if v, ok := props["align"].(string); ok {
		node.Align = v
	}
	if v, ok := props["distribution"].(string); ok {
		node.Distribution = v
	}
	if v, ok := props["padding"].(string); ok {
		node.Padding = v
	}
	if v, ok := props["background"].(string); ok {
		node.Background = v
	}
	if v, ok := props["borderRadius"].(string); ok {
		node.BorderRadius = v
	}
	if v, ok := props["border"].(string); ok {
		node.Border = v
	}
	return node
}

// findWidgetForNode finds which widget owns a given layout node.
func findWidgetForNode(formation *domain.FormationWithData, target *domain.LayoutNode) *domain.Widget {
	for i := range formation.Widgets {
		if formation.Widgets[i].Layout != nil && nodeInTree(formation.Widgets[i].Layout, target) {
			return &formation.Widgets[i]
		}
	}
	for si := range formation.Sections {
		for wi := range formation.Sections[si].Widgets {
			if formation.Sections[si].Widgets[wi].Layout != nil && nodeInTree(formation.Sections[si].Widgets[wi].Layout, target) {
				return &formation.Sections[si].Widgets[wi]
			}
		}
	}
	return nil
}

func nodeInTree(root *domain.LayoutNode, target *domain.LayoutNode) bool {
	if root == target {
		return true
	}
	for _, ch := range root.Children {
		if ch.Node != nil && nodeInTree(ch.Node, target) {
			return true
		}
	}
	return false
}

// reindexWidget rebuilds the atom index for a widget after insert.
func reindexWidget(w *domain.Widget, idx *idIndex) {
	// Remove old atom refs for this widget
	for id, ref := range idx.atoms {
		if ref.widget == w {
			delete(idx.atoms, id)
		}
	}
	// Re-add
	for ai := range w.Atoms {
		if w.Atoms[ai].ID != "" {
			idx.atoms[w.Atoms[ai].ID] = atomRef{widget: w, atomIndex: ai}
		}
	}
}

// removeAtomFromLayout removes a LayoutChild that references the given atom index.
func removeAtomFromLayout(node *domain.LayoutNode, atomIdx int) {
	filtered := node.Children[:0]
	for _, ch := range node.Children {
		if ch.AtomIndex != nil && *ch.AtomIndex == atomIdx {
			continue // Remove this child
		}
		if ch.Node != nil {
			removeAtomFromLayout(ch.Node, atomIdx)
		}
		filtered = append(filtered, ch)
	}
	node.Children = filtered

	// Clean up empty nested nodes
	cleaned := node.Children[:0]
	for _, ch := range node.Children {
		if ch.Node != nil && len(ch.Node.Children) == 0 {
			continue // Remove empty group
		}
		cleaned = append(cleaned, ch)
	}
	node.Children = cleaned
}

// fixAtomIndices decrements all atomIndex values > deletedIdx after an atom removal.
func fixAtomIndices(node *domain.LayoutNode, deletedIdx int) {
	for i := range node.Children {
		if node.Children[i].AtomIndex != nil && *node.Children[i].AtomIndex > deletedIdx {
			newIdx := *node.Children[i].AtomIndex - 1
			node.Children[i].AtomIndex = &newIdx
		}
		if node.Children[i].Node != nil {
			fixAtomIndices(node.Children[i].Node, deletedIdx)
		}
	}
}

// removeChildNode removes a specific node from its parent's children.
func removeChildNode(parent *domain.LayoutNode, target *domain.LayoutNode) {
	filtered := parent.Children[:0]
	for _, ch := range parent.Children {
		if ch.Node == target {
			continue
		}
		filtered = append(filtered, ch)
	}
	parent.Children = filtered
}

// mergeAtomProps merges a raw props map into an Atom.
func mergeAtomProps(atom *domain.Atom, props map[string]interface{}) {
	if props == nil {
		return
	}

	if v, ok := props["value"]; ok {
		atom.Value = v
	}
	if v, ok := props["format"].(string); ok && v != "" {
		atom.Format = domain.AtomFormat(v)
	}
	if v, ok := props["label"].(string); ok {
		atom.Label = v
	}

	// Merge textStyle
	if tsRaw, ok := props["textStyle"]; ok {
		ts := parseTextStyle(tsRaw)
		if ts != nil {
			if atom.TextStyle == nil {
				atom.TextStyle = ts
			} else {
				mergeTextStyle(atom.TextStyle, ts)
			}
		}
	}

	// Shorthand safety net: LLM may send color/fontSize/fontWeight at top level
	if lifted := liftTextStyleShorthands(props); lifted != nil {
		if atom.TextStyle == nil {
			atom.TextStyle = lifted
		} else {
			mergeTextStyle(atom.TextStyle, lifted)
		}
	}

	// Replace wrapper
	if wRaw, ok := props["wrapper"]; ok {
		w := parseWrapper(wRaw)
		atom.Wrapper = w
	}

	// Replace mediaStyle
	if msRaw, ok := props["mediaStyle"]; ok {
		ms := parseMediaStyle(msRaw)
		atom.MediaStyle = ms
	}

	// Replace iconStyle
	if isRaw, ok := props["iconStyle"]; ok {
		is := parseIconStyle(isRaw)
		atom.IconStyle = is
	}
}

// mergeNodeProps merges props into a LayoutNode's visual properties.
func mergeNodeProps(node *domain.LayoutNode, props map[string]interface{}) {
	if props == nil {
		return
	}
	if v, ok := props["gap"].(string); ok {
		node.Gap = v
	}
	if v, ok := props["align"].(string); ok {
		node.Align = v
	}
	if v, ok := props["distribution"].(string); ok {
		node.Distribution = v
	}
	if v, ok := props["padding"].(string); ok {
		node.Padding = v
	}
	if v, ok := props["background"].(string); ok {
		node.Background = v
	}
	if v, ok := props["borderRadius"].(string); ok {
		node.BorderRadius = v
	}
	if v, ok := props["shadow"].(string); ok {
		node.Shadow = v
	}
	if v, ok := props["border"].(string); ok {
		node.Border = v
	}
}

// mergeWidgetProps merges props into a Widget.
func mergeWidgetProps(w *domain.Widget, props map[string]interface{}) {
	if props == nil {
		return
	}
	if v, ok := props["size"].(string); ok && v != "" {
		w.Size = domain.WidgetSize(v)
	}
}

// mergeFormationProps merges props into a FormationWithData (columns, gap, mode).
func mergeFormationProps(f *domain.FormationWithData, props map[string]interface{}) {
	if props == nil || f == nil {
		return
	}
	if v, ok := props["columns"].(float64); ok && v > 0 {
		if f.Grid == nil {
			f.Grid = &domain.GridConfig{}
		}
		f.Grid.Cols = int(v)
	}
	if v, ok := props["rows"].(float64); ok && v > 0 {
		if f.Grid == nil {
			f.Grid = &domain.GridConfig{}
		}
		f.Grid.Rows = int(v)
	}
	if v, ok := props["mode"].(string); ok && v != "" {
		f.Mode = domain.FormationType(v)
	}
}

// mergeTextStyle merges src into dst (src overrides non-zero fields in dst).
func mergeTextStyle(dst, src *domain.TextStyle) {
	if src.FontSize != "" {
		dst.FontSize = src.FontSize
	}
	if src.FontWeight != "" {
		dst.FontWeight = src.FontWeight
	}
	if src.Color != "" {
		dst.Color = src.Color
	}
	if src.TextDecoration != "" {
		dst.TextDecoration = src.TextDecoration
	}
	if src.TextTransform != "" {
		dst.TextTransform = src.TextTransform
	}
	if src.LineClamp > 0 {
		dst.LineClamp = src.LineClamp
	}
	if src.Truncate > 0 {
		dst.Truncate = src.Truncate
	}
	if src.LineHeight != "" {
		dst.LineHeight = src.LineHeight
	}
	if src.LetterSpacing != "" {
		dst.LetterSpacing = src.LetterSpacing
	}
}

// Parsing helpers — convert raw map[string]interface{} to typed structs via JSON roundtrip.

func parseTextStyle(raw interface{}) *domain.TextStyle {
	if raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var ts domain.TextStyle
	if err := json.Unmarshal(b, &ts); err != nil {
		return nil
	}
	return &ts
}

func parseWrapper(raw interface{}) *domain.WrapperConfig {
	if raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var w domain.WrapperConfig
	if err := json.Unmarshal(b, &w); err != nil {
		return nil
	}
	return &w
}

func parseMediaStyle(raw interface{}) *domain.MediaStyle {
	if raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var ms domain.MediaStyle
	if err := json.Unmarshal(b, &ms); err != nil {
		return nil
	}
	return &ms
}

func parseIconStyle(raw interface{}) *domain.IconStyle {
	if raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var is domain.IconStyle
	if err := json.Unmarshal(b, &is); err != nil {
		return nil
	}
	return &is
}

// --- Fix A: Preserve locked customizations across auto rebuilds ---

// AtomOverrideSnapshot captures styling from a locked atom for carry-over.
type AtomOverrideSnapshot struct {
	TextStyle  *domain.TextStyle     `json:"textStyle,omitempty"`
	Wrapper    *domain.WrapperConfig `json:"wrapper,omitempty"`
	MediaStyle *domain.MediaStyle    `json:"mediaStyle,omitempty"`
	IconStyle  *domain.IconStyle     `json:"iconStyle,omitempty"`
	Format     domain.AtomFormat     `json:"format,omitempty"`
}

// FreestyleAtomSnapshot captures a freestyle atom (no fieldName) for carry-over.
type FreestyleAtomSnapshot struct {
	Atom       domain.Atom `json:"atom"`
	ParentNode string      `json:"parentNode"` // layout node name where it was placed
}

// FormationSnapshot captures customizations from an existing formation.
type FormationSnapshot struct {
	FieldOverrides     map[string]AtomOverrideSnapshot // fieldName → overrides
	FreestyleAtoms     []FreestyleAtomSnapshot         // atoms without fieldName (buttons, badges)
	FormationOverrides *FormationOverrideSnapshot      // formation-level params (grid, mode, size)
}

// FormationOverrideSnapshot captures formation-level params that should survive auto rebuild.
type FormationOverrideSnapshot struct {
	Grid       *domain.GridConfig   `json:"grid,omitempty"`
	Mode       domain.FormationType `json:"mode,omitempty"`
	WidgetSize domain.WidgetSize    `json:"widgetSize,omitempty"`
}

// SnapshotCustomizations extracts locked overrides and freestyle atoms from a formation.
func SnapshotCustomizations(formation *domain.FormationWithData) *FormationSnapshot {
	if formation == nil {
		return nil
	}

	snap := &FormationSnapshot{
		FieldOverrides: make(map[string]AtomOverrideSnapshot),
	}

	// Capture formation-level overrides (grid, mode, widget size)
	hasFormationOverrides := false
	fo := FormationOverrideSnapshot{}
	if formation.Grid != nil {
		gc := *formation.Grid
		fo.Grid = &gc
		hasFormationOverrides = true
	}
	if formation.Mode != "" {
		fo.Mode = formation.Mode
		hasFormationOverrides = true
	}
	// Use first widget as reference (all widgets share the same field structure)
	var refWidget *domain.Widget
	if len(formation.Widgets) > 0 {
		refWidget = &formation.Widgets[0]
	} else if len(formation.Sections) > 0 && len(formation.Sections[0].Widgets) > 0 {
		refWidget = &formation.Sections[0].Widgets[0]
	}
	if refWidget == nil {
		if hasFormationOverrides {
			snap.FormationOverrides = &fo
			return snap
		}
		return nil
	}
	if refWidget.Size != "" {
		fo.WidgetSize = refWidget.Size
		hasFormationOverrides = true
	}
	if hasFormationOverrides {
		snap.FormationOverrides = &fo
	}

	for _, atom := range refWidget.Atoms {
		if atom.Rigidity != domain.RigidityLocked {
			continue
		}

		if atom.FieldName != "" {
			// Data-bound atom with locked overrides
			override := AtomOverrideSnapshot{
				Format: atom.Format,
			}
			if atom.TextStyle != nil {
				ts := *atom.TextStyle
				override.TextStyle = &ts
			}
			if atom.Wrapper != nil {
				w := *atom.Wrapper
				override.Wrapper = &w
			}
			if atom.MediaStyle != nil {
				ms := *atom.MediaStyle
				override.MediaStyle = &ms
			}
			if atom.IconStyle != nil {
				is := *atom.IconStyle
				override.IconStyle = &is
			}
			snap.FieldOverrides[atom.FieldName] = override
		} else {
			// Freestyle atom (no data binding) — carry over entirely
			atomCopy := atom
			atomCopy.ID = "" // will be re-stamped
			parentName := findAtomParentNodeName(refWidget, &atom)
			snap.FreestyleAtoms = append(snap.FreestyleAtoms, FreestyleAtomSnapshot{
				Atom:       atomCopy,
				ParentNode: parentName,
			})
		}
	}

	if len(snap.FieldOverrides) == 0 && len(snap.FreestyleAtoms) == 0 && snap.FormationOverrides == nil {
		return nil
	}
	return snap
}

// ApplySnapshot re-applies saved customizations onto a freshly built formation.
func ApplySnapshot(formation *domain.FormationWithData, snap *FormationSnapshot) {
	if snap == nil || formation == nil {
		return
	}

	// Restore formation-level overrides
	if fo := snap.FormationOverrides; fo != nil {
		if fo.Grid != nil {
			gc := *fo.Grid
			formation.Grid = &gc
		}
		if fo.Mode != "" {
			formation.Mode = fo.Mode
		}
		if fo.WidgetSize != "" {
			for wi := range formation.Widgets {
				formation.Widgets[wi].Size = fo.WidgetSize
			}
			for si := range formation.Sections {
				for wi := range formation.Sections[si].Widgets {
					formation.Sections[si].Widgets[wi].Size = fo.WidgetSize
				}
			}
		}
	}

	applyToWidgets := func(widgets []domain.Widget) {
		for wi := range widgets {
			w := &widgets[wi]

			// Re-apply field overrides
			for ai := range w.Atoms {
				a := &w.Atoms[ai]
				override, ok := snap.FieldOverrides[a.FieldName]
				if !ok {
					continue
				}
				if override.TextStyle != nil {
					ts := *override.TextStyle
					a.TextStyle = &ts
				}
				if override.Wrapper != nil {
					wr := *override.Wrapper
					a.Wrapper = &wr
				}
				if override.MediaStyle != nil {
					ms := *override.MediaStyle
					a.MediaStyle = &ms
				}
				if override.IconStyle != nil {
					is := *override.IconStyle
					a.IconStyle = &is
				}
				if override.Format != "" {
					a.Format = override.Format
				}
				a.Rigidity = domain.RigidityLocked
			}

			// Re-insert freestyle atoms
			for _, fs := range snap.FreestyleAtoms {
				atomCopy := fs.Atom
				atomCopy.ID = "" // will be re-stamped
				newIdx := len(w.Atoms)
				w.Atoms = append(w.Atoms, atomCopy)

				// Add to layout
				if w.Layout != nil {
					child := domain.NewAtomChild(newIdx)
					targetNode := findNodeByName(w.Layout, fs.ParentNode)
					if targetNode != nil {
						targetNode.Children = append(targetNode.Children, child)
					} else {
						w.Layout.Children = append(w.Layout.Children, child)
					}
				}
			}
		}
	}

	applyToWidgets(formation.Widgets)
	for si := range formation.Sections {
		applyToWidgets(formation.Sections[si].Widgets)
	}

	StampTreeIDs(formation)
}

// findAtomParentNodeName finds which layout node contains the given atom.
func findAtomParentNodeName(w *domain.Widget, target *domain.Atom) string {
	if w.Layout == nil {
		return ""
	}
	// Find atom index
	targetIdx := -1
	for i := range w.Atoms {
		if &w.Atoms[i] == target {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		return ""
	}
	return findNodeNameForAtomIdx(w.Layout, targetIdx)
}

func findNodeNameForAtomIdx(node *domain.LayoutNode, idx int) string {
	for _, ch := range node.Children {
		if ch.AtomIndex != nil && *ch.AtomIndex == idx {
			return node.Name
		}
		if ch.Node != nil {
			if name := findNodeNameForAtomIdx(ch.Node, idx); name != "" {
				return name
			}
		}
	}
	return ""
}

func findNodeByName(node *domain.LayoutNode, name string) *domain.LayoutNode {
	if name == "" {
		return nil
	}
	if node.Name == name {
		return node
	}
	for _, ch := range node.Children {
		if ch.Node != nil {
			if found := findNodeByName(ch.Node, name); found != nil {
				return found
			}
		}
	}
	return nil
}

// --- Fix D: Wildcard ops — field name as target applies to all widgets ---

// ExpandWildcardOps expands ops that target a field name (not a specific ID) into per-widget ops.
// E.g. target:"price" → becomes ops for a-s0-w0-price, a-s0-w1-price, etc.
func ExpandWildcardOps(formation *domain.FormationWithData, ops []Op) []Op {
	if formation == nil {
		return ops
	}

	idx := buildIndex(formation)
	var expanded []Op
	// Track which widgets already received an op to prevent duplicates
	type dedupKey struct {
		widgetID string
		opType   OpType
		field    string // target or parent field name
	}
	seen := map[dedupKey]bool{}

	for _, op := range ops {
		target := op.Target
		parent := op.Parent

		// Check if target looks like a field name (not a tree ID)
		if target != "" && !IsTreeID(target) {
			// Wildcard: apply to all atoms with this fieldName
			matched := false
			for id, ref := range idx.atoms {
				if ref.widget.Atoms[ref.atomIndex].FieldName == target {
					dk := dedupKey{widgetID: ref.widget.ID, opType: op.Type, field: target}
					if seen[dk] {
						continue
					}
					seen[dk] = true
					expOp := op
					expOp.Target = id
					expanded = append(expanded, expOp)
					matched = true
				}
			}
			if !matched {
				expanded = append(expanded, op) // pass through, will warn
			}
			continue
		}

		// Check if parent looks like a field name for insert/move
		if parent != "" && !IsTreeID(parent) {
			matched := false
			for id, ref := range idx.nodes {
				if ref.node.Name == parent {
					dk := dedupKey{widgetID: ref.widget.ID, opType: op.Type, field: parent}
					if seen[dk] {
						continue
					}
					seen[dk] = true
					expOp := op
					expOp.Parent = id
					expanded = append(expanded, expOp)
					matched = true
				}
			}
			if !matched {
				expanded = append(expanded, op)
			}
			continue
		}

		// For specific ID targets, also track to prevent wildcard duplication
		if target != "" {
			if ref, ok := idx.atoms[target]; ok && ref.widget != nil {
				fieldName := ref.widget.Atoms[ref.atomIndex].FieldName
				if fieldName != "" {
					dk := dedupKey{widgetID: ref.widget.ID, opType: op.Type, field: fieldName}
					seen[dk] = true
				}
			}
		}
		if parent != "" {
			if ref, ok := idx.nodes[parent]; ok && ref.widget != nil {
				if ref.node.Name != "" {
					dk := dedupKey{widgetID: ref.widget.ID, opType: op.Type, field: ref.node.Name}
					seen[dk] = true
				}
			}
		}

		expanded = append(expanded, op)
	}

	return expanded
}

// liftTextStyleShorthands extracts shorthand props (color, fontSize, fontWeight)
// that LLM may send at top level instead of nested in textStyle.
func liftTextStyleShorthands(props map[string]interface{}) *domain.TextStyle {
	var ts *domain.TextStyle
	ensure := func() {
		if ts == nil {
			ts = &domain.TextStyle{}
		}
	}
	if v, ok := props["color"].(string); ok && v != "" {
		ensure()
		ts.Color = v
	}
	if v, ok := props["fontSize"].(string); ok && v != "" {
		ensure()
		ts.FontSize = v
	}
	if v, ok := props["fontWeight"].(string); ok && v != "" {
		ensure()
		ts.FontWeight = v
	}
	if v, ok := props["lineClamp"].(float64); ok && v > 0 {
		ensure()
		ts.LineClamp = int(v)
	}
	return ts
}
