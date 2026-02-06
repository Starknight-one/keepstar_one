package tools

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// FreestyleTool renders atoms with custom style aliases or explicit display overrides
type FreestyleTool struct {
	statePort ports.StatePort
}

// NewFreestyleTool creates the freestyle tool
func NewFreestyleTool(statePort ports.StatePort) *FreestyleTool {
	return &FreestyleTool{
		statePort: statePort,
	}
}

// Definition returns the tool definition for LLM
func (t *FreestyleTool) Definition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "freestyle",
		Description: "Render atoms with custom visual styles. Use style aliases (product-hero, product-compact, service-card) or explicit display overrides. Agent2 has full control over visual presentation.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"style": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"product-hero", "product-compact", "product-detail", "service-card", "service-detail"},
					"description": "Style alias that defines a set of displays for slots. Optional if using explicit overrides.",
				},
				"entity_type": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"product", "service"},
					"description": "Type of entities to render from state",
				},
				"formation": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"grid", "list", "carousel", "single"},
					"description": "Layout mode for the formation",
				},
				"overrides": map[string]interface{}{
					"type":        "object",
					"description": "Explicit slot→display overrides. Keys are slot names (hero, title, price, etc.), values are display names (h1, price-lg, badge-success, etc.)",
					"additionalProperties": map[string]interface{}{
						"type": "string",
					},
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of entities to render. Default: all",
				},
			},
			"required": []string{"entity_type", "formation"},
		},
	}
}

