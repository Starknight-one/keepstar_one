package engine

import (
	"fmt"

	"keepstar/internal/domain"
)

// SectionSource defines how a section gets its content.
type SectionSource string

const (
	SectionSourceAuto      SectionSource = "auto"      // Use V2 engine pipeline with data
	SectionSourceFreestyle SectionSource = "freestyle"  // Literal atoms, no data binding
)

// SectionSpec describes one section in a build-mode composition.
type SectionSpec struct {
	Source SectionSource          `json:"source"`
	Label  string                 `json:"label,omitempty"`

	// For auto sections — same params as auto mode
	AutoParams map[string]interface{} `json:"params,omitempty"`

	// For freestyle sections — literal widget definitions
	Widgets []FreestyleWidgetSpec `json:"widgets,omitempty"`
}

// FreestyleWidgetSpec defines a widget built from literal atoms (no data binding).
type FreestyleWidgetSpec struct {
	Size   string               `json:"size,omitempty"`
	Layout *FreestyleLayoutSpec `json:"layout,omitempty"`
	Atoms  []map[string]interface{} `json:"atoms"` // Raw atom props (same shape as insert props)
}

// FreestyleLayoutSpec defines layout for a freestyle widget.
type FreestyleLayoutSpec struct {
	Type         string `json:"type"` // row, column, flow
	Gap          string `json:"gap,omitempty"`
	Align        string `json:"align,omitempty"`
	Distribution string `json:"distribution,omitempty"`
}

// BuildFormation assembles a multi-section formation from section specs.
// autoExecutor is called for "auto" sections — it runs the existing V2 engine pipeline.
func BuildFormation(
	sections []SectionSpec,
	autoExecutor func(params map[string]interface{}) (*domain.FormationWithData, []string),
) (*domain.FormationWithData, []string) {
	formation := &domain.FormationWithData{
		Mode: domain.FormationTypeGrid,
	}
	var allWarnings []string

	for i, sec := range sections {
		switch sec.Source {
		case SectionSourceAuto:
			if autoExecutor == nil {
				allWarnings = append(allWarnings, fmt.Sprintf("section[%d]: auto executor not available", i))
				continue
			}
			result, warnings := autoExecutor(sec.AutoParams)
			if result == nil {
				allWarnings = append(allWarnings, fmt.Sprintf("section[%d]: auto returned nil", i))
				continue
			}
			allWarnings = append(allWarnings, warnings...)

			section := domain.FormationSection{
				Mode:    result.Mode,
				Grid:    result.Grid,
				Widgets: result.Widgets,
				Label:   sec.Label,
			}
			formation.Sections = append(formation.Sections, section)

		case SectionSourceFreestyle:
			widgets := buildFreestyleWidgets(sec.Widgets)
			if len(widgets) == 0 {
				allWarnings = append(allWarnings, fmt.Sprintf("section[%d]: freestyle produced no widgets", i))
				continue
			}

			// Determine section mode from widget count
			mode := domain.FormationTypeGrid
			if len(widgets) == 1 {
				mode = domain.FormationTypeSingle
			}

			section := domain.FormationSection{
				Mode:    mode,
				Widgets: widgets,
				Label:   sec.Label,
			}
			formation.Sections = append(formation.Sections, section)

		default:
			allWarnings = append(allWarnings, fmt.Sprintf("section[%d]: unknown source %q", i, sec.Source))
		}
	}

	// Stamp IDs on the assembled formation
	StampTreeIDs(formation)

	return formation, allWarnings
}

// buildFreestyleWidgets creates widgets from literal atom specifications.
func buildFreestyleWidgets(specs []FreestyleWidgetSpec) []domain.Widget {
	var widgets []domain.Widget

	for _, spec := range specs {
		w := domain.Widget{
			ID:       generateWidgetID(),
			Template: "GenericCard",
			Type:     domain.WidgetTypeTextBlock,
			Size:     domain.WidgetSizeLarge,
		}
		if spec.Size != "" {
			w.Size = domain.WidgetSize(spec.Size)
		}

		// Build atoms from raw props
		for _, atomProps := range spec.Atoms {
			atom := parseAtomFromProps(atomProps)
			atom.Rigidity = domain.RigidityLocked
			w.AtomsV2 = append(w.AtomsV2, atom)
		}

		// Build layout
		if spec.Layout != nil {
			w.Layout = &domain.LayoutNode{
				Type: domain.LayoutNodeType(spec.Layout.Type),
				Gap:  spec.Layout.Gap,
			}
			if spec.Layout.Align != "" {
				w.Layout.Align = spec.Layout.Align
			}
			if spec.Layout.Distribution != "" {
				w.Layout.Distribution = spec.Layout.Distribution
			}
			// Add all atoms as children
			for i := range w.AtomsV2 {
				w.Layout.Children = append(w.Layout.Children, domain.NewAtomChild(i))
			}
		} else {
			// Default: column layout
			w.Layout = &domain.LayoutNode{
				Type: domain.LayoutNodeColumn,
				Gap:  "sm",
			}
			for i := range w.AtomsV2 {
				w.Layout.Children = append(w.Layout.Children, domain.NewAtomChild(i))
			}
		}

		// Generate v1 compat
		WidgetV2ToLegacy(&w)

		widgets = append(widgets, w)
	}

	return widgets
}

// ParseSectionSpecs parses raw JSON section specs into typed SectionSpec slice.
func ParseSectionSpecs(raw []interface{}) ([]SectionSpec, error) {
	var specs []SectionSpec
	for i, item := range raw {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("section[%d]: expected object", i)
		}

		source, _ := itemMap["source"].(string)
		if source == "" {
			return nil, fmt.Errorf("section[%d]: missing 'source' field", i)
		}

		spec := SectionSpec{
			Source: SectionSource(source),
		}
		if v, ok := itemMap["label"].(string); ok {
			spec.Label = v
		}

		switch SectionSource(source) {
		case SectionSourceAuto:
			// Everything except source/label is auto params
			params := make(map[string]interface{})
			for k, v := range itemMap {
				if k != "source" && k != "label" {
					params[k] = v
				}
			}
			spec.AutoParams = params

		case SectionSourceFreestyle:
			widgetsRaw, ok := itemMap["widgets"].([]interface{})
			if !ok || len(widgetsRaw) == 0 {
				return nil, fmt.Errorf("section[%d]: freestyle requires 'widgets' array", i)
			}
			for wi, wRaw := range widgetsRaw {
				wMap, ok := wRaw.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("section[%d].widgets[%d]: expected object", i, wi)
				}
				fws := FreestyleWidgetSpec{}
				if v, ok := wMap["size"].(string); ok {
					fws.Size = v
				}
				if layoutRaw, ok := wMap["layout"].(map[string]interface{}); ok {
					fws.Layout = &FreestyleLayoutSpec{}
					if v, ok := layoutRaw["type"].(string); ok {
						fws.Layout.Type = v
					}
					if v, ok := layoutRaw["gap"].(string); ok {
						fws.Layout.Gap = v
					}
					if v, ok := layoutRaw["align"].(string); ok {
						fws.Layout.Align = v
					}
					if v, ok := layoutRaw["distribution"].(string); ok {
						fws.Layout.Distribution = v
					}
				}
				if atomsRaw, ok := wMap["atoms"].([]interface{}); ok {
					for _, aRaw := range atomsRaw {
						if aMap, ok := aRaw.(map[string]interface{}); ok {
							fws.Atoms = append(fws.Atoms, aMap)
						}
					}
				}
				spec.Widgets = append(spec.Widgets, fws)
			}
		}

		specs = append(specs, spec)
	}
	return specs, nil
}
