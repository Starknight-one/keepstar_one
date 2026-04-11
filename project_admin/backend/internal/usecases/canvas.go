package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/ports"
)

// CanvasUseCase orchestrates the KeepstarCanvas editor: preset/component
// CRUD, draft/publish transitions, and the per-tenant design-context version
// counter that busts Agent2 prompt cache.
type CanvasUseCase struct {
	repo ports.CanvasPort
}

func NewCanvasUseCase(repo ports.CanvasPort) *CanvasUseCase {
	return &CanvasUseCase{repo: repo}
}

// --- presets ---

func (uc *CanvasUseCase) ListPresets(ctx context.Context, tenantID string) ([]domain.TenantPreset, error) {
	return uc.repo.ListPresets(ctx, tenantID)
}

func (uc *CanvasUseCase) GetPreset(ctx context.Context, tenantID, presetID string) (*domain.TenantPreset, error) {
	return uc.repo.GetPreset(ctx, tenantID, presetID)
}

func (uc *CanvasUseCase) CreatePreset(ctx context.Context, tenantID, authorUserID string, in domain.PresetCreateInput) (*domain.TenantPreset, error) {
	if err := validatePresetName(in.Name); err != nil {
		return nil, err
	}
	if err := validateOps(in.Ops); err != nil {
		return nil, err
	}
	return uc.repo.CreatePreset(ctx, tenantID, in, authorUserID)
}

// UpdatePreset applies a metadata update AND, when ops are supplied, a new
// draft revision. The two live on separate tables so we call each only when
// needed — metadata on the header row, ops on a version row.
func (uc *CanvasUseCase) UpdatePreset(ctx context.Context, tenantID, presetID, authorUserID string, in domain.PresetUpdateInput) (*domain.TenantPreset, error) {
	if in.Name != nil {
		if err := validatePresetName(*in.Name); err != nil {
			return nil, err
		}
	}
	if len(in.Ops) > 0 {
		if err := validateOps(in.Ops); err != nil {
			return nil, err
		}
	}
	if _, err := uc.repo.UpdatePresetMetadata(ctx, tenantID, presetID, in); err != nil {
		return nil, err
	}
	if len(in.Ops) > 0 {
		if _, err := uc.repo.SaveDraftOps(ctx, tenantID, presetID, in.Ops, authorUserID); err != nil {
			return nil, err
		}
	}
	return uc.repo.GetPreset(ctx, tenantID, presetID)
}

func (uc *CanvasUseCase) PublishPreset(ctx context.Context, tenantID, presetID string) (*domain.TenantPreset, error) {
	if _, err := uc.repo.PublishPreset(ctx, tenantID, presetID); err != nil {
		return nil, err
	}
	// Bump design-context version so V4 agent2 prompt cache invalidates for
	// this tenant on the next chat turn.
	if _, err := uc.repo.BumpDesignContextVersion(ctx, tenantID); err != nil {
		return nil, fmt.Errorf("bump design context after publish: %w", err)
	}
	return uc.repo.GetPreset(ctx, tenantID, presetID)
}

func (uc *CanvasUseCase) ForkPreset(ctx context.Context, tenantID, presetID, authorUserID string) (*domain.TenantPreset, error) {
	if _, err := uc.repo.ForkPreset(ctx, tenantID, presetID, authorUserID); err != nil {
		return nil, err
	}
	return uc.repo.GetPreset(ctx, tenantID, presetID)
}

func (uc *CanvasUseCase) DeletePreset(ctx context.Context, tenantID, presetID string) error {
	if err := uc.repo.DeletePreset(ctx, tenantID, presetID); err != nil {
		return err
	}
	// Removing a preset changes what the tenant exposes to agent2 — bump the
	// design-context version so the prompt block rebuilds without the deleted
	// preset's description.
	_, _ = uc.repo.BumpDesignContextVersion(ctx, tenantID)
	return nil
}

// --- components ---

func (uc *CanvasUseCase) ListComponents(ctx context.Context, tenantID string) ([]domain.TenantComponent, error) {
	return uc.repo.ListComponents(ctx, tenantID)
}

