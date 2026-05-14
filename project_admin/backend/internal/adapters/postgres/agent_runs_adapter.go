// Package postgres — AgentRunsAdapter persists discovery_v2 invocations
// to admin.agent_runs. Designed for write-mostly access during a live run,
// then read by curator for inspection. tools_called is appended via JSONB
// concatenation (no full-array rewrite) so concurrent reads stay safe.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type AgentRunsAdapter struct {
	client *Client
	log    *logger.Logger
}

func NewAgentRunsAdapter(client *Client, log *logger.Logger) *AgentRunsAdapter {
	return &AgentRunsAdapter{client: client, log: log}
}

var _ ports.AgentRunsPort = (*AgentRunsAdapter)(nil)

func (a *AgentRunsAdapter) Start(ctx context.Context, in *ports.AgentRunStart) (string, error) {
	if in == nil || in.TenantID == "" || in.Trigger == "" {
		return "", fmt.Errorf("agent run start: tenant_id and trigger required")
	}
	payload := in.TriggerPayload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	var id string
	err := a.client.pool.QueryRow(ctx, `
		INSERT INTO admin.agent_runs (tenant_id, trigger, trigger_payload, status)
		VALUES ($1, $2, $3, 'running')
		RETURNING id::text
	`, in.TenantID, in.Trigger, payload).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("agent run start: %w", err)
	}
	return id, nil
}

// AppendTool concatenates one new tool-call onto tools_called[]. Uses
// `jsonb || '[obj]'::jsonb` which is the safe append idiom for jsonb arrays.
func (a *AgentRunsAdapter) AppendTool(ctx context.Context, runID string, call ports.AgentToolCall) error {
	if runID == "" {
		return fmt.Errorf("agent run append: empty run_id")
	}
	b, err := json.Marshal(call)
	if err != nil {
		return fmt.Errorf("agent run append marshal: %w", err)
	}
	_, err = a.client.pool.Exec(ctx, `
		UPDATE admin.agent_runs
		SET tools_called = tools_called || $2::jsonb
		WHERE id = $1
	`, runID, "["+string(b)+"]")
	if err != nil {
		return fmt.Errorf("agent run append exec: %w", err)
	}
	return nil
}

func (a *AgentRunsAdapter) AddTokens(ctx context.Context, runID string, tokensIn, tokensOut int, costDeltaUSD float64) error {
	if runID == "" {
		return fmt.Errorf("agent run add tokens: empty run_id")
	}
	_, err := a.client.pool.Exec(ctx, `
		UPDATE admin.agent_runs
		SET tokens_in  = tokens_in  + $2,
		    tokens_out = tokens_out + $3,
		    cost_usd   = cost_usd   + $4
		WHERE id = $1
	`, runID, tokensIn, tokensOut, costDeltaUSD)
	if err != nil {
		return fmt.Errorf("agent run add tokens: %w", err)
	}
	return nil
}

func (a *AgentRunsAdapter) Finish(ctx context.Context, runID, status, artifactID string) error {
	if runID == "" || status == "" {
		return fmt.Errorf("agent run finish: run_id and status required")
	}
	var aid any
	if artifactID != "" {
		aid = artifactID
	}
	_, err := a.client.pool.Exec(ctx, `
		UPDATE admin.agent_runs
		SET finished_at = NOW(),
		    status      = $2,
		    artifact_id = $3
		WHERE id = $1
	`, runID, status, aid)
	if err != nil {
		return fmt.Errorf("agent run finish: %w", err)
	}
	return nil
}

func (a *AgentRunsAdapter) GetByID(ctx context.Context, runID string) (*ports.AgentRun, error) {
	row := a.client.pool.QueryRow(ctx, `
		SELECT id::text, tenant_id::text, trigger, trigger_payload, started_at, finished_at,
		       status, tools_called, tokens_in, tokens_out, cost_usd, artifact_id::text
		FROM admin.agent_runs
		WHERE id = $1
	`, runID)
	return scanAgentRun(row)
}

func (a *AgentRunsAdapter) ListForTenant(ctx context.Context, tenantID string, limit, offset int) ([]*ports.AgentRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := a.client.pool.Query(ctx, `
		SELECT id::text, tenant_id::text, trigger, trigger_payload, started_at, finished_at,
		       status, tools_called, tokens_in, tokens_out, cost_usd, artifact_id::text
		FROM admin.agent_runs
		WHERE tenant_id = $1
		ORDER BY started_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("agent runs list: %w", err)
	}
	defer rows.Close()

	var out []*ports.AgentRun
	for rows.Next() {
		r, err := scanAgentRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanAgentRun(s rowScanner) (*ports.AgentRun, error) {
	var r ports.AgentRun
	var artifactID *string
	if err := s.Scan(&r.ID, &r.TenantID, &r.Trigger, &r.TriggerPayload, &r.StartedAt, &r.FinishedAt,
		&r.Status, &r.ToolsCalled, &r.TokensIn, &r.TokensOut, &r.CostUSD, &artifactID); err != nil {
		return nil, err
	}
	r.ArtifactID = artifactID
	return &r, nil
}
