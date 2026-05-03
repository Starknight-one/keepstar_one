package adapters

// V5 chat / trace inspection — read-side only. Curator never writes
// to v5_*. The same Neon DB underpins V5, V4, admin, and Curator;
// this file just runs SELECTs against tables V5 owns.
//
// Three queries:
//   ListChats(filter)       — paginated session list with aggregates
//   GetChatTimeline(sessID) — session header + per-turn rows
//   GetChatTurn(sessID, rid) — full turn detail incl. spans JSONB

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ChatRow is one row in the Curator chats list. Aggregates come from
// v5_chat_session_traces; tenant info from catalog.tenants.
type ChatRow struct {
	SessionID      string    `json:"sessionId"`
	TenantID       string    `json:"tenantId"`
	TenantSlug     string    `json:"tenantSlug"`
	TenantName     string    `json:"tenantName"`
	CreatedAt      time.Time `json:"createdAt"`
	LastActivityAt time.Time `json:"lastActivityAt"`
	TurnCount      int       `json:"turnCount"`
	TotalCostUSD   float64   `json:"totalCostUsd"`
	TotalTokens    int64     `json:"totalTokens"`
	HasErrors      bool      `json:"hasErrors"`
	Status         string    `json:"status"` // "active" | "closed"
	LatestQuery    string    `json:"latestQuery,omitempty"`
}

// ListChatsFilter narrows the SELECT. All fields optional.
type ListChatsFilter struct {
	TenantSlugOrID string // filters to one tenant if non-empty
	Status         string // "active" | "closed" | "" (all)
	Search         string // substring search on session_id / latest_query / tenant_slug
	Limit          int
	Offset         int
}

// ListChats returns chat sessions with aggregates. Sessions without
// any persisted trace yet (turn_count=0) are still listed so Curator
// can see live sessions that haven't completed their first turn.
func (c *Client) ListChats(ctx context.Context, f ListChatsFilter) ([]ChatRow, int, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}

	const cutoff = "30 minutes" // active = activity within last 30 min

	// Build WHERE clause dynamically. Args ordered: $1=tenantFilter,
	// $2=tenantFilter (slug-or-id), $3=search, $4=statusFilter,
	// $5=limit, $6=offset.
	var conditions []string
	args := []any{}

	tenantArg := strings.TrimSpace(f.TenantSlugOrID)
	if tenantArg != "" {
		args = append(args, tenantArg)
		conditions = append(conditions, fmt.Sprintf("(t.id::text = $%d OR t.slug = $%d)", len(args), len(args)))
	}

	search := strings.TrimSpace(f.Search)
	if search != "" {
		args = append(args, "%"+search+"%")
		conditions = append(conditions,
			fmt.Sprintf("(s.id::text ILIKE $%d OR t.slug ILIKE $%d OR latest.user_query ILIKE $%d)",
				len(args), len(args), len(args)))
	}

	switch strings.ToLower(strings.TrimSpace(f.Status)) {
	case "active":
		conditions = append(conditions, fmt.Sprintf("s.updated_at > NOW() - INTERVAL '%s'", cutoff))
	case "closed":
		conditions = append(conditions, fmt.Sprintf("s.updated_at <= NOW() - INTERVAL '%s'", cutoff))
	}

	whereSQL := ""
	if len(conditions) > 0 {
		whereSQL = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Aggregates from traces; latest user_query via DISTINCT ON.
	query := fmt.Sprintf(`
        WITH agg AS (
            SELECT
                tr.session_id,
                COUNT(*) AS turn_count,
                COALESCE(SUM(tr.cost_usd), 0) AS total_cost,
                COALESCE(SUM(tr.tokens_input + tr.tokens_output), 0) AS total_tokens,
                BOOL_OR(tr.status = 'error') AS has_errors,
                MAX(tr.created_at) AS last_trace_at
            FROM v5_chat_session_traces tr
            GROUP BY tr.session_id
        ),
        latest AS (
            SELECT DISTINCT ON (tr.session_id)
                tr.session_id, tr.user_query
            FROM v5_chat_session_traces tr
            ORDER BY tr.session_id, tr.created_at DESC
        )
        SELECT
            s.id::text, s.tenant_id::text,
            COALESCE(t.slug, ''), COALESCE(t.name, ''),
            s.created_at, s.updated_at,
            COALESCE(agg.turn_count, 0),
            COALESCE(agg.total_cost, 0),
            COALESCE(agg.total_tokens, 0),
            COALESCE(agg.has_errors, false),
            COALESCE(latest.user_query, ''),
            COUNT(*) OVER () AS total_count
        FROM v5_chat_sessions s
        LEFT JOIN catalog.tenants t ON t.id::text = s.tenant_id
        LEFT JOIN agg ON agg.session_id = s.id
        LEFT JOIN latest ON latest.session_id = s.id
        %s
        ORDER BY s.updated_at DESC
        LIMIT $%d OFFSET $%d
    `, whereSQL, len(args)+1, len(args)+2)

	args = append(args, f.Limit, f.Offset)

	rows, err := c.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query chats: %w", err)
	}
	defer rows.Close()

	out := []ChatRow{}
	total := 0
	for rows.Next() {
		var r ChatRow
		if err := rows.Scan(
			&r.SessionID, &r.TenantID, &r.TenantSlug, &r.TenantName,
			&r.CreatedAt, &r.LastActivityAt,
			&r.TurnCount, &r.TotalCostUSD, &r.TotalTokens, &r.HasErrors,
			&r.LatestQuery, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan chat row: %w", err)
		}
		if time.Since(r.LastActivityAt) <= 30*time.Minute {
			r.Status = "active"
		} else {
			r.Status = "closed"
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iter chats: %w", err)
	}
	return out, total, nil
}

