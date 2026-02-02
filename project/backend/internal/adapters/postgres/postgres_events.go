package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"keepstar/internal/domain"
)

// EventAdapter implements ports.EventPort using PostgreSQL
type EventAdapter struct {
	client *Client
}

// NewEventAdapter creates a new PostgreSQL event adapter
func NewEventAdapter(client *Client) *EventAdapter {
	return &EventAdapter{client: client}
}

// TrackEvent records a chat event
func (a *EventAdapter) TrackEvent(ctx context.Context, event *domain.ChatEvent) error {
	eventDataJSON, err := json.Marshal(event.EventData)
	if err != nil {
		return fmt.Errorf("marshal event data: %w", err)
	}

	var sessionID, userID *string
	if event.SessionID != "" {
		sessionID = &event.SessionID
	}
	if event.UserID != "" {
		userID = &event.UserID
	}

	_, err = a.client.pool.Exec(ctx, `
		INSERT INTO chat_events (id, session_id, user_id, event_type, event_data, created_at)
		VALUES (
			CASE WHEN $1 = '' OR $1 IS NULL THEN gen_random_uuid() ELSE $1::uuid END,
			$2::uuid, $3::uuid, $4, $5, $6
		)
	`, event.ID, sessionID, userID, event.EventType, eventDataJSON, event.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	return nil
}

// GetSessionEvents returns all events for a session
func (a *EventAdapter) GetSessionEvents(ctx context.Context, sessionID string) ([]domain.ChatEvent, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, session_id, user_id, event_type, event_data, created_at
		FROM chat_events
		WHERE session_id = $1
		ORDER BY created_at ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []domain.ChatEvent
	for rows.Next() {
		var event domain.ChatEvent
		var sessionIDPtr, userIDPtr *string
		var eventDataJSON []byte

		err := rows.Scan(
			&event.ID,
			&sessionIDPtr,
			&userIDPtr,
			&event.EventType,
			&eventDataJSON,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		if sessionIDPtr != nil {
			event.SessionID = *sessionIDPtr
		}
		if userIDPtr != nil {
			event.UserID = *userIDPtr
		}

		if len(eventDataJSON) > 0 {
			if err := json.Unmarshal(eventDataJSON, &event.EventData); err != nil {
				return nil, fmt.Errorf("unmarshal event data: %w", err)
			}
		}

		events = append(events, event)
	}

	return events, nil
}
