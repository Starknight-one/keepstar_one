// Package adapters — catalog_v2.go reads from the post-rebuild tables
// (inbox_items, tenant_action_log, agent_runs, tenant_catalog_schema) for
// the new curator tabs. Pure read; no writes to these tables from curator.
package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// InboxRow is a curator-facing summary of one catalog.inbox_items row.
type InboxRow struct {
	ID          string          `json:"id"`
	SourceKind  string          `json:"source_kind"`
	ExternalID  string          `json:"external_id"`
	PayloadHash string          `json:"payload_hash"`
	FetchedAt   time.Time       `json:"fetched_at"`
	AppliedAt   *time.Time      `json:"applied_at,omitempty"`
	Preview     json.RawMessage `json:"preview"` // first ~500 chars of raw
}

// InboxItemFull returns one inbox row with the full raw payload.
type InboxItemFull struct {
	InboxRow
	Raw json.RawMessage `json:"raw"`
}

// ActionLogEntry mirrors admin.tenant_action_log for the curator UI.
type ActionLogEntry struct {
	ID         int64           `json:"id"`
	Action     string          `json:"action"`
	Status     string          `json:"status"`
	Payload    json.RawMessage `json:"payload"`
	OccurredAt time.Time       `json:"occurred_at"`
}

// AgentRunSummary is what curator lists in the Agent Runs tab.
type AgentRunSummary struct {
	ID         string          `json:"id"`
	Trigger    string          `json:"trigger"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
	Status     string          `json:"status"`
	TokensIn   int             `json:"tokens_in"`
	TokensOut  int             `json:"tokens_out"`
	CostUSD    float64         `json:"cost_usd"`
	ArtifactID *string         `json:"artifact_id,omitempty"`
}

// AgentRunDetail extends summary with the tool call list.
type AgentRunDetail struct {
	AgentRunSummary
	TriggerPayload json.RawMessage `json:"trigger_payload"`
	ToolsCalled    json.RawMessage `json:"tools_called"`
}

// MappingArtifactView is the curator-visible representation of a tenant's
// current mapping artifact (whichever version is stored).
type MappingArtifactView struct {
	Status          string          `json:"status"`
	ArtifactVersion int             `json:"artifact_version"`
	DiscoveredAt    time.Time       `json:"discovered_at"`
	ValidatedAt     *time.Time      `json:"validated_at,omitempty"`
	Body            json.RawMessage `json:"body"` // raw mapping_artifact JSONB
}

// ----- inbox -----

const inboxPreviewBytes = 500

func (c *Client) ListInbox(ctx context.Context, tenantID string, limit, offset int) ([]InboxRow, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := c.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM catalog.inbox_items WHERE tenant_id = $1`, tenantID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("inbox count: %w", err)
	}

	rows, err := c.Pool.Query(ctx, `
		SELECT id::text, source_kind, external_id, payload_hash, fetched_at, applied_at,
		       LEFT(raw::text, $4)::jsonb AS preview
		FROM catalog.inbox_items
		WHERE tenant_id = $1
		ORDER BY fetched_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset, inboxPreviewBytes)
	if err != nil {
		return nil, 0, fmt.Errorf("inbox list: %w", err)
	}
	defer rows.Close()

	var out []InboxRow
	for rows.Next() {
		var r InboxRow
		if err := rows.Scan(&r.ID, &r.SourceKind, &r.ExternalID, &r.PayloadHash, &r.FetchedAt, &r.AppliedAt, &r.Preview); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (c *Client) GetInboxItem(ctx context.Context, itemID string) (*InboxItemFull, error) {
	row := c.Pool.QueryRow(ctx, `
		SELECT id::text, source_kind, external_id, payload_hash, fetched_at, applied_at, raw
		FROM catalog.inbox_items
		WHERE id = $1
	`, itemID)
	var r InboxItemFull
	if err := row.Scan(&r.ID, &r.SourceKind, &r.ExternalID, &r.PayloadHash, &r.FetchedAt, &r.AppliedAt, &r.Raw); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("not found")
		}
		return nil, fmt.Errorf("inbox get: %w", err)
	}
	r.Preview = r.Raw
	return &r, nil
}

// ----- action log -----

func (c *Client) ListActionLog(ctx context.Context, tenantID string, limit, offset int) ([]ActionLogEntry, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	var total int
	if err := c.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM admin.tenant_action_log WHERE tenant_id = $1`, tenantID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("action log count: %w", err)
	}
	rows, err := c.Pool.Query(ctx, `
		SELECT id, action, status, payload, occurred_at
		FROM admin.tenant_action_log
		WHERE tenant_id = $1
		ORDER BY occurred_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("action log list: %w", err)
	}
	defer rows.Close()

	var out []ActionLogEntry
	for rows.Next() {
		var e ActionLogEntry
		if err := rows.Scan(&e.ID, &e.Action, &e.Status, &e.Payload, &e.OccurredAt); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// ----- agent runs -----

func (c *Client) ListAgentRuns(ctx context.Context, tenantID string, limit, offset int) ([]AgentRunSummary, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var total int
	if err := c.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM admin.agent_runs WHERE tenant_id = $1`, tenantID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("agent runs count: %w", err)
	}
	rows, err := c.Pool.Query(ctx, `
		SELECT id::text, trigger, started_at, finished_at, status,
		       tokens_in, tokens_out, cost_usd, artifact_id::text
		FROM admin.agent_runs
		WHERE tenant_id = $1
		ORDER BY started_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("agent runs list: %w", err)
	}
	defer rows.Close()

	var out []AgentRunSummary
	for rows.Next() {
		var s AgentRunSummary
		var artifactID *string
		if err := rows.Scan(&s.ID, &s.Trigger, &s.StartedAt, &s.FinishedAt, &s.Status, &s.TokensIn, &s.TokensOut, &s.CostUSD, &artifactID); err != nil {
			return nil, 0, err
		}
		s.ArtifactID = artifactID
		out = append(out, s)
	}
	return out, total, rows.Err()
}

func (c *Client) GetAgentRun(ctx context.Context, runID string) (*AgentRunDetail, error) {
	row := c.Pool.QueryRow(ctx, `
		SELECT id::text, trigger, trigger_payload, started_at, finished_at, status,
		       tokens_in, tokens_out, cost_usd, tools_called, artifact_id::text
		FROM admin.agent_runs
		WHERE id = $1
	`, runID)
	var d AgentRunDetail
	var artifactID *string
	if err := row.Scan(&d.ID, &d.Trigger, &d.TriggerPayload, &d.StartedAt, &d.FinishedAt, &d.Status,
		&d.TokensIn, &d.TokensOut, &d.CostUSD, &d.ToolsCalled, &artifactID); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("not found")
		}
		return nil, fmt.Errorf("agent run get: %w", err)
	}
	d.ArtifactID = artifactID
	return &d, nil
}

// ----- mapping artifact -----

func (c *Client) GetMappingArtifact(ctx context.Context, tenantID string) (*MappingArtifactView, error) {
	row := c.Pool.QueryRow(ctx, `
		SELECT status, artifact_version, mapping_artifact, discovered_at, validated_at
		FROM catalog.tenant_catalog_schema
		WHERE tenant_id = $1
	`, tenantID)
	var v MappingArtifactView
	if err := row.Scan(&v.Status, &v.ArtifactVersion, &v.Body, &v.DiscoveredAt, &v.ValidatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("artifact get: %w", err)
	}
	return &v, nil
}
