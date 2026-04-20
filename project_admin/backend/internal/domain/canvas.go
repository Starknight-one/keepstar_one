package domain

import (
	"encoding/json"
	"time"
)

// PresetStatus is the lifecycle state of a single preset/component version.
// draft: mutable, never served to the chat backend.
// published: immutable, served to agent2 until superseded.
// archived: hidden, kept for audit and session reproducibility.
type PresetStatus string

const (
	PresetStatusDraft     PresetStatus = "draft"
	PresetStatusPublished PresetStatus = "published"
	PresetStatusArchived  PresetStatus = "archived"
)

// TenantPreset is the preset header — stable identity, per-tenant, carries a
// pointer to its latest version. Ops live on the version rows, never here.
type TenantPreset struct {
	ID               string          `json:"id"`
	TenantID         string          `json:"tenantId"`
	Name             string          `json:"name"`
	Category         string          `json:"category"`
	DefaultReplicate bool            `json:"defaultReplicate"`
	EntityType       string          `json:"entityType"`
	Description      string          `json:"description"`
	LatestVersionID  string          `json:"latestVersionId,omitempty"`
	LatestVersion    *PresetVersion  `json:"latestVersion,omitempty"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

// PresetVersion is an immutable snapshot of preset ops. New edits fork a new
// row with status=draft. Publishing flips the status and stamps published_at.
type PresetVersion struct {
	ID           string          `json:"id"`
	PresetID     string          `json:"presetId"`
	Version      int             `json:"version"`
	Status       PresetStatus    `json:"status"`
	OpsJSON      json.RawMessage `json:"ops"`
	AuthorUserID string          `json:"authorUserId,omitempty"`
	PublishedAt  *time.Time      `json:"publishedAt,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
}

// TenantComponent mirrors TenantPreset for reusable sub-fragments (price_pill,
// rating_stars, ...). Presets reference components by ID via $ref in ops.
type TenantComponent struct {
	ID              string            `json:"id"`
	TenantID        string            `json:"tenantId"`
	Name            string            `json:"name"`
	Category        string            `json:"category"`
	Description     string            `json:"description"`
	LatestVersionID string            `json:"latestVersionId,omitempty"`
	LatestVersion   *ComponentVersion `json:"latestVersion,omitempty"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
}

// ComponentVersion is structurally identical to PresetVersion. Kept as a
// separate type so handlers/adapters don't cross-bleed between the two tables.
type ComponentVersion struct {
	ID           string          `json:"id"`
	ComponentID  string          `json:"componentId"`
	Version      int             `json:"version"`
	Status       PresetStatus    `json:"status"`
	OpsJSON      json.RawMessage `json:"ops"`
	AuthorUserID string          `json:"authorUserId,omitempty"`
	PublishedAt  *time.Time      `json:"publishedAt,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
}

// DesignToken is one named value in the tenant's design system. theme_axis +
// theme_value scope the token to an optional theme variant (e.g. halloween).
// Rows with empty theme_axis/theme_value form the default theme.
type DesignToken struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenantId"`
	Category   string    `json:"category"`
	Name       string    `json:"name"`
	Value      string    `json:"value"`
	ThemeAxis  string    `json:"themeAxis,omitempty"`
	ThemeValue string    `json:"themeValue,omitempty"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// --- input payloads used by the canvas HTTP handler ---

type PresetCreateInput struct {
	Name             string          `json:"name"`
	Category         string          `json:"category"`
	EntityType       string          `json:"entityType"`
	Description      string          `json:"description"`
	DefaultReplicate *bool           `json:"defaultReplicate,omitempty"`
	Ops              json.RawMessage `json:"ops,omitempty"`
}

type PresetUpdateInput struct {
	Name             *string         `json:"name,omitempty"`
	Category         *string         `json:"category,omitempty"`
	EntityType       *string         `json:"entityType,omitempty"`
	Description      *string         `json:"description,omitempty"`
	DefaultReplicate *bool           `json:"defaultReplicate,omitempty"`
	Ops              json.RawMessage `json:"ops,omitempty"`
}

type ComponentCreateInput struct {
	Name        string          `json:"name"`
	Category    string          `json:"category"`
	Description string          `json:"description"`
	Ops         json.RawMessage `json:"ops,omitempty"`
}

type ComponentUpdateInput struct {
	Name        *string         `json:"name,omitempty"`
	Category    *string         `json:"category,omitempty"`
	Description *string         `json:"description,omitempty"`
	Ops         json.RawMessage `json:"ops,omitempty"`
}

type DesignTokenUpsertInput struct {
	Category   string `json:"category"`
	Name       string `json:"name"`
	Value      string `json:"value"`
	ThemeAxis  string `json:"themeAxis,omitempty"`
	ThemeValue string `json:"themeValue,omitempty"`
}
