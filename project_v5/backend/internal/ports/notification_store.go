package ports

import (
	"context"

	"keepstar_v5/internal/domain"
)

// NotificationStore writes public.v5_notifications rows (R12). The notify
// executor is the only caller in v1; dispatch is inline post-commit. The
// read side (list endpoints, mark-read, unread badge) is v2.
type NotificationStore interface {
	CreateNotification(ctx context.Context, n *domain.Notification) error
}
