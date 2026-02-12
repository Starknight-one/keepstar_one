package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"keepstar/internal/domain"
)

// LogAdapter handles writing request logs to PostgreSQL
type LogAdapter struct {
	client *Client
}

// NewLogAdapter creates a new LogAdapter
func NewLogAdapter(client *Client) *LogAdapter {
	return &LogAdapter{client: client}
}

// RequestLog represents a single HTTP request log entry
type RequestLog struct {
	ID         string
	Service    string
	Method     string
	Path       string
	Status     int
	DurationMs int64
	SessionID  string
	TenantSlug string
	UserID     string
	Error      string
	Spans      []domain.Span
	Metadata   map[string]any
}

// RecordRequestLog inserts a request log into the database.
// Designed to be called as fire-and-forget (goroutine).
func (a *LogAdapter) RecordRequestLog(ctx context.Context, log *RequestLog) error {
	var spansJSON, metadataJSON []byte
	var err error

	if len(log.Spans) > 0 {
		spansJSON, err = json.Marshal(log.Spans)
		if err != nil {
			return fmt.Errorf("marshal spans: %w", err)
		}
	}

	if len(log.Metadata) > 0 {
		metadataJSON, err = json.Marshal(log.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
	}

	var sessionID, tenantSlug, userID, errStr *string
	if log.SessionID != "" {
		sessionID = &log.SessionID
	}
	if log.TenantSlug != "" {
		tenantSlug = &log.TenantSlug
	}
	if log.UserID != "" {
		userID = &log.UserID
	}
	if log.Error != "" {
		errStr = &log.Error
	}

	_, err = a.client.pool.Exec(ctx, `
		INSERT INTO request_logs (id, service, method, path, status, duration_ms, session_id, tenant_slug, user_id, error, spans, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, log.ID, log.Service, log.Method, log.Path, log.Status, log.DurationMs,
		sessionID, tenantSlug, userID, errStr, spansJSON, metadataJSON)
	if err != nil {
		return fmt.Errorf("insert request log: %w", err)
	}

	return nil
}
