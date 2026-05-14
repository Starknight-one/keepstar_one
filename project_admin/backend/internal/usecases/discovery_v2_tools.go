// Package usecases — discovery_v2_tools defines the tool surface exposed
// to the LLM agent and the dispatcher that runs them against InboxPort.
//
// Why split from discovery_v2.go: the agent loop is the orchestration; the
// tool defs + JSON schemas + arg-parsing live here. Keeps both files
// reviewable.
package usecases

import (
	"context"
	"encoding/json"
	"fmt"

	"keepstar-admin/internal/adapters/anthropic"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/ports"
)

// discoveryToolName aliases for readability.
const (
	toolCountTotal     = "count_total"
	toolListFields     = "list_fields"
	toolSampleValues   = "sample_values"
	toolCountBy        = "count_by"
	toolFieldStats     = "field_stats"
	toolPeekFullRows   = "peek_full_rows"
	toolCommitArtifact = "commit_artifact"
)

// discoveryTools returns the ToolDef list passed to Anthropic. CacheControl
// is set on the LAST tool — Anthropic caches everything up to and including
// the marked block, so the entire tool list (which is identical across
// turns) gets cached.
func discoveryTools() []anthropic.ToolDef {
	defs := []anthropic.ToolDef{
		{
			Name:        toolCountTotal,
			Description: "Return the total number of inbox rows for this tenant. No arguments.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		},
		{
			Name: toolListFields,
			Description: "Return up to `limit` distinct top-level JSONB keys observed across all inbox " +
				"rows for this tenant. Use this first to see the shape of the source data. Default limit 100.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{"limit":{"type":"integer","minimum":1,"maximum":500}},
				"additionalProperties":false
			}`),
		},
		{
			Name: toolSampleValues,
			Description: "Return up to `limit` distinct stringified values for ONE top-level field. " +
				"Cheap; safe to call for many fields. Default limit 50.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"field":{"type":"string"},
					"limit":{"type":"integer","minimum":1,"maximum":200}
				},
				"required":["field"],
				"additionalProperties":false
			}`),
		},
		{
			Name: toolCountBy,
			Description: "Return value→count distribution for ONE field, top-N buckets by frequency. " +
				"Use when you want to see how skewed a field is. Default max_buckets 50.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"field":{"type":"string"},
					"max_buckets":{"type":"integer","minimum":1,"maximum":500}
				},
				"required":["field"],
				"additionalProperties":false
			}`),
		},
		{
			Name: toolFieldStats,
			Description: "Compact stats for ONE field: non-null count, distinct count, sample values, " +
				"numeric min/max (if applicable), length min/max. One call per field; cheap.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"field":{"type":"string"},
					"sample_n":{"type":"integer","minimum":1,"maximum":100}
				},
				"required":["field"],
				"additionalProperties":false
			}`),
		},
		{
			Name: toolPeekFullRows,
			Description: "HEAVY — returns 1..5 ENTIRE raw inbox rows. Use sparingly when aggregates " +
				"aren't enough to disambiguate. Counted against a per-run cap of 10 calls; further " +
				"calls return an error. Default limit 3.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{"limit":{"type":"integer","minimum":1,"maximum":5}},
				"additionalProperties":false
			}`),
		},
		{
			Name: toolCommitArtifact,
			Description: "Finalize discovery. Submit the mapping artifact and terminate. " +
				"`vertical` must be one of: cosmetics, electronics, furniture, apparel, footwear, food, unknown. " +
				"`field_map` is an ordered list of rules — each maps an inbox field to a target (master.<col>, " +
				"<vertical>.<col>, or tier3.<key>) with optional transform. Optional `notes` for free-form summary.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"vertical":{"type":"string","enum":["cosmetics","electronics","furniture","apparel","footwear","food","unknown"]},
					"field_map":{
						"type":"array",
						"items":{
							"type":"object",
							"properties":{
								"from":{"type":"string"},
								"to":{"type":"string"},
								"transform":{"type":"string"},
								"default":{"type":"string"}
							},
							"required":["from","to"],
							"additionalProperties":false
						}
					},
					"notes":{"type":"string"}
				},
				"required":["vertical","field_map"],
				"additionalProperties":false
			}`),
		},
	}
	// Mark cache breakpoint on the last tool — Anthropic caches up to and
	// including it. Keep the tool list stable across turns to hit the cache.
	if n := len(defs); n > 0 {
		defs[n-1].CacheControl = &anthropic.CacheControl{Type: "ephemeral"}
	}
	return defs
}

// dispatchResult is what dispatchTool returns to the loop: rendered string
// for the LLM tool_result, a preview for agent_runs logging, plus the
// optional "committed artifact" signal.
type dispatchResult struct {
	Output          string                    // JSON-encoded result for LLM
	Preview         json.RawMessage           // compact preview for agent_runs log
	IsError         bool                      // surfaced to LLM as tool_result is_error
	CommitArtifact  *domain.MappingArtifactV2 // non-nil iff commit_artifact was called
}