// Execute renders entities with freestyle styling
func (t *FreestyleTool) Execute(ctx context.Context, toolCtx ToolContext, input map[string]interface{}) (*domain.ToolResult, error) {
	entityType, _ := input["entity_type"].(string)
	formationMode, _ := input["formation"].(string)
	styleName, _ := input["style"].(string)
	overrides, _ := input["overrides"].(map[string]interface{})
	limit, _ := input["limit"].(float64) // JSON numbers come as float64

	state, err := t.statePort.GetState(ctx, toolCtx.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// Build display mapping: start with style, then apply overrides
	displayMapping := make(map[domain.AtomSlot]domain.AtomDisplay)

	// Apply style alias if provided
	if styleName != "" {
		style := domain.DisplayStyle(styleName)
		if styles, ok := domain.DisplayStyles[style]; ok {
			for slot, display := range styles {
				displayMapping[slot] = display
			}
		}
	}

	// Apply explicit overrides
	for slotStr, displayStr := range overrides {
		slot := domain.AtomSlot(slotStr)
		display := domain.AtomDisplay(displayStr.(string))
		displayMapping[slot] = display
	}

	// Get formation type
	formationType := parseFormationType(formationMode)

	// Build widgets based on entity type
	var widgets []domain.Widget
	var count int

	switch entityType {
	case "product":
		products := state.Current.Data.Products
		count = len(products)
		if limit > 0 && int(limit) < count {
			count = int(limit)
		}
		widgets = buildFreestyleWidgets(products[:count], displayMapping, domain.EntityTypeProduct)

	case "service":
		services := state.Current.Data.Services
		count = len(services)
		if limit > 0 && int(limit) < count {
			count = int(limit)
		}
		widgets = buildFreestyleServiceWidgets(services[:count], displayMapping)

	default:
		return &domain.ToolResult{Content: "error: unknown entity_type", IsError: true}, nil
	}

	if count == 0 {
		return &domain.ToolResult{Content: fmt.Sprintf("error: no %ss in state", entityType)}, nil
	}

	// Build formation
	formation := &domain.FormationWithData{
		Mode:    formationType,
		Widgets: widgets,
	}

	// Store in state via zone-write
	template := map[string]interface{}{
		"formation": formation,
	}

	info := domain.DeltaInfo{
		TurnID:    toolCtx.TurnID,
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   toolCtx.ActorID,
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "template",
		Action:    domain.Action{Type: domain.ActionLayout, Tool: "freestyle"},
	}
	if _, err := t.statePort.UpdateTemplate(ctx, toolCtx.SessionID, template, info); err != nil {
		return nil, fmt.Errorf("update template: %w", err)
	}

	styleInfo := "custom"
	if styleName != "" {
		styleInfo = styleName
	}

	return &domain.ToolResult{
		Content: fmt.Sprintf("ok: rendered %d %ss with freestyle style=%s, formation=%s", count, entityType, styleInfo, formationMode),
	}, nil
}

// parseFormationType converts string to FormationType
func parseFormationType(mode string) domain.FormationType {
	switch mode {
	case "grid":
		return domain.FormationTypeGrid
	case "list":
		return domain.FormationTypeList
	case "carousel":
		return domain.FormationTypeCarousel
	case "single":
		return domain.FormationTypeSingle
	default:
		return domain.FormationTypeGrid
	}
}

// buildFreestyleWidgets creates widgets for products with custom display mapping
func buildFreestyleWidgets(products []domain.Product, displayMapping map[domain.AtomSlot]domain.AtomDisplay, entityType domain.EntityType) []domain.Widget {
	widgets := make([]domain.Widget, 0, len(products))

	for i, p := range products {
		atoms := buildFreestyleAtoms(p, displayMapping)
		widget := domain.Widget{
			ID:       uuid.New().String(),
			Template: domain.WidgetTemplateProductCard,
			Size:     domain.WidgetSizeMedium,
			Priority: i,
			Atoms:    atoms,
			EntityRef: &domain.EntityRef{
				Type: entityType,
				ID:   p.ID,
			},
		}
		widgets = append(widgets, widget)
	}

	return widgets
}

// buildFreestyleAtoms creates atoms for a product with custom displays
func buildFreestyleAtoms(p domain.Product, displayMapping map[domain.AtomSlot]domain.AtomDisplay) []domain.Atom {
	atoms := make([]domain.Atom, 0)

	// Image/Hero
	if len(p.Images) > 0 {
		display := getDisplayOrDefault(displayMapping, domain.AtomSlotHero, domain.DisplayImageCover)
		atoms = append(atoms, domain.Atom{
			Type:    domain.AtomTypeImage,
			Subtype: domain.SubtypeImageURL,
			Display: string(display),
			Value:   p.Images,
			Slot:    domain.AtomSlotHero,
			Meta:    map[string]interface{}{"size": "large"},
		})
	}

	// Title
	if p.Name != "" {
		display := getDisplayOrDefault(displayMapping, domain.AtomSlotTitle, domain.DisplayH2)
		atoms = append(atoms, domain.Atom{
			Type:    domain.AtomTypeText,
			Subtype: domain.SubtypeString,
			Display: string(display),
			Value:   p.Name,
			Slot:    domain.AtomSlotTitle,
		})
	}

	// Brand (primary)
	if p.Brand != "" {
		display := getDisplayOrDefault(displayMapping, domain.AtomSlotPrimary, domain.DisplayTag)
		atoms = append(atoms, domain.Atom{
			Type:    domain.AtomTypeText,
			Subtype: domain.SubtypeString,
			Display: string(display),
			Value:   p.Brand,
			Slot:    domain.AtomSlotPrimary,
		})
	}

	// Category (primary)
	if p.Category != "" {
		display := getDisplayOrDefault(displayMapping, domain.AtomSlotPrimary, domain.DisplayTag)
		atoms = append(atoms, domain.Atom{
			Type:    domain.AtomTypeText,
			Subtype: domain.SubtypeString,
			Display: string(display),
			Value:   p.Category,
			Slot:    domain.AtomSlotPrimary,
		})
	}

	// Rating (primary)
	if p.Rating > 0 {
		display := getDisplayOrDefault(displayMapping, domain.AtomSlotPrimary, domain.DisplayRating)
		atoms = append(atoms, domain.Atom{
			Type:    domain.AtomTypeNumber,
			Subtype: domain.SubtypeRating,
			Display: string(display),
			Value:   p.Rating,
			Slot:    domain.AtomSlotPrimary,
		})
	}

	// Price
	if p.Price > 0 {
		display := getDisplayOrDefault(displayMapping, domain.AtomSlotPrice, domain.DisplayPrice)
		currency := p.Currency
		if currency == "" {
			currency = "$"
		}
		atoms = append(atoms, domain.Atom{
			Type:    domain.AtomTypeNumber,
			Subtype: domain.SubtypeCurrency,
			Display: string(display),
			Value:   p.Price,
			Slot:    domain.AtomSlotPrice,
			Meta:    map[string]interface{}{"currency": currency},
		})
	}

	// Description (secondary)
	if p.Description != "" {
		display := getDisplayOrDefault(displayMapping, domain.AtomSlotSecondary, domain.DisplayBody)
		atoms = append(atoms, domain.Atom{
			Type:    domain.AtomTypeText,
			Subtype: domain.SubtypeString,
			Display: string(display),
			Value:   p.Description,
			Slot:    domain.AtomSlotSecondary,
		})
	}

	return atoms
}

// buildFreestyleServiceWidgets creates widgets for services with custom display mapping
func buildFreestyleServiceWidgets(services []domain.Service, displayMapping map[domain.AtomSlot]domain.AtomDisplay) []domain.Widget {
	widgets := make([]domain.Widget, 0, len(services))

	for i, s := range services {
		atoms := buildFreestyleServiceAtoms(s, displayMapping)
		widget := domain.Widget{
			ID:       uuid.New().String(),
			Template: "ServiceCard",
			Size:     domain.WidgetSizeMedium,
			Priority: i,
			Atoms:    atoms,
			EntityRef: &domain.EntityRef{
				Type: domain.EntityTypeService,
				ID:   s.ID,
			},
		}
		widgets = append(widgets, widget)
	}

	return widgets
}

// buildFreestyleServiceAtoms creates atoms for a service with custom displays
func buildFreestyleServiceAtoms(s domain.Service, displayMapping map[domain.AtomSlot]domain.AtomDisplay) []domain.Atom {
	atoms := make([]domain.Atom, 0)

	// Image/Hero
	if len(s.Images) > 0 {
		display := getDisplayOrDefault(displayMapping, domain.AtomSlotHero, domain.DisplayImageCover)
		atoms = append(atoms, domain.Atom{
			Type:    domain.AtomTypeImage,
			Subtype: domain.SubtypeImageURL,
			Display: string(display),
			Value:   s.Images,
			Slot:    domain.AtomSlotHero,
			Meta:    map[string]interface{}{"size": "large"},
		})
	}

	// Title
	if s.Name != "" {
		display := getDisplayOrDefault(displayMapping, domain.AtomSlotTitle, domain.DisplayH2)
		atoms = append(atoms, domain.Atom{
			Type:    domain.AtomTypeText,
			Subtype: domain.SubtypeString,
			Display: string(display),
			Value:   s.Name,
			Slot:    domain.AtomSlotTitle,
		})
	}

	// Provider (primary)
	if s.Provider != "" {
		display := getDisplayOrDefault(displayMapping, domain.AtomSlotPrimary, domain.DisplayCaption)
		atoms = append(atoms, domain.Atom{
			Type:    domain.AtomTypeText,
			Subtype: domain.SubtypeString,
			Display: string(display),
			Value:   s.Provider,
			Slot:    domain.AtomSlotPrimary,
		})
	}

	// Duration (primary)
	if s.Duration != "" {
		display := getDisplayOrDefault(displayMapping, domain.AtomSlotPrimary, domain.DisplayCaption)
		atoms = append(atoms, domain.Atom{
			Type:    domain.AtomTypeText,
			Subtype: domain.SubtypeString,
			Display: string(display),
			Value:   s.Duration,
			Slot:    domain.AtomSlotPrimary,
		})
	}

	// Rating (primary)
	if s.Rating > 0 {
		display := getDisplayOrDefault(displayMapping, domain.AtomSlotPrimary, domain.DisplayRatingCompact)
		atoms = append(atoms, domain.Atom{
			Type:    domain.AtomTypeNumber,
			Subtype: domain.SubtypeRating,
			Display: string(display),
			Value:   s.Rating,
			Slot:    domain.AtomSlotPrimary,
		})
	}

	// Price
	if s.Price > 0 {
		display := getDisplayOrDefault(displayMapping, domain.AtomSlotPrice, domain.DisplayPrice)
		currency := s.Currency
		if currency == "" {
			currency = "$"
		}
		atoms = append(atoms, domain.Atom{
			Type:    domain.AtomTypeNumber,
			Subtype: domain.SubtypeCurrency,
			Display: string(display),
			Value:   s.Price,
			Slot:    domain.AtomSlotPrice,
			Meta:    map[string]interface{}{"currency": currency},
		})
	}

	// Description (secondary)
	if s.Description != "" {
		display := getDisplayOrDefault(displayMapping, domain.AtomSlotSecondary, domain.DisplayBody)
		atoms = append(atoms, domain.Atom{
			Type:    domain.AtomTypeText,
			Subtype: domain.SubtypeString,
			Display: string(display),
			Value:   s.Description,
			Slot:    domain.AtomSlotSecondary,
		})
	}

	return atoms
}

// getDisplayOrDefault returns the display for a slot from mapping, or the default
func getDisplayOrDefault(mapping map[domain.AtomSlot]domain.AtomDisplay, slot domain.AtomSlot, defaultDisplay domain.AtomDisplay) domain.AtomDisplay {
	if display, ok := mapping[slot]; ok {
		return display
	}
	return defaultDisplay
}
