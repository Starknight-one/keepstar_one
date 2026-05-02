package domain

import (
	"encoding/json"
	"time"
)

// PresetStatus is the three-state lifecycle copied from V4's KeepstarCanvas
// (which is itself flagged for removal — the V5 schema is the new
// source-of-truth that the future v9-canvas microservice will write).
type PresetStatus string

const (
	PresetStatusDraft     PresetStatus = "draft"
	PresetStatusPublished PresetStatus = "published"
	PresetStatusArchived  PresetStatus = "archived"
)

// Preset is the unit a tenant edits in the (future) v9-canvas microservice
// and that V5 reads at render time. Header metadata lives in v5_presets;
// the immutable doc body is one row of v5_preset_versions.
//
// DocumentJSON holds the raw engine.Document JSON. We keep it raw at the
// domain layer to avoid a domain → engine import cycle (binding_to_map.go
// already pulls domain into engine for ProductToMap). Callers in the
// engine/usecase layer unmarshal into engine.Document on demand.
type Preset struct {
	ID               string          `json:"id"`
	TenantID         string          `json:"tenantId"`
	Name             string          `json:"name"`
	Category         string          `json:"category"`
	EntityType       string          `json:"entityType"`
	Description      string          `json:"description"`
	DefaultReplicate bool            `json:"defaultReplicate"`
	Version          int             `json:"version"`
	Status           PresetStatus    `json:"status"`
	DocumentJSON     json.RawMessage `json:"document"`
	PublishedAt      *time.Time      `json:"publishedAt,omitempty"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}
