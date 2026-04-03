package engine_v4

import (
	"fmt"

	"keepstar_v4/internal/domain"
)

// defaultPresets returns all registered preset functions.
func defaultPresets() map[string]PresetFunc {
	return map[string]PresetFunc{
		"product_card_grid": presetProductCardGrid,
		"product_detail":    presetProductDetail,
		"product_row":       presetProductRow,
	}
}

// makeAtom creates an atom with field binding and preferred rigidity.
func makeAtom(fieldName string, atomType domain.AtomType, slot domain.AtomSlot) domain.Atom {
	return domain.Atom{
		Type:      atomType,
		FieldName: fieldName,
		Slot:      slot,
		Rigidity:  domain.RigidityPreferred,
	}
}

// presetProductCardGrid creates a grid formation with one card widget per data entry.
// Each card: image (hero), name (title, xl bold), price (currency), rating (stars-compact), brand (badge).
func presetProductCardGrid(data []map[string]interface{}, entityType string) *domain.FormationWithData {
	count := len(data)

	// Determine grid columns based on item count
	cols := 3
	if count <= 1 {
		cols = 1
	} else if count <= 4 {
		cols = 2
	}

	widgets := make([]domain.Widget, 0, count)
	for i := range data {
		// Atoms: image, name, price, rating, brand
		imageAtom := makeAtom("images", domain.AtomTypeImage, domain.AtomSlotHero)
		imageAtom.MediaStyle = &domain.MediaStyle{
			AspectRatio: "4/3",
			ObjectFit:   "cover",
		}

		nameAtom := makeAtom("name", domain.AtomTypeText, domain.AtomSlotTitle)
		nameAtom.TextStyle = &domain.TextStyle{
			FontSize:   "xl",
			FontWeight: "bold",
			LineClamp:  2,
		}

		priceAtom := makeAtom("price", domain.AtomTypeNumber, domain.AtomSlotPrice)
		priceAtom.Format = domain.FormatCurrency

		ratingAtom := makeAtom("rating", domain.AtomTypeNumber, domain.AtomSlotPrimary)
		ratingAtom.Format = domain.FormatStarsCompact

		brandAtom := makeAtom("brand", domain.AtomTypeText, domain.AtomSlotBadge)
		brandAtom.Wrapper = &domain.WrapperConfig{
			Type:    "badge",
			Variant: "subtle",
		}

		atoms := []domain.Atom{imageAtom, nameAtom, priceAtom, ratingAtom, brandAtom}

		// Layout tree:
		// column(root)
		//   span(image)            — atom 0
		//   column(info)
		//     atom(name)           — atom 1
		//     row(price-row)
		//       atom(price)        — atom 2
		//       atom(rating)       — atom 3
		//   flow(badges)
		//     atom(brand)          — atom 4
		priceRow := &domain.LayoutNode{
			Type:     domain.LayoutNodeRow,
			Gap:      "sm",
			Align:    "center",
			Name:     "price-row",
			Children: []domain.LayoutChild{domain.NewAtomChild(2), domain.NewAtomChild(3)},
		}

		infoCol := &domain.LayoutNode{
			Type:  domain.LayoutNodeColumn,
			Gap:   "xs",
			Name:  "info",
			Children: []domain.LayoutChild{
				domain.NewAtomChild(1),
				domain.NewNodeChild(priceRow),
			},
		}

		badgesFlow := &domain.LayoutNode{
			Type:     domain.LayoutNodeFlow,
			Gap:      "xs",
			Wrap:     true,
			Name:     "badges",
			Children: []domain.LayoutChild{domain.NewAtomChild(4)},
		}

		root := &domain.LayoutNode{
			Type: domain.LayoutNodeColumn,
			Gap:  "sm",
			Name: "root",
			Children: []domain.LayoutChild{
				{Node: &domain.LayoutNode{
					Type:     domain.LayoutNodeSpan,
					Name:     "hero",
					Children: []domain.LayoutChild{domain.NewAtomChild(0)},
				}},
				domain.NewNodeChild(infoCol),
				domain.NewNodeChild(badgesFlow),
			},
		}

		widgets = append(widgets, domain.Widget{
			Size:   domain.WidgetSizeMedium,
			Layout: root,
			Atoms:  atoms,
			EntityRef: &domain.EntityRef{
				Type: domain.EntityType(entityType),
				ID:   fmt.Sprintf("%d", i),
			},
		})
	}

	return &domain.FormationWithData{
		Mode:    domain.FormationTypeGrid,
		Grid:    &domain.GridConfig{Cols: cols},
		Widgets: widgets,
	}
}

