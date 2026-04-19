package usecases

import (
	"context"
	"fmt"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// IntegrationsUseCase handles the tenant-facing lifecycle of per-source
// integrations (list, view, disconnect). OAuth flows and sync orchestration
// live in source-specific usecases (ShopifyUseCase, CSVImportUseCase) which
// share this one to persist their connection rows.
type IntegrationsUseCase struct {
	repo ports.IntegrationsPort
	log  *logger.Logger
}

func NewIntegrationsUseCase(repo ports.IntegrationsPort, log *logger.Logger) *IntegrationsUseCase {
	return &IntegrationsUseCase{repo: repo, log: log}
}

// ListForTenant returns integrations with credentials stripped for API
// response safety. Config map is preserved.
func (uc *IntegrationsUseCase) ListForTenant(ctx context.Context, tenantID string) ([]domain.Integration, error) {
	integrations, err := uc.repo.ListByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list integrations: %w", err)
	}
	// Strip credentials before handing off to API layer — belt-and-suspenders.
	for i := range integrations {
		integrations[i].Credentials = ""
	}
	return integrations, nil
}

func (uc *IntegrationsUseCase) Get(ctx context.Context, tenantID, id string) (*domain.Integration, error) {
	return uc.repo.Get(ctx, tenantID, id)
}

func (uc *IntegrationsUseCase) Disconnect(ctx context.Context, tenantID, id string) error {
	return uc.repo.Delete(ctx, tenantID, id)
}

// Repo gives sibling usecases (CSV, Shopify) direct access to the store — they
// need to seal credentials and manage OAuth state rows themselves.
func (uc *IntegrationsUseCase) Repo() ports.IntegrationsPort {
	return uc.repo
}

// StartOAuthStateSweeper launches a ticker that deletes expired rows every
// 15 minutes. Returns a stop func the caller can defer.
func (uc *IntegrationsUseCase) StartOAuthStateSweeper(ctx context.Context) func() {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := uc.repo.CleanupExpiredOAuthStates(ctx); err != nil {
					uc.log.Error("oauth_state_cleanup_failed", "error", err)
				} else if n > 0 {
					uc.log.Info("oauth_state_cleanup", "removed", n)
				}
			}
		}
	}()
	return func() { close(stop) }
}
