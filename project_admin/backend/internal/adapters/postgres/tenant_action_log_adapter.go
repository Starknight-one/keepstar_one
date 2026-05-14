package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type TenantActionLogAdapter struct {
	client *Client
	log    *logger.Logger
}

func NewTenantActionLogAdapter(client *Client, log *logger.Logger) *TenantActionLogAdapter {
	return &TenantActionLogAdapter{client: client, log: log}
}

var _ ports.TenantActionLogPort = (*TenantActionLogAdapter)(nil)

func (a *TenantActionLogAdapter) Log(ctx context.Context, e *ports.TenantActionLogEntry) error {
	if e == nil {
		return fmt.Errorf("action log: nil entry")
	}
	if e.TenantID == "" || e.Action == "" || e.Status == "" {
		return fmt.Errorf("action log: tenant_id/action/status required")
	}
	payload := e.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	_, err := a.client.pool.Exec(ctx, `
		INSERT INTO admin.tenant_action_log (tenant_id, action, status, payload)
		VALUES ($1, $2, $3, $4)
	`, e.TenantID, e.Action, e.Status, payload)
	if err != nil {
		return fmt.Errorf("action log insert: %w", err)
	}
	return nil
}

func (a *TenantActionLogAdapter) List(ctx context.Context, tenantID string, limit, offset int) ([]*ports.TenantActionLogEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, tenant_id, action, status, payload, occurred_at
		FROM admin.tenant_action_log
		WHERE tenant_id = $1
		ORDER BY occurred_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("action log list: %w", err)
	}
	defer rows.Close()

	var out []*ports.TenantActionLogEntry
	for rows.Next() {
		e := &ports.TenantActionLogEntry{}
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Action, &e.Status, &e.Payload, &e.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