// presetProductDetail creates a single-widget formation for a detailed product view.
// Fields: image, name (2xl bold), price (xl, currency), rating, brand, description, category, tags.
func presetProductDetail(data []map[string]interface{}, entityType string) *domain.FormationWithData {
	widgets := make([]domain.Widget, 0, 1)

	if len(data) == 0 {
		return &domain.FormationWithData{
			Mode:    domain.FormationTypeSingle,
			Widgets: widgets,
		}
	}

	// Atoms: image, name, price, rating, brand, description, category, tags
	imageAtom := makeAtom("images", domain.AtomTypeImage, domain.AtomSlotHero)
	imageAtom.MediaStyle = &domain.MediaStyle{
		AspectRatio: "16/9",
		ObjectFit:   "cover",
	}

	nameAtom := makeAtom("name", domain.AtomTypeText, domain.AtomSlotTitle)
	nameAtom.TextStyle = &domain.TextStyle{
		FontSize:   "2xl",
		FontWeight: "bold",
	}

	priceAtom := makeAtom("price", domain.AtomTypeNumber, domain.AtomSlotPrice)
	priceAtom.Format = domain.FormatCurrency
	priceAtom.TextStyle = &domain.TextStyle{
		FontSize: "xl",
	}

	ratingAtom := makeAtom("rating", domain.AtomTypeNumber, domain.AtomSlotPrimary)
	ratingAtom.Format = domain.FormatStarsCompact

	brandAtom := makeAtom("brand", domain.AtomTypeText, domain.AtomSlotBadge)
	brandAtom.Wrapper = &domain.WrapperConfig{
		Type:    "badge",
		Variant: "subtle",
	}

	descAtom := makeAtom("description", domain.AtomTypeText, domain.AtomSlotDescription)
	descAtom.TextStyle = &domain.TextStyle{
		LineClamp: 6,
	}

	categoryAtom := makeAtom("category", domain.AtomTypeText, domain.AtomSlotSecondary)
	categoryAtom.Wrapper = &domain.WrapperConfig{
		Type:    "tag",
		Variant: "outline",
	}

	tagsAtom := makeAtom("tags", domain.AtomTypeText, domain.AtomSlotTags)
	tagsAtom.Wrapper = &domain.WrapperConfig{
		Type:    "tag",
		Variant: "subtle",
	}

	atoms := []domain.Atom{imageAtom, nameAtom, priceAtom, ratingAtom, brandAtom, descAtom, categoryAtom, tagsAtom}

	// Layout tree:
	// column(root)
	//   span(hero)                — atom 0 (image)
	//   column(content)
	//     atom(name)              — atom 1
	//     row(price-row)
	//       atom(price)           — atom 2
	//       atom(rating)          — atom 3
	//     atom(brand)             — atom 4
	//     atom(description)       — atom 5
	//     flow(tags)
	//       atom(category)        — atom 6
	//       atom(tags)            — atom 7
	priceRow := &domain.LayoutNode{
		Type:     domain.LayoutNodeRow,
		Gap:      "sm",
		Align:    "center",
		Name:     "price-row",
		Children: []domain.LayoutChild{domain.NewAtomChild(2), domain.NewAtomChild(3)},
	}

	tagsFlow := &domain.LayoutNode{
		Type:     domain.LayoutNodeFlow,
		Gap:      "xs",
		Wrap:     true,
		Name:     "tags",
		Children: []domain.LayoutChild{domain.NewAtomChild(6), domain.NewAtomChild(7)},
	}

	contentCol := &domain.LayoutNode{
		Type: domain.LayoutNodeColumn,
		Gap:  "md",
		Name: "content",
		Children: []domain.LayoutChild{
			domain.NewAtomChild(1),
			domain.NewNodeChild(priceRow),
			domain.NewAtomChild(4),
			domain.NewAtomChild(5),
			domain.NewNodeChild(tagsFlow),
		},
	}

	root := &domain.LayoutNode{
		Type: domain.LayoutNodeColumn,
		Gap:  "md",
		Name: "root",
		Children: []domain.LayoutChild{
			{Node: &domain.LayoutNode{
				Type:     domain.LayoutNodeSpan,
				Name:     "hero",
				Children: []domain.LayoutChild{domain.NewAtomChild(0)},
			}},
			domain.NewNodeChild(contentCol),
		},
	}

	widgets = append(widgets, domain.Widget{
		Size:   domain.WidgetSizeLarge,
		Layout: root,
		Atoms:  atoms,
		EntityRef: &domain.EntityRef{
			Type: domain.EntityType(entityType),
			ID:   "0",
		},
	})

	return &domain.FormationWithData{
		Mode:    domain.FormationTypeSingle,
		Widgets: widgets,
	}
}

