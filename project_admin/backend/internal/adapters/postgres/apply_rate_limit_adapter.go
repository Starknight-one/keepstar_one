package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

// ApplyRateLimitAdapter answers "when did this tenant last successfully
// run apply?" by reading admin.tenant_action_log. Used by
// UpdateOrchestrator to enforce the ≤1 apply/day window on webhook-driven
// updates.
type ApplyRateLimitAdapter struct {
	client *Client
	log    *logger.Logger
}

func NewApplyRateLimitAdapter(client *Client, log *logger.Logger) *ApplyRateLimitAdapter {
	return &ApplyRateLimitAdapter{client: client, log: log}
}

var _ usecases.ApplyRateLimitPort = (*ApplyRateLimitAdapter)(nil)

func (a *ApplyRateLimitAdapter) LastAppliedAt(ctx context.Context, tenantID string) (time.Time, error) {
	if tenantID == "" {
		return time.Time{}, fmt.Errorf("rate limit: empty tenant_id")
	}
	var ts time.Time
	err := a.client.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(occurred_at), 'epoch'::timestamptz)
		FROM admin.tenant_action_log
		WHERE tenant_id = $1
		  AND action = 'apply'
		  AND status IN ('ok', 'warning')
	`, tenantID).Scan(&ts)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("rate limit query: %w", err)
	}
	// COALESCE with 'epoch' gives us a zero-meaning sentinel — translate
	// back to Go's zero time so callers can rely on .IsZero().
	if ts.Year() <= 1970 {
		return time.Time{}, nil
	}
	return ts, nil
}
