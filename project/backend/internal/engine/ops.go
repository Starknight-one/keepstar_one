package engine

import (
	"encoding/json"
	"fmt"

	"keepstar/internal/domain"
)

// OpType discriminates tree operations.
type OpType string

const (
	OpUpdate OpType = "update"
	OpDelete OpType = "delete"
	OpInsert OpType = "insert"
	OpMove   OpType = "move"
)

// Op is a single tree operation from Agent2.
type Op struct {
	Type   OpType                 `json:"op"`
	Target string                 `json:"target,omitempty"`  // ID of atom, layout node, or widget
	Ref    string                 `json:"ref,omitempty"`     // Local alias for insert (subsequent ops can reference this)
	Parent string                 `json:"parent,omitempty"`  // Where to insert/move to (widget or layout node ID)
	After  string                 `json:"after,omitempty"`   // Position after this ID (for insert/move ordering)
	Props  map[string]interface{} `json:"props,omitempty"`   // Properties to set
}

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
}

// idIndex maps IDs to their targets in the formation tree.
type idIndex struct {
	atoms   map[string]atomRef
	nodes   map[string]nodeRef
	widgets map[string]*domain.Widget
}

// buildIndex walks the formation and creates an ID -> target mapping.
func buildIndex(formation *domain.FormationWithData) idIndex {
	idx := idIndex{
		atoms:   make(map[string]atomRef),
		nodes:   make(map[string]nodeRef),
		widgets: make(map[string]*domain.Widget),
	}

	indexWidgets := func(widgets []domain.Widget) {
		for wi := range widgets {
			w := &widgets[wi]
			if w.ID != "" {
				idx.widgets[w.ID] = w
			}
			for ai := range w.AtomsV2 {
				if w.AtomsV2[ai].ID != "" {
					idx.atoms[w.AtomsV2[ai].ID] = atomRef{widget: w, atomIndex: ai}
				}
			}
			if w.Layout != nil {
				indexLayoutNode(w.Layout, nil, &idx)
			}
		}
	}

	indexWidgets(formation.Widgets)
	for si := range formation.Sections {
		indexWidgets(formation.Sections[si].Widgets)
	}

	return idx
}

func indexLayoutNode(node *domain.LayoutNode, parent *domain.LayoutNode, idx *idIndex) {
	if node.ID != "" {
		idx.nodes[node.ID] = nodeRef{node: node, parent: parent}
	}
	for i := range node.Children {
		if node.Children[i].Node != nil {
			indexLayoutNode(node.Children[i].Node, node, idx)
		}
	}
}

