package ports

import (
	"context"

	"keepstar-admin/internal/domain"
)

// CanvasPort is the repository boundary for the KeepstarCanvas editor.
// All methods are tenant-scoped — the usecase layer pulls tenantID from
// the JWT middleware and passes it down here.
type CanvasPort interface {
	// --- presets ---

	ListPresets(ctx context.Context, tenantID string) ([]domain.TenantPreset, error)
	GetPreset(ctx context.Context, tenantID string, presetID string) (*domain.TenantPreset, error)
	GetPresetByName(ctx context.Context, tenantID string, name string) (*domain.TenantPreset, error)
	CreatePreset(ctx context.Context, tenantID string, in domain.PresetCreateInput, authorUserID string) (*domain.TenantPreset, error)
	UpdatePresetMetadata(ctx context.Context, tenantID string, presetID string, in domain.PresetUpdateInput) (*domain.TenantPreset, error)
	// SaveDraftOps creates (or updates) a draft version for a preset with the
	// given ops. If the current latest version is already draft, it's mutated
	// in place; otherwise a new draft row is forked.
	SaveDraftOps(ctx context.Context, tenantID string, presetID string, ops []byte, authorUserID string) (*domain.PresetVersion, error)
	PublishPreset(ctx context.Context, tenantID string, presetID string) (*domain.PresetVersion, error)
	ForkPreset(ctx context.Context, tenantID string, presetID string, authorUserID string) (*domain.PresetVersion, error)
	DeletePreset(ctx context.Context, tenantID string, presetID string) error
	// GetLatestPublishedOps returns the ops of the newest published version,
	// or (nil, false, nil) if the preset has no published version yet.
	GetLatestPublishedOps(ctx context.Context, tenantID string, name string) ([]byte, bool, error)

	// --- components ---

	ListComponents(ctx context.Context, tenantID string) ([]domain.TenantComponent, error)
	GetComponent(ctx context.Context, tenantID string, componentID string) (*domain.TenantComponent, error)
	CreateComponent(ctx context.Context, tenantID string, in domain.ComponentCreateInput, authorUserID string) (*domain.TenantComponent, error)
	UpdateComponentMetadata(ctx context.Context, tenantID string, componentID string, in domain.ComponentUpdateInput) (*domain.TenantComponent, error)
	SaveComponentDraftOps(ctx context.Context, tenantID string, componentID string, ops []byte, authorUserID string) (*domain.ComponentVersion, error)
	PublishComponent(ctx context.Context, tenantID string, componentID string) (*domain.ComponentVersion, error)
	DeleteComponent(ctx context.Context, tenantID string, componentID string) error

	// --- design tokens ---

	ListDesignTokens(ctx context.Context, tenantID string) ([]domain.DesignToken, error)
	UpsertDesignToken(ctx context.Context, tenantID string, in domain.DesignTokenUpsertInput) (*domain.DesignToken, error)
	DeleteDesignToken(ctx context.Context, tenantID string, tokenID string) error

	// BumpDesignContextVersion increments the per-tenant version counter that
	// V4's agent2 prompt cache keys on. Called by the usecase after any
	// publish so a fresh chat session picks up the new design immediately.
	BumpDesignContextVersion(ctx context.Context, tenantID string) (int64, error)
	GetDesignContextVersion(ctx context.Context, tenantID string) (int64, error)
}