// TurnSummary is one entry in the chat detail timeline. Excludes the
// full spans payload — that comes via GetChatTurn for waterfall view.
type TurnSummary struct {
	RequestID    string    `json:"requestId"`
	UserQuery    string    `json:"userQuery"`
	LatencyMs    int64     `json:"latencyMs"`
	Agent1Ms     int64     `json:"agent1Ms"`
	Agent2Ms     int64     `json:"agent2Ms"`
	TokensInput  int64     `json:"tokensInput"`
	TokensOutput int64     `json:"tokensOutput"`
	CostUSD      float64   `json:"costUsd"`
	Status       string    `json:"status"`
	ErrorMessage string    `json:"errorMessage,omitempty"`
	SpanCount    int       `json:"spanCount"`
	CreatedAt    time.Time `json:"createdAt"`
}

// ChatTimeline aggregates session header + ordered turns.
type ChatTimeline struct {
	Session      ChatRow       `json:"session"`
	Turns        []TurnSummary `json:"turns"`
	TotalCostUSD float64       `json:"totalCostUsd"`
	TotalTokens  int64         `json:"totalTokens"`
}

// GetChatTimeline returns the session's full ordered turn list
// (oldest first) plus the same header shape ListChats produces. Empty
// session (no traces yet) returns an empty Turns slice with the
// session row populated.
func (c *Client) GetChatTimeline(ctx context.Context, sessionID string) (ChatTimeline, error) {
	var tl ChatTimeline
	tl.Turns = []TurnSummary{}

	const headerQ = `
        SELECT
            s.id::text, s.tenant_id::text,
            COALESCE(t.slug, ''), COALESCE(t.name, ''),
            s.created_at, s.updated_at
        FROM v5_chat_sessions s
        LEFT JOIN catalog.tenants t ON t.id::text = s.tenant_id
        WHERE s.id = $1::uuid
    `
	if err := c.Pool.QueryRow(ctx, headerQ, sessionID).Scan(
		&tl.Session.SessionID, &tl.Session.TenantID,
		&tl.Session.TenantSlug, &tl.Session.TenantName,
		&tl.Session.CreatedAt, &tl.Session.LastActivityAt,
	); err != nil {
		return tl, fmt.Errorf("session header: %w", err)
	}
	if time.Since(tl.Session.LastActivityAt) <= 30*time.Minute {
		tl.Session.Status = "active"
	} else {
		tl.Session.Status = "closed"
	}

	const turnsQ = `
        SELECT
            request_id, user_query,
            latency_ms,
            COALESCE(agent1_ms, 0), COALESCE(agent2_ms, 0),
            COALESCE(tokens_input, 0), COALESCE(tokens_output, 0),
            COALESCE(cost_usd, 0),
            status, COALESCE(error_message, ''),
            span_count, created_at
        FROM v5_chat_session_traces
        WHERE session_id = $1::uuid
        ORDER BY created_at ASC
    `
	rows, err := c.Pool.Query(ctx, turnsQ, sessionID)
	if err != nil {
		return tl, fmt.Errorf("query turns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t TurnSummary
		if err := rows.Scan(
			&t.RequestID, &t.UserQuery,
			&t.LatencyMs, &t.Agent1Ms, &t.Agent2Ms,
			&t.TokensInput, &t.TokensOutput, &t.CostUSD,
			&t.Status, &t.ErrorMessage,
			&t.SpanCount, &t.CreatedAt,
		); err != nil {
			return tl, fmt.Errorf("scan turn: %w", err)
		}
		tl.Turns = append(tl.Turns, t)
		tl.TotalCostUSD += t.CostUSD
		tl.TotalTokens += t.TokensInput + t.TokensOutput
	}
	if err := rows.Err(); err != nil {
		return tl, fmt.Errorf("iter turns: %w", err)
	}
	tl.Session.TurnCount = len(tl.Turns)
	tl.Session.TotalCostUSD = tl.TotalCostUSD
	tl.Session.TotalTokens = tl.TotalTokens
	return tl, nil
}

// TurnDetail bundles one turn + the full spans tree as raw JSON
// (passed through to the frontend for waterfall rendering).
type TurnDetail struct {
	Turn  TurnSummary       `json:"turn"`
	Spans []json.RawMessage `json:"spans"`
}

// GetChatTurn fetches one turn including its full spans JSONB.
func (c *Client) GetChatTurn(ctx context.Context, sessionID, requestID string) (TurnDetail, error) {
	var td TurnDetail
	td.Spans = []json.RawMessage{}

	const q = `
        SELECT
            request_id, user_query,
            latency_ms,
            COALESCE(agent1_ms, 0), COALESCE(agent2_ms, 0),
            COALESCE(tokens_input, 0), COALESCE(tokens_output, 0),
            COALESCE(cost_usd, 0),
            status, COALESCE(error_message, ''),
            span_count, created_at,
            spans
        FROM v5_chat_session_traces
        WHERE session_id = $1::uuid AND request_id = $2
    `
	var spansJSON []byte
	if err := c.Pool.QueryRow(ctx, q, sessionID, requestID).Scan(
		&td.Turn.RequestID, &td.Turn.UserQuery,
		&td.Turn.LatencyMs, &td.Turn.Agent1Ms, &td.Turn.Agent2Ms,
		&td.Turn.TokensInput, &td.Turn.TokensOutput, &td.Turn.CostUSD,
		&td.Turn.Status, &td.Turn.ErrorMessage,
		&td.Turn.SpanCount, &td.Turn.CreatedAt,
		&spansJSON,
	); err != nil {
		return td, fmt.Errorf("get turn: %w", err)
	}

	var spans []json.RawMessage
	if len(spansJSON) > 0 {
		if err := json.Unmarshal(spansJSON, &spans); err != nil {
			return td, fmt.Errorf("unmarshal spans: %w", err)
		}
	}
	td.Spans = spans
	return td, nil
}
