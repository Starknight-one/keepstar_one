package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"keepstar/internal/domain"
)

// TraceAdapter implements ports.TracePort with PostgreSQL storage + console output
type TraceAdapter struct {
	client *Client
}

// NewTraceAdapter creates a new TraceAdapter
func NewTraceAdapter(client *Client) *TraceAdapter {
	return &TraceAdapter{client: client}
}

// Record saves trace to DB and prints human-readable summary to console
func (a *TraceAdapter) Record(ctx context.Context, trace *domain.PipelineTrace) error {
	// Print to console first (always, even if DB write fails)
	printTrace(trace)

	traceJSON, err := json.Marshal(trace)
	if err != nil {
		return fmt.Errorf("marshal trace: %w", err)
	}

	_, err = a.client.pool.Exec(ctx, `
		INSERT INTO pipeline_traces (id, session_id, query, turn_id, timestamp, trace_data, total_ms, cost_usd, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, trace.ID, trace.SessionID, trace.Query, trace.TurnID, trace.Timestamp,
		traceJSON, trace.TotalMs, trace.CostUSD, trace.Error)
	if err != nil {
		return fmt.Errorf("insert trace: %w", err)
	}

	return nil
}

// List returns recent traces, newest first
func (a *TraceAdapter) List(ctx context.Context, limit int) ([]*domain.PipelineTrace, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := a.client.pool.Query(ctx, `
		SELECT trace_data FROM pipeline_traces
		ORDER BY timestamp DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query traces: %w", err)
	}
	defer rows.Close()

	var traces []*domain.PipelineTrace
	for rows.Next() {
		var traceJSON []byte
		if err := rows.Scan(&traceJSON); err != nil {
			return nil, fmt.Errorf("scan trace: %w", err)
		}
		var trace domain.PipelineTrace
		if err := json.Unmarshal(traceJSON, &trace); err != nil {
			continue
		}
		traces = append(traces, &trace)
	}

	return traces, nil
}

// Get returns a single trace by ID
func (a *TraceAdapter) Get(ctx context.Context, traceID string) (*domain.PipelineTrace, error) {
	var traceJSON []byte
	err := a.client.pool.QueryRow(ctx, `
		SELECT trace_data FROM pipeline_traces WHERE id = $1
	`, traceID).Scan(&traceJSON)
	if err != nil {
		return nil, fmt.Errorf("get trace: %w", err)
	}

	var trace domain.PipelineTrace
	if err := json.Unmarshal(traceJSON, &trace); err != nil {
		return nil, fmt.Errorf("unmarshal trace: %w", err)
	}

	return &trace, nil
}

