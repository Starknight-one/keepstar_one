package postgres

import (
	"context"
	"fmt"
	"time"
)

// RetentionService handles periodic cleanup of old data
type RetentionService struct {
	client *Client
	config RetentionConfig
}

// RetentionConfig defines TTL and limits for retention policy
type RetentionConfig struct {
	TraceMaxAge         time.Duration // Delete traces older than this (default: 48h)
	DeadSessionMaxAge   time.Duration // Delete dead session data older than this (default: 1h)
	ConversationMaxMsgs int           // Keep last N messages in conversation_history (default: 20)
	CleanupInterval     time.Duration // How often to run cleanup (default: 30min)
	RequestLogMaxAge    time.Duration // Delete request_logs older than this (default: 72h)
}

// DefaultRetentionConfig returns sensible defaults
func DefaultRetentionConfig() RetentionConfig {
	return RetentionConfig{
		TraceMaxAge:         48 * time.Hour,
		DeadSessionMaxAge:   1 * time.Hour,
		ConversationMaxMsgs: 20,
		CleanupInterval:     30 * time.Minute,
		RequestLogMaxAge:    72 * time.Hour,
	}
}

// NewRetentionService creates a new retention service
func NewRetentionService(client *Client, config RetentionConfig) *RetentionService {
	return &RetentionService{client: client, config: config}
}

// Start begins the periodic cleanup loop. Blocks until ctx is cancelled.
func (s *RetentionService) Start(ctx context.Context, logFn func(msg string, args ...interface{})) {
	// Run once on startup
	s.runCleanup(ctx, logFn)

	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runCleanup(ctx, logFn)
		}
	}
}

func (s *RetentionService) runCleanup(ctx context.Context, logFn func(string, ...interface{})) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	tracesDeleted, err := s.cleanupTraces(ctx)
	if err != nil {
		logFn("retention_traces_error", "error", err)
	} else if tracesDeleted > 0 {
		logFn("retention_traces_cleaned", "deleted", tracesDeleted)
	}

	sessionsDeleted, err := s.cleanupDeadSessions(ctx)
	if err != nil {
		logFn("retention_sessions_error", "error", err)
	} else if sessionsDeleted > 0 {
		logFn("retention_sessions_cleaned", "deleted", sessionsDeleted)
	}

	trimmed, err := s.trimConversationHistory(ctx)
	if err != nil {
		logFn("retention_history_error", "error", err)
	} else if trimmed > 0 {
		logFn("retention_history_trimmed", "sessions", trimmed)
	}

	logsDeleted, err := s.cleanupRequestLogs(ctx)
	if err != nil {
		logFn("retention_request_logs_error", "error", err)
	} else if logsDeleted > 0 {
		logFn("retention_request_logs_cleaned", "deleted", logsDeleted)
	}
}

// cleanupTraces deletes pipeline_traces older than TraceMaxAge
func (s *RetentionService) cleanupTraces(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-s.config.TraceMaxAge)
	result, err := s.client.pool.Exec(ctx,
		`DELETE FROM pipeline_traces WHERE timestamp < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete old traces: %w", err)
	}
	return result.RowsAffected(), nil
}

// cleanupDeadSessions deletes all data for closed sessions older than DeadSessionMaxAge
func (s *RetentionService) cleanupDeadSessions(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-s.config.DeadSessionMaxAge)

	rows, err := s.client.pool.Query(ctx,
		`SELECT id FROM chat_sessions WHERE status = 'closed' AND ended_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("find dead sessions: %w", err)
	}
	defer rows.Close()

	var sessionIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, fmt.Errorf("scan session id: %w", err)
		}
		sessionIDs = append(sessionIDs, id)
	}

	if len(sessionIDs) == 0 {
		return 0, nil
	}

	// Delete in correct order for FK constraints
	tables := []string{
		"chat_session_deltas",
		"chat_session_state",
		"chat_messages",
		"pipeline_traces",
		"chat_sessions",
	}
	for _, sid := range sessionIDs {
		for _, table := range tables {
			col := "session_id"
			if table == "chat_sessions" {
				col = "id"
			}
			if _, err := s.client.pool.Exec(ctx,
				fmt.Sprintf(`DELETE FROM %s WHERE %s = $1`, table, col), sid); err != nil {
				return 0, fmt.Errorf("delete %s for %s: %w", table, sid, err)
			}
		}
	}

	return int64(len(sessionIDs)), nil
}

// cleanupRequestLogs deletes request_logs older than RequestLogMaxAge
func (s *RetentionService) cleanupRequestLogs(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-s.config.RequestLogMaxAge)
	result, err := s.client.pool.Exec(ctx,
		`DELETE FROM request_logs WHERE timestamp < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete old request logs: %w", err)
	}
	return result.RowsAffected(), nil
}

// trimConversationHistory keeps only the last N messages for sessions with oversized history.
// conversation_history is the biggest space consumer (~60% of DB size).
func (s *RetentionService) trimConversationHistory(ctx context.Context) (int64, error) {
	maxMsgs := s.config.ConversationMaxMsgs
	if maxMsgs <= 0 {
		return 0, nil
	}

	// Keep last N elements of the JSONB array using OFFSET to skip the old ones.
	// Use CASE to protect jsonb_array_length from being called on non-array values
	// (PostgreSQL can evaluate WHERE conditions in any order).
	result, err := s.client.pool.Exec(ctx, `
		WITH candidates AS (
			SELECT id, conversation_history,
			       CASE WHEN jsonb_typeof(conversation_history) = 'array'
			            THEN jsonb_array_length(conversation_history)
			            ELSE 0
			       END AS arr_len
			FROM chat_session_state
			WHERE conversation_history IS NOT NULL
			  AND jsonb_typeof(conversation_history) = 'array'
		)
		UPDATE chat_session_state s
		SET conversation_history = (
			SELECT COALESCE(jsonb_agg(elem), '[]'::jsonb)
			FROM (
				SELECT elem
				FROM jsonb_array_elements(c.conversation_history) WITH ORDINALITY AS t(elem, ord)
				ORDER BY ord
				OFFSET GREATEST(0, c.arr_len - $1)
			) sub
		),
		updated_at = NOW()
		FROM candidates c
		WHERE s.id = c.id AND c.arr_len > $1
	`, maxMsgs)
	if err != nil {
		return 0, fmt.Errorf("trim conversation history: %w", err)
	}
	return result.RowsAffected(), nil
}