// ApplyOps applies a batch of operations to an existing formation.
// Returns warnings (non-fatal) and error (fatal).
func ApplyOps(formation *domain.FormationWithData, ops []Op) ([]string, error) {
	if formation == nil {
		return nil, fmt.Errorf("no existing formation to modify")
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

	// Run constraints after all ops
	runPostOpsConstraints(formation)

	return warnings, nil
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
		atom := &ref.widget.AtomsV2[ref.atomIndex]
		mergeAtomProps(atom, op.Props)
		// Auto-lock: agent explicitly modified this atom
		if atom.Rigidity != domain.RigidityLocked {
			atom.Rigidity = domain.RigidityLocked
		}
		// Regenerate v1 compat
		WidgetV2ToLegacy(ref.widget)
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

	return fmt.Sprintf("target %q not found", op.Target)
}

// applyDelete removes an atom or layout node from the tree.
func applyDelete(op Op, idx *idIndex) string {
	// Try atom
	if ref, ok := idx.atoms[op.Target]; ok {
		w := ref.widget
		atomIdx := ref.atomIndex

		// Remove from AtomsV2
		w.AtomsV2 = append(w.AtomsV2[:atomIdx], w.AtomsV2[atomIdx+1:]...)

		// Remove matching LayoutChild and fix remaining atomIndex references
		if w.Layout != nil {
			removeAtomFromLayout(w.Layout, atomIdx)
			fixAtomIndices(w.Layout, atomIdx)
		}

		// Regenerate v1 compat
		WidgetV2ToLegacy(w)

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
		idx.nodes[op.Target] = nodeRef{node: ref.node, parent: parentNode}
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

	// Determine if inserting an atom or a layout node
	propType, _ := op.Props["type"].(string)

	// If type matches a LayoutNodeType, insert a layout node
	if propType == "row" || propType == "column" || propType == "flow" || propType == "span" {
		return insertLayoutNode(op, idx)
	}

	// Otherwise insert an atom
	return insertAtom(op, idx, formation)
}

// insertAtom creates a new AtomV2 and appends it to the parent widget.
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
	newIdx := len(targetWidget.AtomsV2)
	targetWidget.AtomsV2 = append(targetWidget.AtomsV2, atom)

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

	// Regen v1 compat
	WidgetV2ToLegacy(targetWidget)

	// Re-index this widget's atoms (indices changed)
	reindexWidget(targetWidget, idx)

	// ID will be assigned by StampTreeIDs in runPostOpsConstraints
	return fmt.Sprintf("__pending_atom_%d", newIdx), ""
}

// insertLayoutNode creates a new LayoutNode and inserts it into a parent node.
func insertLayoutNode(op Op, idx *idIndex) (string, string) {
	parentRef, ok := idx.nodes[op.Parent]
	if !ok {
		// Try widget — insert as child of root layout
		if w, ok := idx.widgets[op.Parent]; ok && w.Layout != nil {
			parentRef = nodeRef{node: w.Layout}
		} else {
			return "", fmt.Sprintf("parent node %q not found for insert", op.Parent)
		}
	}

	node := parseLayoutNodeFromProps(op.Props)
	child := domain.NewNodeChild(node)
	insertChildIntoNode(parentRef.node, child, op.After, idx)

	// Index the new node
	if node.ID != "" {
		idx.nodes[node.ID] = nodeRef{node: node, parent: parentRef.node}
	}

	return fmt.Sprintf("__pending_node_%p", node), ""
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
					if ref.widget.AtomsV2[ref.atomIndex].ID == afterID {
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

// parseAtomFromProps creates an AtomV2 from raw props map.
func parseAtomFromProps(props map[string]interface{}) domain.AtomV2 {
	atom := domain.AtomV2{}

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
	for ai := range w.AtomsV2 {
		if w.AtomsV2[ai].ID != "" {
			idx.atoms[w.AtomsV2[ai].ID] = atomRef{widget: w, atomIndex: ai}
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

// mergeAtomProps merges a raw props map into an AtomV2.
func mergeAtomProps(atom *domain.AtomV2, props map[string]interface{}) {
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

// NOTE: mergeTextStyle is defined in engine_v2.go (same package)

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

// runPostOpsConstraints runs constraint pipeline on the modified formation.
func runPostOpsConstraints(formation *domain.FormationWithData) {
	for i := range formation.Widgets {
		w := &formation.Widgets[i]
		// Per-atom constraints (skip locked)
		w.AtomsV2 = applyAtomV2Constraints(w.AtomsV2)
		// Per-widget constraints
		applyWidgetV2Constraints(w)
		// Regen v1 compat
		WidgetV2ToLegacy(w)
	}

	// Cross-widget constraints
	if len(formation.Widgets) > 1 {
		applyCrossWidgetV2Constraints(formation.Widgets, formation.Mode)
	}

	// Same for sections
	for si := range formation.Sections {
		for wi := range formation.Sections[si].Widgets {
			w := &formation.Sections[si].Widgets[wi]
			w.AtomsV2 = applyAtomV2Constraints(w.AtomsV2)
			applyWidgetV2Constraints(w)
			WidgetV2ToLegacy(w)
		}
		if len(formation.Sections[si].Widgets) > 1 {
			applyCrossWidgetV2Constraints(formation.Sections[si].Widgets, formation.Sections[si].Mode)
		}
	}

	// Re-stamp IDs (in case new nodes were created)
	StampTreeIDs(formation)
}
