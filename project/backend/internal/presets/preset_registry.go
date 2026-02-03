package presets

import "keepstar/internal/domain"

// PresetRegistry holds all available presets
type PresetRegistry struct {
	presets map[domain.PresetName]domain.Preset
}

// NewPresetRegistry creates a preset registry with all presets registered
func NewPresetRegistry() *PresetRegistry {
	r := &PresetRegistry{
		presets: make(map[domain.PresetName]domain.Preset),
	}

	// Register product presets
	r.Register(ProductGridPreset)
	r.Register(ProductCardPreset)
	r.Register(ProductCompactPreset)

	// Register service presets
	r.Register(ServiceCardPreset)
	r.Register(ServiceListPreset)

	return r
}

// Register adds a preset to the registry
func (r *PresetRegistry) Register(preset domain.Preset) {
	r.presets[domain.PresetName(preset.Name)] = preset
}

// Get returns a preset by name
func (r *PresetRegistry) Get(name domain.PresetName) (domain.Preset, bool) {
	p, ok := r.presets[name]
	return p, ok
}

// GetByEntityType returns all presets for a given entity type
func (r *PresetRegistry) GetByEntityType(entityType domain.EntityType) []domain.Preset {
	var result []domain.Preset
	for _, p := range r.presets {
		if p.EntityType == entityType {
			result = append(result, p)
		}
	}
	return result
}

// List returns all preset names
func (r *PresetRegistry) List() []domain.PresetName {
	names := make([]domain.PresetName, 0, len(r.presets))
	for name := range r.presets {
		names = append(names, name)
	}
	return names
}
