package engine

import (
	"keepstar/internal/domain"
)

// AgentInstructions represents the parsed parameters from Agent 2's tool call.
// This is the v2 equivalent of the visual_assembly tool parameters.
type AgentInstructions struct {
	Preset string   `json:"preset,omitempty"` // Preset name to use as base
	Show   []string `json:"show,omitempty"`   // Fields to show (by field name or label)
	Hide   []string `json:"hide,omitempty"`   // Fields to hide
	Order  []string `json:"order,omitempty"`  // Field display order

	// Per-atom overrides (keyed by field name or label)
	Atoms map[string]AtomOverride `json:"atoms,omitempty"`

	// Layout/formation overrides
	Layout    string `json:"layout,omitempty"`    // "grid","list","single","carousel","comparison"
	Size      string `json:"size,omitempty"`      // "tiny","small","medium","large"
	Direction string `json:"direction,omitempty"` // "vertical","horizontal"

	// Pagination
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// AtomOverride holds per-atom styling overrides from the agent.
type AtomOverride struct {
	TextStyle *domain.TextStyle     `json:"textStyle,omitempty"`
	Wrapper   *domain.WrapperConfig `json:"wrapper,omitempty"`
	Format    string                `json:"format,omitempty"`
	Color     string                `json:"color,omitempty"`
	Rigidity  domain.Rigidity       `json:"rigidity,omitempty"`
}

// PresetV2 and PresetV2Field are defined in domain/preset_v2_entity.go
// to avoid import cycles (presets package imports engine).