func (uc *CanvasUseCase) GetComponent(ctx context.Context, tenantID, componentID string) (*domain.TenantComponent, error) {
	return uc.repo.GetComponent(ctx, tenantID, componentID)
}

func (uc *CanvasUseCase) CreateComponent(ctx context.Context, tenantID, authorUserID string, in domain.ComponentCreateInput) (*domain.TenantComponent, error) {
	if err := validatePresetName(in.Name); err != nil {
		return nil, err
	}
	if err := validateOps(in.Ops); err != nil {
		return nil, err
	}
	return uc.repo.CreateComponent(ctx, tenantID, in, authorUserID)
}

func (uc *CanvasUseCase) UpdateComponent(ctx context.Context, tenantID, componentID, authorUserID string, in domain.ComponentUpdateInput) (*domain.TenantComponent, error) {
	if in.Name != nil {
		if err := validatePresetName(*in.Name); err != nil {
			return nil, err
		}
	}
	if len(in.Ops) > 0 {
		if err := validateOps(in.Ops); err != nil {
			return nil, err
		}
	}
	if _, err := uc.repo.UpdateComponentMetadata(ctx, tenantID, componentID, in); err != nil {
		return nil, err
	}
	if len(in.Ops) > 0 {
		if _, err := uc.repo.SaveComponentDraftOps(ctx, tenantID, componentID, in.Ops, authorUserID); err != nil {
			return nil, err
		}
	}
	return uc.repo.GetComponent(ctx, tenantID, componentID)
}

func (uc *CanvasUseCase) PublishComponent(ctx context.Context, tenantID, componentID string) (*domain.TenantComponent, error) {
	if _, err := uc.repo.PublishComponent(ctx, tenantID, componentID); err != nil {
		return nil, err
	}
	if _, err := uc.repo.BumpDesignContextVersion(ctx, tenantID); err != nil {
		return nil, fmt.Errorf("bump design context after component publish: %w", err)
	}
	return uc.repo.GetComponent(ctx, tenantID, componentID)
}

func (uc *CanvasUseCase) DeleteComponent(ctx context.Context, tenantID, componentID string) error {
	if err := uc.repo.DeleteComponent(ctx, tenantID, componentID); err != nil {
		return err
	}
	_, _ = uc.repo.BumpDesignContextVersion(ctx, tenantID)
	return nil
}

// --- design tokens ---

func (uc *CanvasUseCase) ListTokens(ctx context.Context, tenantID string) ([]domain.DesignToken, error) {
	return uc.repo.ListDesignTokens(ctx, tenantID)
}

func (uc *CanvasUseCase) UpsertToken(ctx context.Context, tenantID string, in domain.DesignTokenUpsertInput) (*domain.DesignToken, error) {
	if strings.TrimSpace(in.Category) == "" || strings.TrimSpace(in.Name) == "" {
		return nil, errors.New("category and name are required")
	}
	tok, err := uc.repo.UpsertDesignToken(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	_, _ = uc.repo.BumpDesignContextVersion(ctx, tenantID)
	return tok, nil
}

func (uc *CanvasUseCase) DeleteToken(ctx context.Context, tenantID, tokenID string) error {
	if err := uc.repo.DeleteDesignToken(ctx, tenantID, tokenID); err != nil {
		return err
	}
	_, _ = uc.repo.BumpDesignContextVersion(ctx, tenantID)
	return nil
}

// --- validation helpers ---

var errInvalidName = errors.New("name must be lowercase_snake, 2-200 chars")

func validatePresetName(name string) error {
	n := strings.TrimSpace(name)
	if len(n) < 2 || len(n) > 200 {
		return errInvalidName
	}
	for _, r := range n {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return errInvalidName
	}
	return nil
}

func validateOps(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var arr []any
	if err := json.Unmarshal(raw, &arr); err != nil {
		return fmt.Errorf("ops must be a JSON array: %w", err)
	}
	return nil
}
