package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"keepstar/internal/domain"
)

// CacheAdapter implements ports.CachePort using PostgreSQL
type CacheAdapter struct {
	client *Client
}

// NewCacheAdapter creates a new PostgreSQL cache adapter
func NewCacheAdapter(client *Client) *CacheAdapter {
	return &CacheAdapter{client: client}
}

// GetSession returns a session by ID with all its messages
func (a *CacheAdapter) GetSession(ctx context.Context, id string) (*domain.Session, error) {
	// Get session
	var session domain.Session
	var userID *string
	var endedAt *time.Time
	var metadataJSON []byte

	err := a.client.pool.QueryRow(ctx, `
		SELECT id, user_id, tenant_id, status, metadata, started_at, ended_at, last_activity_at, created_at, updated_at
		FROM chat_sessions
		WHERE id = $1
	`, id).Scan(
		&session.ID,
		&userID,
		&session.TenantID,
		&session.Status,
		&metadataJSON,
		&session.StartedAt,
		&endedAt,
		&session.LastActivityAt,
		&session.CreatedAt,
		&session.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, domain.ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query session: %w", err)
	}

	if userID != nil {
		session.UserID = *userID
	}
	session.EndedAt = endedAt

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &session.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal session metadata: %w", err)
		}
	}

	// Get messages
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, role, content, widgets, formation, tokens_used, model_used, latency_ms, sent_at, received_at, created_at
		FROM chat_messages
		WHERE session_id = $1
		ORDER BY sent_at ASC
	`, id)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var msg domain.Message
		var widgetsJSON, formationJSON []byte
		var tokensUsed, latencyMs *int
		var modelUsed *string
		var receivedAt *time.Time

		err := rows.Scan(
			&msg.ID,
			&msg.Role,
			&msg.Content,
			&widgetsJSON,
			&formationJSON,
			&tokensUsed,
			&modelUsed,
			&latencyMs,
			&msg.SentAt,
			&receivedAt,
			&msg.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}

		msg.SessionID = id
		msg.ReceivedAt = receivedAt

		if tokensUsed != nil {
			msg.TokensUsed = *tokensUsed
		}
		if modelUsed != nil {
			msg.ModelUsed = *modelUsed
		}
		if latencyMs != nil {
			msg.LatencyMs = *latencyMs
		}

		if len(widgetsJSON) > 0 {
			if err := json.Unmarshal(widgetsJSON, &msg.Widgets); err != nil {
				return nil, fmt.Errorf("unmarshal widgets: %w", err)
			}
		}
		if len(formationJSON) > 0 {
			if err := json.Unmarshal(formationJSON, &msg.Formation); err != nil {
				return nil, fmt.Errorf("unmarshal formation: %w", err)
			}
		}

		session.Messages = append(session.Messages, msg)
	}

	return &session, nil
}

// SaveSession saves or updates a session and its messages
func (a *CacheAdapter) SaveSession(ctx context.Context, session *domain.Session) error {
	tx, err := a.client.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	metadataJSON, err := json.Marshal(session.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	var userID *string
	if session.UserID != "" {
		userID = &session.UserID
	}

	// Upsert session
	_, err = tx.Exec(ctx, `
		INSERT INTO chat_sessions (id, user_id, tenant_id, status, metadata, started_at, ended_at, last_activity_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			status = EXCLUDED.status,
			metadata = EXCLUDED.metadata,
			ended_at = EXCLUDED.ended_at,
			last_activity_at = EXCLUDED.last_activity_at,
			updated_at = EXCLUDED.updated_at
	`, session.ID, userID, session.TenantID, session.Status, metadataJSON,
		session.StartedAt, session.EndedAt, session.LastActivityAt, session.CreatedAt, session.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert session: %w", err)
	}

	// Insert new messages
	for _, msg := range session.Messages {
		widgetsJSON, err := json.Marshal(msg.Widgets)
		if err != nil {
			return fmt.Errorf("marshal widgets: %w", err)
		}

		var formationJSON []byte
		if msg.Formation != nil {
			formationJSON, err = json.Marshal(msg.Formation)
			if err != nil {
				return fmt.Errorf("marshal formation: %w", err)
			}
		}

		var tokensUsed, latencyMs *int
		var modelUsed *string
		if msg.TokensUsed > 0 {
			tokensUsed = &msg.TokensUsed
		}
		if msg.ModelUsed != "" {
			modelUsed = &msg.ModelUsed
		}
		if msg.LatencyMs > 0 {
			latencyMs = &msg.LatencyMs
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO chat_messages (id, session_id, role, content, widgets, formation, tokens_used, model_used, latency_ms, sent_at, received_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT (id) DO NOTHING
		`, msg.ID, session.ID, msg.Role, msg.Content, widgetsJSON, formationJSON,
			tokensUsed, modelUsed, latencyMs, msg.SentAt, msg.ReceivedAt, msg.Timestamp)
		if err != nil {
			return fmt.Errorf("insert message: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// DeleteSession marks a session as closed (keeps traces and history for debug)
func (a *CacheAdapter) DeleteSession(ctx context.Context, id string) error {
	_, err := a.client.pool.Exec(ctx, `
		UPDATE chat_sessions SET status = 'closed', ended_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("close session: %w", err)
	}
	return nil
}

// CacheProducts stores products in session metadata
func (a *CacheAdapter) CacheProducts(ctx context.Context, sessionID string, products []domain.Product) error {
	productsJSON, err := json.Marshal(products)
	if err != nil {
		return fmt.Errorf("marshal products: %w", err)
	}

	_, err = a.client.pool.Exec(ctx, `
		UPDATE chat_sessions
		SET metadata = jsonb_set(COALESCE(metadata, '{}'), '{cached_products}', $1::jsonb),
		    updated_at = NOW()
		WHERE id = $2
	`, productsJSON, sessionID)
	if err != nil {
		return fmt.Errorf("cache products: %w", err)
	}

	return nil
}

// GetCachedProducts retrieves products from session metadata
func (a *CacheAdapter) GetCachedProducts(ctx context.Context, sessionID string) ([]domain.Product, error) {
	var metadataJSON []byte

	err := a.client.pool.QueryRow(ctx, `
		SELECT metadata->'cached_products'
		FROM chat_sessions
		WHERE id = $1
	`, sessionID).Scan(&metadataJSON)

	if err == pgx.ErrNoRows {
		return nil, domain.ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query cached products: %w", err)
	}

	if len(metadataJSON) == 0 || string(metadataJSON) == "null" {
		return nil, nil
	}

	var products []domain.Product
	if err := json.Unmarshal(metadataJSON, &products); err != nil {
		return nil, fmt.Errorf("unmarshal products: %w", err)
	}

	return products, nil
}