// printTrace outputs a human-readable pipeline trace to stderr
func printTrace(t *domain.PipelineTrace) {
	w := os.Stderr
	line := strings.Repeat("─", 60)

	fmt.Fprintf(w, "\n%s\n", line)
	fmt.Fprintf(w, "  PIPELINE  session=%.8s  query=%q\n", t.SessionID, t.Query)
	fmt.Fprintf(w, "%s\n", line)

	// Agent1
	if t.Agent1 != nil {
		a := t.Agent1
		fmt.Fprintf(w, "  AGENT1  %dms (llm=%dms tool=%dms)\n", a.TotalMs, a.LLMMs, a.ToolMs)
		fmt.Fprintf(w, "    system: %d chars  msgs=%d  tools=%d\n", a.SystemPromptChars, a.MessageCount, a.ToolDefCount)
		if a.ToolName != "" {
			fmt.Fprintf(w, "    tool: %s\n", a.ToolName)
			if a.ToolInput != "" {
				fmt.Fprintf(w, "    input: %s\n", truncate(a.ToolInput, 120))
			}
			fmt.Fprintf(w, "    result: %s\n", truncate(a.ToolResult, 120))
			// Tool internal breakdown (normalize, fallback, etc.)
			if len(a.ToolBreakdown) > 0 {
				if v, ok := a.ToolBreakdown["normalize_path"]; ok {
					fmt.Fprintf(w, "    normalize: path=%v  %vms\n", v, a.ToolBreakdown["normalize_ms"])
				}
				if v, ok := a.ToolBreakdown["normalize_input"]; ok {
					fmt.Fprintf(w, "      in:  %s\n", v)
				}
				if v, ok := a.ToolBreakdown["normalize_output"]; ok {
					fmt.Fprintf(w, "      out: %s\n", v)
				}
				if v, ok := a.ToolBreakdown["sql_filter"]; ok {
					fmt.Fprintf(w, "    filter: %s\n", v)
				}
				if v, ok := a.ToolBreakdown["tenant"]; ok {
					fmt.Fprintf(w, "    tenant: %v\n", v)
				}
				if v, ok := a.ToolBreakdown["sql_ms"]; ok {
					fmt.Fprintf(w, "    sql: %vms", v)
					if fb, ok := a.ToolBreakdown["fallback_step"]; ok {
						step := fmt.Sprintf("%v", fb)
						switch step {
						case "0":
							fmt.Fprintf(w, "  fallback=none (direct hit)")
						case "1":
							fmt.Fprintf(w, "  fallback=brand-only")
						case "2":
							fmt.Fprintf(w, "  fallback=search-only")
						case "3":
							fmt.Fprintf(w, "  fallback=exhausted (empty)")
						}
					}
					fmt.Fprintln(w)
				}
				if v, ok := a.ToolBreakdown["price_conversion"]; ok {
					fmt.Fprintf(w, "    prices: %s\n", v)
				}
			}
		} else {
			fmt.Fprintf(w, "    NO TOOL CALLED  stop=%s\n", a.StopReason)
		}
		fmt.Fprintf(w, "    tokens: %d in + %d out  $%.6f\n", a.InputTokens, a.OutputTokens, a.CostUSD)
		if a.CacheRead > 0 {
			fmt.Fprintf(w, "    cache: read=%d write=%d\n", a.CacheRead, a.CacheWrite)
		}
	}

	// State after Agent1
	if t.StateAfterAgent1 != nil {
		s := t.StateAfterAgent1
		fmt.Fprintf(w, "  STATE  products=%d services=%d fields=%v aliases=%v deltas=%d\n",
			s.ProductCount, s.ServiceCount, s.Fields, s.Aliases, s.DeltaCount)
		for _, d := range s.Deltas {
			fmt.Fprintf(w, "    delta[%d] %s %s actor=%s", d.Step, d.DeltaType, d.Path, d.ActorID)
			if d.Tool != "" {
				fmt.Fprintf(w, " tool=%s", d.Tool)
			}
			if d.Count > 0 {
				fmt.Fprintf(w, " count=%d", d.Count)
			}
			if len(d.Fields) > 0 {
				fmt.Fprintf(w, " fields=%v", d.Fields)
			}
			fmt.Fprintln(w)
		}
	}

	// Agent2
	if t.Agent2 != nil {
		a := t.Agent2
		fmt.Fprintf(w, "  AGENT2  %dms (llm=%dms tool=%dms)\n", a.TotalMs, a.LLMMs, a.ToolMs)
		if a.PromptSent != "" {
			fmt.Fprintf(w, "    prompt: %s\n", truncate(a.PromptSent, 150))
		}
		if a.ToolName != "" {
			fmt.Fprintf(w, "    tool: %s\n", a.ToolName)
			fmt.Fprintf(w, "    result: %s\n", truncate(a.ToolResult, 120))
		} else {
			fmt.Fprintf(w, "    NO TOOL CALLED\n")
		}
		fmt.Fprintf(w, "    tokens: %d in + %d out  $%.6f\n", a.InputTokens, a.OutputTokens, a.CostUSD)
	}

	// Formation
	if t.FormationResult != nil {
		f := t.FormationResult
		fmt.Fprintf(w, "  FORMATION  mode=%s widgets=%d", f.Mode, f.WidgetCount)
		if f.FirstWidget != "" {
			fmt.Fprintf(w, "  first=%q", f.FirstWidget)
		}
		fmt.Fprintln(w)
	} else {
		fmt.Fprintf(w, "  FORMATION  nil (no widgets returned)\n")
	}

	// Error
	if t.Error != "" {
		fmt.Fprintf(w, "  ERROR  %s\n", t.Error)
	}

	// Summary
	fmt.Fprintf(w, "  DONE  %dms  $%.6f\n", t.TotalMs, t.CostUSD)
	fmt.Fprintf(w, "%s\n\n", line)
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
