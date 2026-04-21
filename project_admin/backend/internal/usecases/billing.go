package usecases

import (
	"context"
	"fmt"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/ports"
)

type BillingUseCase struct {
	billing ports.BillingPort
}

func NewBillingUseCase(billing ports.BillingPort) *BillingUseCase {
	return &BillingUseCase{billing: billing}
}

// Overview composes the /billing/overview response: plan metadata, cycle
// window, multiplier-adjusted usage, and the available-plans list in canonical
// order. The tenant is auto-provisioned by GetSubscription on first access.
func (uc *BillingUseCase) Overview(ctx context.Context, tenantID string) (*domain.BillingOverview, error) {
	sub, err := uc.billing.GetSubscription(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}

	plan, ok := domain.PlanCatalog[sub.Plan]
	if !ok {
		plan = domain.PlanCatalog["growth"]
	}

	apiTokens, costUSD, err := uc.billing.AggregateTokensForCycle(ctx, tenantID, sub.CycleStart, sub.CycleEnd)
	if err != nil {
		return nil, fmt.Errorf("aggregate tokens: %w", err)
	}

	multiplier := sub.TokenMultiplier
	if multiplier <= 0 {
		multiplier = 5
	}
	tokensDisplayed := int64(float64(apiTokens) * multiplier)

	available := make([]domain.Plan, 0, len(domain.PlanOrder))
	for _, id := range domain.PlanOrder {
		if p, ok := domain.PlanCatalog[id]; ok {
			available = append(available, p)
		}
	}

	return &domain.BillingOverview{
		Plan: plan,
		Subscription: domain.BillingSubView{
			Status:     sub.Status,
			CycleStart: sub.CycleStart,
			CycleEnd:   sub.CycleEnd,
			RenewsAt:   sub.CycleEnd,
		},
		Usage: domain.BillingUsage{
			TokensUsed:      tokensDisplayed,
			TokensQuota:     plan.TokensPerMonth,
			APITokensActual: apiTokens,
			Multiplier:      multiplier,
			CostUSDActual:   costUSD,
		},
		Preferences: domain.Preferences{
			AlertThreshold:   sub.AlertThreshold,
			AlertSlack:       sub.AlertSlack,
			OverdraftEnabled: sub.OverdraftEnabled,
			OverdraftNotify:  sub.OverdraftNotify,
		},
		AvailablePlans: available,
	}, nil
}

func (uc *BillingUseCase) ListInvoices(ctx context.Context, tenantID string, limit int) ([]domain.Invoice, error) {
	return uc.billing.ListInvoices(ctx, tenantID, limit)
}

func (uc *BillingUseCase) UpdatePlan(ctx context.Context, tenantID, plan string) error {
	return uc.billing.UpdatePlan(ctx, tenantID, plan)
}

func (uc *BillingUseCase) UpdatePreferences(ctx context.Context, tenantID string, p domain.Preferences) error {
	return uc.billing.UpdatePreferences(ctx, tenantID, p)
}
