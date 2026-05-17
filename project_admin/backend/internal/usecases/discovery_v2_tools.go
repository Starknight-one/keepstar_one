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
			Description: "Finalize discovery and exit. Submit the artifact as branches by vertical class plus " +
				"optional classify_rules. One branch per vertical the tenant carries (cosmetics, electronics, " +
				"furniture, haircare, apparel, footwear, food, ski, unknown). Each branch has its own field_map: " +
				"target prefixes are master.<col> for Tier 1 columns, cosmetics.<col> ONLY when the branch is " +
				"'cosmetics' (typed table exists), and tier3.<key> for every other vertical's attributes. " +
				"classify_rules are evaluated row-by-row when Shopify product_type doesn't alias to a known " +
				"vertical — supported DSL: \"product_type contains 'X'\", \"brand = 'X'\", \"name contains 'X'\", " +
				"\"tag = 'X'\".",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"branches":{
						"type":"array",
						"minItems":1,
						"items":{
							"type":"object",
							"properties":{
								"vertical":{"type":"string","enum":["cosmetics","electronics","furniture","haircare","apparel","footwear","food","ski","unknown"]},
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
								}
							},
							"required":["vertical","field_map"],
							"additionalProperties":false
						}
					},
					"classify_rules":{
						"type":"array",
						"items":{
							"type":"object",
							"properties":{
								"when":{"type":"string"},
								"then_vertical":{"type":"string"}
							},
							"required":["when","then_vertical"],
							"additionalProperties":false
						}
					},
					"notes":{"type":"string"}
				},
				"required":["branches"],
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
	Output         string                    // JSON-encoded result for LLM
	Preview        json.RawMessage           // compact preview for agent_runs log
	IsError        bool                      // surfaced to LLM as tool_result is_error
	CommitArtifact *domain.MappingArtifactV3 // non-nil iff commit_artifact was called
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
			Branches      []domain.VerticalBranch `json:"branches"`
			ClassifyRules []domain.ClassifyRule   `json:"classify_rules"`
			Notes         string                  `json:"notes"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return errResult(fmt.Errorf("commit_artifact: invalid args: %w", err))
		}
		if len(a.Branches) == 0 {
			return errResult(fmt.Errorf("commit_artifact: at least one branch required"))
		}
		ruleCount := 0
		for _, b := range a.Branches {
			if b.Vertical == "" {
				return errResult(fmt.Errorf("commit_artifact: every branch needs a vertical"))
			}
			if len(b.FieldMap) == 0 {
				return errResult(fmt.Errorf("commit_artifact: branch %q has empty field_map", b.Vertical))
			}
			ruleCount += len(b.FieldMap)
		}
		art := &domain.MappingArtifactV3{
			Version:       3,
			Branches:      a.Branches,
			ClassifyRules: a.ClassifyRules,
			Notes:         a.Notes,
		}
		body, _ := json.Marshal(map[string]any{
			"committed":      true,
			"branches":       art.VerticalNames(),
			"total_rules":    ruleCount,
			"classify_rules": len(a.ClassifyRules),
		})
		preview, _ := json.Marshal(map[string]any{
			"branches":       art.VerticalNames(),
			"rules":          ruleCount,
			"classify_rules": len(a.ClassifyRules),
		})
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
