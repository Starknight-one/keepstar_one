# Presets

Preset registry for entity rendering.

## Files

- `preset_registry.go` — Central registry for presets
- `product_presets.go` — Product rendering presets
- `service_presets.go` — Service rendering presets

## Concept

Presets define how entities (products, services) map to widgets:
- Which fields to display
- Which slot each field goes to
- Which atom type to use for rendering
- Default formation mode and widget size

## Registry

```go
registry := presets.NewPresetRegistry()

// Get preset by name
preset, ok := registry.Get(domain.PresetProductGrid)

// Available presets
// - product_grid: multiple products in grid
// - product_card: single product card
// - product_compact: compact list
// - product_detail: full product detail view (drill-down)
// - service_card: service in grid
// - service_list: services in list
// - service_detail: full service detail view (drill-down)
```

## Preset Structure

```go
type Preset struct {
    Name        string
    EntityType  EntityType      // product, service
    Template    string          // widget template name
    Slots       map[AtomSlot]SlotConfig
    Fields      []FieldConfig   // field → slot → atomType
    DefaultMode FormationType   // grid, list, carousel
    DefaultSize WidgetSize      // small, medium, large
}

type FieldConfig struct {
    Name     string    // field name: "price", "rating"
    Slot     AtomSlot  // target slot: hero, title, primary
    AtomType AtomType  // render type: text, price, rating
    Priority int       // display order
    Required bool      // must include
}
```

## Rules

- Imports: `domain/` only
- Presets are stateless configurations
- Tools use presets to build formations