// dispatchTool runs one tool invocation against InboxPort. Caller is
// responsible for enforcing per-call ordering and budget — this function
// only touches the heavy-tool counter.
func dispatchTool(
	ctx context.Context,
	inbox ports.InboxPort,
	tenantID string,
	name string,
	args json.RawMessage,
	heavyCallsUsed *int,
	heavyCallsCap int,
) *dispatchResult {
	switch name {
	case toolCountTotal:
		n, err := inbox.CountTotal(ctx, tenantID)
		if err != nil {
			return errResult(err)
		}
		return okJSON(map[string]int{"total": n})

	case toolListFields:
		var a struct{ Limit int `json:"limit"` }
		_ = json.Unmarshal(args, &a)
		if a.Limit == 0 {
			a.Limit = 100
		}
		fs, err := inbox.ListFields(ctx, tenantID, a.Limit)
		if err != nil {
			return errResult(err)
		}
		return okJSON(map[string]any{"fields": fs, "count": len(fs)})

	case toolSampleValues:
		var a struct {
			Field string `json:"field"`
			Limit int    `json:"limit"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.Field == "" {
			return errResult(fmt.Errorf("sample_values: field required"))
		}
		if a.Limit == 0 {
			a.Limit = 50
		}
		vs, err := inbox.SampleValues(ctx, tenantID, a.Field, a.Limit)
		if err != nil {
			return errResult(err)
		}
		return okJSON(map[string]any{"field": a.Field, "values": vs, "count": len(vs)})

	case toolCountBy:
		var a struct {
			Field      string `json:"field"`
			MaxBuckets int    `json:"max_buckets"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.Field == "" {
			return errResult(fmt.Errorf("count_by: field required"))
		}
		if a.MaxBuckets == 0 {
			a.MaxBuckets = 50
		}
		dist, err := inbox.CountBy(ctx, tenantID, a.Field, a.MaxBuckets)
		if err != nil {
			return errResult(err)
		}
		return okJSON(map[string]any{"field": a.Field, "distribution": dist})

	case toolFieldStats:
		var a struct {
			Field    string `json:"field"`
			SampleN  int    `json:"sample_n"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.Field == "" {
			return errResult(fmt.Errorf("field_stats: field required"))
		}
		if a.SampleN == 0 {
			a.SampleN = 20
		}
		st, err := inbox.FieldStats(ctx, tenantID, a.Field, a.SampleN)
		if err != nil {
			return errResult(err)
		}
		return okJSON(st)

	case toolPeekFullRows:
		if *heavyCallsUsed >= heavyCallsCap {
			return errResult(fmt.Errorf("peek_full_rows: per-run cap of %d reached; rely on aggregate tools", heavyCallsCap))
		}
		*heavyCallsUsed++
		var a struct{ Limit int `json:"limit"` }
		_ = json.Unmarshal(args, &a)
		if a.Limit == 0 {
			a.Limit = 3
		}
		rows, err := inbox.PeekFullRows(ctx, tenantID, a.Limit)
		if err != nil {
			return errResult(err)
		}
		// Build a JSON array of raw payloads + minimal envelope.
		outs := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			outs = append(outs, map[string]any{
				"external_id": r.ExternalID,
				"source_kind": r.SourceKind,
				"raw":         r.Raw,
			})
		}
		preview, _ := json.Marshal(map[string]any{"rows_returned": len(outs)})
		body, _ := json.Marshal(map[string]any{"rows": outs})
		return &dispatchResult{Output: string(body), Preview: preview}

	case toolCommitArtifact:
		var a struct {
			Vertical string                    `json:"vertical"`
			FieldMap []domain.FieldMappingRule `json:"field_map"`
			Notes    string                    `json:"notes"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errResult(fmt.Errorf("commit_artifact: invalid args: %w", err))
		}
		if a.Vertical == "" || len(a.FieldMap) == 0 {
			return errResult(fmt.Errorf("commit_artifact: vertical and non-empty field_map required"))
		}
		art := &domain.MappingArtifactV2{
			Version:  2,
			Vertical: a.Vertical,
			FieldMap: a.FieldMap,
			Notes:    a.Notes,
		}
		body, _ := json.Marshal(map[string]any{
			"committed":     true,
			"vertical":      a.Vertical,
			"field_map_len": len(a.FieldMap),
		})
		preview, _ := json.Marshal(map[string]any{"vertical": a.Vertical, "rules": len(a.FieldMap)})
		return &dispatchResult{Output: string(body), Preview: preview, CommitArtifact: art}

	default:
		return errResult(fmt.Errorf("unknown tool: %s", name))
	}
}

// okJSON marshals v to JSON and packages it as a dispatchResult.
func okJSON(v any) *dispatchResult {
	body, _ := json.Marshal(v)
	preview := body
	if len(preview) > 500 {
		preview = json.RawMessage(`{"truncated":true}`)
	}
	return &dispatchResult{Output: string(body), Preview: preview}
}

func errResult(err error) *dispatchResult {
	msg := err.Error()
	body, _ := json.Marshal(map[string]string{"error": msg})
	return &dispatchResult{Output: string(body), IsError: true, Preview: json.RawMessage(fmt.Sprintf(`{"error":%q}`, msg))}
}
