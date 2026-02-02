package ports

import (
	"context"

	"keepstar/internal/domain"
)

// EventPort defines the interface for tracking chat events
type EventPort interface {
	// TrackEvent records a chat event
	TrackEvent(ctx context.Context, event *domain.ChatEvent) error

	// GetSessionEvents returns all events for a session
	GetSessionEvents(ctx context.Context, sessionID string) ([]domain.ChatEvent, error)
}
