package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"keepstar-admin/internal/domain"
)

// BillingAdapter implements ports.BillingPort on top of
// admin.billing_subscriptions / admin.billing_invoices, and reads token usage
// from pipeline_traces joined back through chat_sessions.
//
// chat_sessions.tenant_id is the slug (VARCHAR); admin middleware gives us the
// UUID; so every aggregation JOINs through catalog.tenants to bridge the two.
type BillingAdapter struct {
	client *Client
}

func NewBillingAdapter(client *Client) *BillingAdapter {
	return &BillingAdapter{client: client}
}

// GetSubscription returns the tenant row, inserting a default Growth-plan row
// with a 5× multiplier on first access. The default cycle starts at the first
// of the current month so UI has a non-zero cycle window out of the gate.
func (a *BillingAdapter) GetSubscription(ctx context.Context, tenantID string) (*domain.Subscription, error) {
	_, err := a.client.pool.Exec(ctx, `
		INSERT INTO admin.billing_subscriptions (tenant_id)
		VALUES ($1)
		ON CONFLICT (tenant_id) DO NOTHING
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("ensure subscription: %w", err)
	}

	var s domain.Subscription
	err = a.client.pool.QueryRow(ctx, `
		SELECT tenant_id::text, plan, status, cycle_start, cycle_end,
		       token_multiplier, alert_threshold, alert_slack,
		       overdraft_enabled, overdraft_notify, updated_at
		FROM admin.billing_subscriptions
		WHERE tenant_id = $1
	`, tenantID).Scan(
		&s.TenantID, &s.Plan, &s.Status, &s.CycleStart, &s.CycleEnd,
		&s.TokenMultiplier, &s.AlertThreshold, &s.AlertSlack,
		&s.OverdraftEnabled, &s.OverdraftNotify, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("subscription missing after insert: %w", err)
		}
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	return &s, nil
}

func (a *BillingAdapter) UpdatePlan(ctx context.Context, tenantID, plan string) error {
	if !domain.IsValidPlan(plan) {
		return fmt.Errorf("invalid plan: %s", plan)
	}
	_, err := a.client.pool.Exec(ctx, `
		INSERT INTO admin.billing_subscriptions (tenant_id, plan, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (tenant_id) DO UPDATE
		   SET plan = EXCLUDED.plan, updated_at = now()
	`, tenantID, plan)
	if err != nil {
		return fmt.Errorf("update plan: %w", err)
	}
	return nil
}

func (a *BillingAdapter) UpdatePreferences(ctx context.Context, tenantID string, p domain.Preferences) error {
	if p.AlertThreshold < 0 || p.AlertThreshold > 100 {
		return fmt.Errorf("alert threshold out of range")
	}
	_, err := a.client.pool.Exec(ctx, `
		INSERT INTO admin.billing_subscriptions (tenant_id, alert_threshold, alert_slack, overdraft_enabled, overdraft_notify, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (tenant_id) DO UPDATE
		   SET alert_threshold   = EXCLUDED.alert_threshold,
		       alert_slack       = EXCLUDED.alert_slack,
		       overdraft_enabled = EXCLUDED.overdraft_enabled,
		       overdraft_notify  = EXCLUDED.overdraft_notify,
		       updated_at        = now()
	`, tenantID, p.AlertThreshold, p.AlertSlack, p.OverdraftEnabled, p.OverdraftNotify)
	if err != nil {
		return fmt.Errorf("update preferences: %w", err)
	}
	return nil
}

func (a *BillingAdapter) ListInvoices(ctx context.Context, tenantID string, limit int) ([]domain.Invoice, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, tenant_id::text, period_start, period_end,
		       tokens_used, amount_usd, status, pdf_url, created_at
		FROM admin.billing_invoices
		WHERE tenant_id = $1
		ORDER BY period_start DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}
	defer rows.Close()

	var items []domain.Invoice
	for rows.Next() {
		var inv domain.Invoice
		if err := rows.Scan(
			&inv.ID, &inv.TenantID, &inv.PeriodStart, &inv.PeriodEnd,
			&inv.TokensUsed, &inv.AmountUSD, &inv.Status, &inv.PDFURL, &inv.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan invoice: %w", err)
		}
		items = append(items, inv)
	}
	return items, nil
}

// AggregateTokensForCycle sums agent1+agent2 input/output tokens and cost from
// pipeline_traces for the tenant's current cycle. chat_sessions.tenant_id is
// the slug, so we bridge through catalog.tenants to the admin UUID tenantID.
//
// Returns (apiTokens, costUSD). Multiplier is applied downstream.
func (a *BillingAdapter) AggregateTokensForCycle(ctx context.Context, tenantID string, start, end time.Time) (int64, float64, error) {
	var tokens int64
	var cost float64
	err := a.client.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(
				COALESCE((pt.trace_data->'agent1'->>'inputTokens')::bigint, 0) +
				COALESCE((pt.trace_data->'agent1'->>'outputTokens')::bigint, 0) +
				COALESCE((pt.trace_data->'agent2'->>'inputTokens')::bigint, 0) +
				COALESCE((pt.trace_data->'agent2'->>'outputTokens')::bigint, 0)
			), 0) AS total_tokens,
			COALESCE(SUM(pt.cost_usd), 0) AS total_cost
		FROM pipeline_traces pt
		JOIN chat_sessions cs ON cs.id::text = pt.session_id
		JOIN catalog.tenants t ON t.slug = cs.tenant_id
		WHERE t.id = $1
		  AND pt.timestamp >= $2
		  AND pt.timestamp <  $3
	`, tenantID, start, end).Scan(&tokens, &cost)
	if err != nil {
		return 0, 0, fmt.Errorf("aggregate tokens: %w", err)
	}
	return tokens, cost, nil
}
