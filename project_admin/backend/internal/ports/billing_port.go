package ports

import (
	"context"
	"time"

	"keepstar-admin/internal/domain"
)

// BillingPort abstracts storage for subscriptions, invoices, and the
// token-aggregation query that drives "usage this cycle".
type BillingPort interface {
	// GetSubscription returns the tenant's row, auto-provisioning a default
	// Growth subscription on first access so the overview endpoint never
	// 404s for a logged-in tenant.
	GetSubscription(ctx context.Context, tenantID string) (*domain.Subscription, error)

	UpdatePlan(ctx context.Context, tenantID, plan string) error
	UpdatePreferences(ctx context.Context, tenantID string, prefs domain.Preferences) error

	ListInvoices(ctx context.Context, tenantID string, limit int) ([]domain.Invoice, error)

	// AggregateTokensForCycle returns the real API token count and USD
	// cost for the tenant between [start, end). The multiplier is NOT
	// applied here; the usecase decides how to present the number.
	AggregateTokensForCycle(ctx context.Context, tenantID string, start, end time.Time) (int64, float64, error)
}
