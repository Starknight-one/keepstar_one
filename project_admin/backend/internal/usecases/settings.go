package usecases

import (
	"context"
	"encoding/json"
	"fmt"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/ports"
)

type SettingsUseCase struct {
	catalog ports.AdminCatalogPort
}

func NewSettingsUseCase(catalog ports.AdminCatalogPort) *SettingsUseCase {
	return &SettingsUseCase{catalog: catalog}
}

func (uc *SettingsUseCase) Get(ctx context.Context, tenantID string) (*domain.TenantSettings, error) {
	tenant, err := uc.catalog.GetTenantByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	var settings domain.TenantSettings
	if tenant.Settings != nil {
		raw, _ := json.Marshal(tenant.Settings)
		json.Unmarshal(raw, &settings)
	}
	return &settings, nil
}

func (uc *SettingsUseCase) Update(ctx context.Context, tenantID string, settings domain.TenantSettings) error {
	if err := uc.catalog.UpdateTenantSettings(ctx, tenantID, settings); err != nil {
		return fmt.Errorf("update settings: %w", err)
	}
	return nil
}
