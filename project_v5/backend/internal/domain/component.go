package domain

import (
	"encoding/json"
	"time"
)

// ComponentStatus mirrors PresetStatus — same draft/published/archived
// lifecycle, separate enum so callers can't accidentally pass a
// preset-status into a component-API.
type ComponentStatus string

const (
	ComponentStatusDraft     ComponentStatus = "draft"
	ComponentStatusPublished ComponentStatus = "published"
	ComponentStatusArchived  ComponentStatus = "archived"
)

// Component is a reusable v9 RefNode target — a tiny engine.Document with
// (typically) one root that presets reference via {type:"ref",
// ref:"<component-root-id>"}. Header metadata in v5_components, immutable
// version body in v5_component_versions.
//
// DocumentJSON is held raw to keep the domain layer engine-agnostic
// (engine → domain dependency already exists via binding_to_map.go's
// ProductToMap; we don't want a cycle the other way).
type Component struct {
	ID           string          `json:"id"`
	TenantID     string          `json:"tenantId"`
	Name         string          `json:"name"`
	Category     string          `json:"category"`
	Description  string          `json:"description"`
	Version      int             `json:"version"`
	Status       ComponentStatus `json:"status"`
	DocumentJSON json.RawMessage `json:"document"`
	PublishedAt  *time.Time      `json:"publishedAt,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}
