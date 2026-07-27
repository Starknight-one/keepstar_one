package postgres

import (
	"context"
	"fmt"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// NotificationAdapter implements ports.NotificationStore against
// public.v5_notifications (R12). Write path only in v1 — the notify
// executor is the sole caller, dispatched inline post-commit; read
// endpoints, mark-read and the unread badge are v2.
type NotificationAdapter struct {
	client *Client
}

// NewNotificationAdapter constructs the notification write store.
func NewNotificationAdapter(client *Client) *NotificationAdapter {
	return &NotificationAdapter{client: client}
}

var _ ports.NotificationStore = (*NotificationAdapter)(nil)

// CreateNotification inserts one row and fills n.ID / n.Audience /
// n.CreatedAt from the DB. Audience defaults to 'crm' (DDL default);
// title is required (NOT NULL and the only thing the CRM surface shows).
func (a *NotificationAdapter) CreateNotification(ctx context.Context, n *domain.Notification) error {
	if n == nil || n.TenantID == "" {
		return &domain.Error{Code: "NOTIFICATION_INVALID", Message: "notification missing tenant id"}
	}
	if n.Title == "" {
		return &domain.Error{Code: "NOTIFICATION_INVALID", Message: "notification missing title"}
	}
	err := a.client.pool.QueryRow(ctx, `
		INSERT INTO v5_notifications (tenant_id, audience, title, body, entity_slug, record_id)
		VALUES ($1::uuid, COALESCE(NULLIF($2, ''), 'crm'), $3, NULLIF($4, ''),
		        NULLIF($5, ''), NULLIF($6, '')::uuid)
		RETURNING id::text, audience, created_at
	`, n.TenantID, n.Audience, n.Title, n.Body, n.EntitySlug, n.RecordID).
		Scan(&n.ID, &n.Audience, &n.CreatedAt)
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}
	return nil
}