// presetProductRow creates a list formation with compact row widgets.
// Each row: image (small), name, price, rating.
func presetProductRow(data []map[string]interface{}, entityType string) *domain.FormationWithData {
	widgets := make([]domain.Widget, 0, len(data))

	for i := range data {
		// Atoms: image, name, price, rating
		imageAtom := makeAtom("images", domain.AtomTypeImage, domain.AtomSlotHero)
		imageAtom.MediaStyle = &domain.MediaStyle{
			AspectRatio: "1/1",
			ObjectFit:   "cover",
		}

		nameAtom := makeAtom("name", domain.AtomTypeText, domain.AtomSlotTitle)
		nameAtom.TextStyle = &domain.TextStyle{
			FontWeight: "medium",
			LineClamp:  1,
		}

		priceAtom := makeAtom("price", domain.AtomTypeNumber, domain.AtomSlotPrice)
		priceAtom.Format = domain.FormatCurrency

		ratingAtom := makeAtom("rating", domain.AtomTypeNumber, domain.AtomSlotPrimary)
		ratingAtom.Format = domain.FormatStarsCompact

		atoms := []domain.Atom{imageAtom, nameAtom, priceAtom, ratingAtom}

		// Layout tree:
		// row(root)
		//   atom(image)           — atom 0
		//   column(info)
		//     atom(name)          — atom 1
		//     row(price-row)
		//       atom(price)       — atom 2
		//       atom(rating)      — atom 3
		priceRow := &domain.LayoutNode{
			Type:     domain.LayoutNodeRow,
			Gap:      "sm",
			Align:    "center",
			Name:     "price-row",
			Children: []domain.LayoutChild{domain.NewAtomChild(2), domain.NewAtomChild(3)},
		}

		infoCol := &domain.LayoutNode{
			Type: domain.LayoutNodeColumn,
			Gap:  "xs",
			Name: "info",
			Children: []domain.LayoutChild{
				domain.NewAtomChild(1),
				domain.NewNodeChild(priceRow),
			},
		}

		imageChild := domain.NewAtomChild(0)
		imageChild.Sizing = "hug"
		imageChild.MaxWidth = "64px"

		infoChild := domain.NewNodeChild(infoCol)
		infoChild.Grow = 1

		root := &domain.LayoutNode{
			Type:  domain.LayoutNodeRow,
			Gap:   "md",
			Align: "center",
			Name:  "root",
			Children: []domain.LayoutChild{
				imageChild,
				infoChild,
			},
		}

		widgets = append(widgets, domain.Widget{
			Size:   domain.WidgetSizeSmall,
			Layout: root,
			Atoms:  atoms,
			EntityRef: &domain.EntityRef{
				Type: domain.EntityType(entityType),
				ID:   fmt.Sprintf("%d", i),
			},
		})
	}

	return &domain.FormationWithData{
		Mode:    domain.FormationTypeList,
		Widgets: widgets,
	}
}
