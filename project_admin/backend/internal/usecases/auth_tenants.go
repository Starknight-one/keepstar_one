package usecases

import (
	"context"
	"errors"
	"fmt"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// TenantsUseCase handles workspace listing + switching. A user can belong to
// many tenants via admin.user_tenants; Select() re-issues a session whose
// `tid` claim is the chosen tenant so downstream handlers scope queries
// correctly.
type TenantsUseCase struct {
	memberships ports.UserTenantsPort
	sessions    *SessionsUseCase
	users       ports.AuthPort
	log         *logger.Logger
}

func NewTenantsUseCase(m ports.UserTenantsPort, s *SessionsUseCase, u ports.AuthPort, log *logger.Logger) *TenantsUseCase {
	return &TenantsUseCase{memberships: m, sessions: s, users: u, log: log}
}

func (uc *TenantsUseCase) List(ctx context.Context, userID string) ([]ports.UserTenantMembership, error) {
	return uc.memberships.ListForUser(ctx, userID)
}

// Select verifies the user is a member of the target tenant and issues a new
// token pair scoped to it. Returns ErrInvalidCredentials if the user has no
// membership — we do not leak whether the tenant exists.
func (uc *TenantsUseCase) Select(ctx context.Context, userID, tenantID, ua, ip string) (*TokenPair, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id required")
	}
	role, ok, err := uc.memberships.HasMembership(ctx, userID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("membership check: %w", err)
	}
	if !ok {
		return nil, domain.ErrInvalidCredentials
	}
	user, err := uc.users.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return uc.sessions.IssueForTenant(ctx, user, tenantID, domain.AdminRole(role), ua, ip)
}
