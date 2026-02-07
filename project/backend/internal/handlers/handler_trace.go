package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// TraceHandler serves the pipeline trace debug pages
type TraceHandler struct {
	tracePort ports.TracePort
	cachePort ports.CachePort
}

// NewTraceHandler creates a trace handler
func NewTraceHandler(tracePort ports.TracePort, cachePort ports.CachePort) *TraceHandler {
	return &TraceHandler{tracePort: tracePort, cachePort: cachePort}
}

// HandleKillSession handles POST /debug/kill-session
func (h *TraceHandler) HandleKillSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	sessionID := r.FormValue("session_id")
	if sessionID == "" {
		http.Error(w, "session_id required", http.StatusBadRequest)
		return
	}
	if h.cachePort == nil {
		http.Error(w, "no cache port", http.StatusInternalServerError)
		return
	}
	if err := h.cachePort.DeleteSession(r.Context(), sessionID); err != nil {
		http.Error(w, "delete failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Redirect back to traces list
	http.Redirect(w, r, "/debug/traces/", http.StatusSeeOther)
}

// HandleTraces serves GET /debug/traces/ and /debug/traces/{id}
func (h *TraceHandler) HandleTraces(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/debug/traces")
	path = strings.TrimPrefix(path, "/")
	traceID := strings.TrimSuffix(path, "/")

	if traceID == "" {
		h.handleList(w, r)
	} else {
		h.handleDetail(w, r, traceID)
	}
}

func (h *TraceHandler) handleList(w http.ResponseWriter, r *http.Request) {
	traces, err := h.tracePort.List(r.Context(), 100)
	if err != nil {
		http.Error(w, "Failed to load traces: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if r.URL.Query().Get("format") == "json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(traces)
		return
	}

	// Check which sessions are alive
	aliveSessions := make(map[string]bool)
	if h.cachePort != nil {
		seen := make(map[string]bool)
		for _, t := range traces {
			if seen[t.SessionID] {
				continue
			}
			seen[t.SessionID] = true
			sess, err := h.cachePort.GetSession(r.Context(), t.SessionID)
			if err == nil && sess.Status == "active" {
				aliveSessions[t.SessionID] = true
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	traceListTpl.Execute(w, map[string]interface{}{
		"Traces":        traces,
		"Count":         len(traces),
		"AliveSessions": aliveSessions,
	})
}

func (h *TraceHandler) handleDetail(w http.ResponseWriter, r *http.Request, traceID string) {
	trace, err := h.tracePort.Get(r.Context(), traceID)
	if err != nil {
		http.Error(w, "Trace not found: "+err.Error(), http.StatusNotFound)
		return
	}

	if r.URL.Query().Get("format") == "json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(trace)
		return
	}

	// Pretty-print full JSON for the raw view
	rawJSON, _ := json.MarshalIndent(trace, "", "  ")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	traceDetailTpl.Execute(w, map[string]interface{}{
		"Trace":   trace,
		"RawJSON": string(rawJSON),
	})
}

var traceFuncs = template.FuncMap{
	"shortID": func(s string) string {
		if len(s) > 8 {
			return s[:8]
		}
		return s
	},
	"statusColor": func(errStr string) string {
		if errStr != "" {
			return "#ff6b6b"
		}
		return "#00ff88"
	},
	"statusText": func(errStr string) string {
		if errStr != "" {
			return "ERROR"
		}
		return "OK"
	},
	"spanDepth": func(name string) int {
		return strings.Count(name, ".")
	},
	"spanLabel": func(name string) string {
		// "agent1.llm.ttfb" → "LLM thinking", "agent1.tool" → "tool"
		labels := map[string]string{
			"pipeline": "pipeline",
		}
		if l, ok := labels[name]; ok {
			return l
		}
		// Extract suffix for nice labels
		parts := strings.Split(name, ".")
		last := parts[len(parts)-1]
		switch last {
		case "llm":
			return parts[0] + " → LLM call"
		case "ttfb":
			return "LLM thinking"
		case "body":
			return "reading response"
		case "tool":
			return parts[0] + " → tool"
		case "embed":
			return "embedding"
		case "sql":
			return "SQL keyword"
		case "vector":
			return "pgvector"
		case "state":
			return "state update"
		}
		// agent-level: "agent1", "agent2"
		if len(parts) == 1 {
			return name
		}
		return last
	},
	"spanColor": func(name string) string {
		switch {
		case strings.HasSuffix(name, ".ttfb"):
			return "#7dcfff" // cyan
		case strings.HasSuffix(name, ".body"):
			return "#5a7abf" // dark blue
		case strings.Contains(name, ".llm"):
			return "#7aa2f7" // blue
		case strings.HasSuffix(name, ".embed"):
			return "#73c991" // bright green
		case strings.HasSuffix(name, ".sql"):
			return "#e0af68" // yellow
		case strings.HasSuffix(name, ".vector"):
			return "#b877db" // magenta
		case strings.Contains(name, ".tool"):
			return "#9ece6a" // green
		case strings.Contains(name, ".state"):
			return "#565680" // gray
		case name == "pipeline":
			return "#bb9af7" // purple
		default:
			return "#e0af68" // orange (agent-level)
		}
	},
	"spanPercent": func(ms int64, totalMs int) float64 {
		if totalMs <= 0 {
			return 0
		}
		return float64(ms) / float64(totalMs) * 100
	},
	"maxTTFB": func(spans []domain.Span) string {
		var max int64
		for _, s := range spans {
			if strings.HasSuffix(s.Name, ".ttfb") && s.DurationMs > max {
				max = s.DurationMs
			}
		}
		if max == 0 {
			return "-"
		}
		return fmt.Sprintf("%dms", max)
	},
	"mult": func(a, b int) int {
		return a * b
	},
	"divInt": func(a, b int) int {
		if b == 0 {
			return 0
		}
		return a / b
	},
}

var traceListTpl = template.Must(template.New("traceList").Funcs(traceFuncs).Parse(`<!DOCTYPE html>
<html>
<head>
<title>Pipeline Traces ({{.Count}})</title>
<style>
	* { box-sizing: border-box; margin: 0; padding: 0; }
	body { font-family: 'SF Mono', 'Fira Code', monospace; background: #0a0a1a; color: #ccc; padding: 24px; }
	h1 { color: #fff; margin-bottom: 8px; font-size: 20px; }
	.subtitle { color: #666; margin-bottom: 24px; font-size: 13px; }
	table { width: 100%; border-collapse: collapse; }
	th { text-align: left; padding: 8px 12px; color: #666; font-size: 11px; text-transform: uppercase; border-bottom: 1px solid #222; }
	td { padding: 10px 12px; border-bottom: 1px solid #151525; font-size: 13px; }
	tr:hover { background: #111128; }
	a { color: #7aa2f7; text-decoration: none; }
	a:hover { text-decoration: underline; }
	.ok { color: #00ff88; }
	.err { color: #ff6b6b; }
	.ms { color: #bb9af7; }
	.cost { color: #e0af68; }
	.tool { color: #9ece6a; }
	.query { color: #fff; max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.time { color: #565680; }
	.empty { color: #444; padding: 40px; text-align: center; }
	.refresh { color: #444; margin-top: 16px; display: inline-block; }
	.kill-btn { background: #2a1020; color: #ff6b6b; border: 1px solid #ff6b6b33; padding: 2px 8px; border-radius: 4px; cursor: pointer; font-size: 11px; font-family: inherit; }
	.kill-btn:hover { background: #ff6b6b; color: #0a0a1a; }
	.session { color: #565680; font-size: 12px; }
</style>
</head>
<body>
<h1>Pipeline Traces</h1>
<p class="subtitle">{{.Count}} traces recorded. <a href="?format=json">JSON</a> &middot; <a id="refreshBtn" href="javascript:refresh()">Refresh</a></p>

{{if .Traces}}
<table>
<tr>
	<th>Time</th>
	<th>Status</th>
	<th>Query</th>
	<th>Agent1</th>
	<th>Normalize</th>
	<th>Agent2</th>
	<th>Formation</th>
	<th>TTFB</th>
	<th>Total</th>
	<th>Cost</th>
	<th>Session</th>
</tr>
{{range .Traces}}
<tr>
	<td class="time">{{.Timestamp.Format "15:04:05"}}</td>
	<td><span style="color: {{statusColor .Error}}">{{statusText .Error}}</span></td>
	<td class="query"><a href="/debug/traces/{{.ID}}">{{.Query}}</a></td>
	<td>
		{{if .Agent1}}
			<span class="tool">{{.Agent1.ToolName}}</span>
			<span class="ms">{{.Agent1.TotalMs}}ms</span>
		{{else}}-{{end}}
	</td>
	<td>
		{{if .Agent1}}{{if .Agent1.ToolBreakdown}}{{if index .Agent1.ToolBreakdown "normalize_path"}}
			<span style="color:{{if eq (index .Agent1.ToolBreakdown "normalize_path") "fast"}}#00ff88{{else}}#e0af68{{end}}">{{index .Agent1.ToolBreakdown "normalize_path"}}</span>
			<span class="ms">{{index .Agent1.ToolBreakdown "normalize_ms"}}ms</span>
		{{else}}-{{end}}{{else}}-{{end}}{{else}}-{{end}}
	</td>
	<td>
		{{if .Agent2}}
			<span class="tool">{{.Agent2.ToolName}}</span>
			<span class="ms">{{.Agent2.TotalMs}}ms</span>
		{{else}}-{{end}}
	</td>
	<td>
		{{if .FormationResult}}
			{{.FormationResult.Mode}} / {{.FormationResult.WidgetCount}}w
		{{else}}<span class="err">nil</span>{{end}}
	</td>
	<td class="ms">{{maxTTFB .Spans}}</td>
	<td class="ms">{{.TotalMs}}ms</td>
	<td class="cost">${{printf "%.4f" .CostUSD}}</td>
	<td>
		<span class="session">{{shortID .SessionID}}</span>
		{{if index $.AliveSessions .SessionID}}
		<form method="POST" action="/debug/kill-session" style="display:inline" onsubmit="return confirm('Kill session {{shortID .SessionID}}?')">
			<input type="hidden" name="session_id" value="{{.SessionID}}">
			<button class="kill-btn" type="submit">Kill</button>
		</form>
		{{else}}
		<button class="kill-btn" disabled style="opacity:0.3;cursor:default">Dead</button>
		{{end}}
	</td>
</tr>
{{end}}
</table>
{{else}}
<p class="empty" id="empty">No traces yet. Send a request to /api/v1/pipeline.</p>
{{end}}

<script>
async function refresh() {
	const btn = document.getElementById('refreshBtn');
	btn.textContent = '...';
	try {
		const resp = await fetch('/debug/traces/?format=json');
		const traces = await resp.json();
		if (traces && traces.length !== {{.Count}}) {
			location.reload();
		} else {
			btn.textContent = 'Refresh (no new)';
			setTimeout(() => { btn.textContent = 'Refresh'; }, 1000);
		}
	} catch(e) { btn.textContent = 'Refresh'; }
}
</script>
</body>
</html>`))

var traceDetailTpl = template.Must(template.New("traceDetail").Funcs(traceFuncs).Parse(`<!DOCTYPE html>
<html>
<head>
<title>Trace {{shortID .Trace.ID}}</title>
<style>
	* { box-sizing: border-box; margin: 0; padding: 0; }
	body { font-family: 'SF Mono', 'Fira Code', monospace; background: #0a0a1a; color: #ccc; padding: 24px; max-width: 900px; }
	h1 { color: #fff; font-size: 18px; margin-bottom: 4px; }
	h2 { color: #7aa2f7; font-size: 14px; margin: 24px 0 12px 0; text-transform: uppercase; }
	a { color: #7aa2f7; text-decoration: none; }
	a:hover { text-decoration: underline; }
	.back { margin-bottom: 16px; display: inline-block; font-size: 13px; }
	.meta { color: #666; font-size: 12px; margin-bottom: 20px; }
	.section { background: #111128; border-radius: 8px; padding: 16px; margin-bottom: 16px; }
	.row { display: flex; gap: 24px; margin-bottom: 8px; flex-wrap: wrap; }
	.cell { min-width: 100px; }
	.label { color: #565680; font-size: 11px; text-transform: uppercase; }
	.value { font-size: 15px; margin-top: 2px; }
	.ok { color: #00ff88; }
	.err { color: #ff6b6b; }
	.ms { color: #bb9af7; }
	.cost { color: #e0af68; }
	.tool { color: #9ece6a; }
	.tokens { color: #7dcfff; }
	pre { background: #06060f; padding: 12px; border-radius: 6px; overflow-x: auto; font-size: 12px; line-height: 1.5; max-height: 400px; overflow-y: auto; margin-top: 8px; color: #888; }
	.expandable { cursor: pointer; user-select: none; color: #7aa2f7; font-size: 12px; }
	.expandable:hover { color: #fff; }
	.hidden { display: none; }
	.error-box { background: #2a1020; border: 1px solid #ff6b6b; border-radius: 8px; padding: 16px; margin-bottom: 16px; }
</style>
<script>
function toggle(id) {
	var el = document.getElementById(id);
	el.classList.toggle('hidden');
}
</script>
</head>
<body>
<a class="back" href="/debug/traces/">&larr; All Traces</a>
<h1>{{.Trace.Query}}</h1>
<p class="meta">{{.Trace.Timestamp.Format "2006-01-02 15:04:05"}} &middot; session={{shortID .Trace.SessionID}} &middot; turn={{shortID .Trace.TurnID}} &middot; <a href="?format=json">JSON</a></p>

{{if .Trace.Error}}
<div class="error-box">
	<span class="err">ERROR: {{.Trace.Error}}</span>
</div>
{{end}}

<!-- Summary -->
<div class="section">
	<div class="row">
		<div class="cell"><div class="label">Status</div><div class="value" style="color: {{statusColor .Trace.Error}}">{{statusText .Trace.Error}}</div></div>
		<div class="cell"><div class="label">Total</div><div class="value ms">{{.Trace.TotalMs}}ms</div></div>
		<div class="cell"><div class="label">Cost</div><div class="value cost">${{printf "%.6f" .Trace.CostUSD}}</div></div>
		{{if .Trace.FormationResult}}
		<div class="cell"><div class="label">Formation</div><div class="value">{{.Trace.FormationResult.Mode}} / {{.Trace.FormationResult.WidgetCount}} widgets</div></div>
		{{end}}
	</div>
</div>

<!-- Agent 1 -->
{{if .Trace.Agent1}}
<h2>Agent 1 — Tool Caller</h2>
<div class="section">
	<div class="row">
		<div class="cell"><div class="label">Total</div><div class="value ms">{{.Trace.Agent1.TotalMs}}ms</div></div>
		<div class="cell"><div class="label">LLM</div><div class="value ms">{{.Trace.Agent1.LLMMs}}ms</div></div>
		<div class="cell"><div class="label">Tool</div><div class="value ms">{{.Trace.Agent1.ToolMs}}ms</div></div>
		<div class="cell"><div class="label">Model</div><div class="value">{{.Trace.Agent1.Model}}</div></div>
		<div class="cell"><div class="label">Cost</div><div class="value cost">${{printf "%.6f" .Trace.Agent1.CostUSD}}</div></div>
	</div>
	<div class="row">
		<div class="cell"><div class="label">Input Tokens</div><div class="value tokens">{{.Trace.Agent1.InputTokens}}</div></div>
		<div class="cell"><div class="label">Output Tokens</div><div class="value tokens">{{.Trace.Agent1.OutputTokens}}</div></div>
		<div class="cell"><div class="label">Cache Read</div><div class="value ok">{{.Trace.Agent1.CacheRead}}</div></div>
		<div class="cell"><div class="label">Cache Write</div><div class="value">{{.Trace.Agent1.CacheWrite}}</div></div>
	</div>
	<div class="row" style="margin-top: 8px;">
		<div class="cell"><div class="label">System Prompt</div><div class="value">{{.Trace.Agent1.SystemPromptChars}} chars</div></div>
		<div class="cell"><div class="label">Messages</div><div class="value">{{.Trace.Agent1.MessageCount}}</div></div>
		<div class="cell"><div class="label">Tool Defs</div><div class="value">{{.Trace.Agent1.ToolDefCount}}</div></div>
	</div>
	{{if .Trace.Agent1.SystemPrompt}}
	<span class="expandable" onclick="toggle('a1system')">&#9654; System Prompt</span>
	<pre id="a1system" class="hidden">{{.Trace.Agent1.SystemPrompt}}</pre>
	{{end}}
	{{if .Trace.Agent1.ToolName}}
	<div class="row" style="margin-top: 12px;">
		<div class="cell"><div class="label">Tool Called</div><div class="value tool">{{.Trace.Agent1.ToolName}}</div></div>
	</div>
	{{if .Trace.Agent1.ToolInput}}
	<div class="label" style="margin-top: 8px;">Tool Input</div>
	<pre>{{.Trace.Agent1.ToolInput}}</pre>
	{{end}}
	<div class="label" style="margin-top: 8px;">Tool Result</div>
	<pre>{{.Trace.Agent1.ToolResult}}</pre>
	{{if .Trace.Agent1.ToolBreakdown}}
	<div style="margin-top: 16px; border-top: 1px solid #222; padding-top: 12px;">
		<div class="label" style="margin-bottom: 8px; color: #bb9af7;">Tool Breakdown</div>
		<div class="row">
			{{if index .Trace.Agent1.ToolBreakdown "normalize_path"}}
			<div class="cell"><div class="label">Normalize</div><div class="value" style="color: {{if eq (index .Trace.Agent1.ToolBreakdown "normalize_path") "fast"}}#00ff88{{else}}#e0af68{{end}}">{{index .Trace.Agent1.ToolBreakdown "normalize_path"}}</div></div>
			{{end}}
			{{if index .Trace.Agent1.ToolBreakdown "normalize_ms"}}
			<div class="cell"><div class="label">Norm ms</div><div class="value ms">{{index .Trace.Agent1.ToolBreakdown "normalize_ms"}}ms</div></div>
			{{end}}
			{{if index .Trace.Agent1.ToolBreakdown "sql_ms"}}
			<div class="cell"><div class="label">SQL ms</div><div class="value ms">{{index .Trace.Agent1.ToolBreakdown "sql_ms"}}ms</div></div>
			{{end}}
			{{if index .Trace.Agent1.ToolBreakdown "tenant"}}
			<div class="cell"><div class="label">Tenant</div><div class="value">{{index .Trace.Agent1.ToolBreakdown "tenant"}}</div></div>
			{{end}}
			{{if index .Trace.Agent1.ToolBreakdown "fallback_step"}}
			<div class="cell"><div class="label">Fallback</div><div class="value" style="color: {{if eq (printf "%v" (index .Trace.Agent1.ToolBreakdown "fallback_step")) "0"}}#00ff88{{else}}#e0af68{{end}}">step {{index .Trace.Agent1.ToolBreakdown "fallback_step"}}</div></div>
			{{end}}
		</div>
		{{if index .Trace.Agent1.ToolBreakdown "normalize_input"}}
		<div style="margin-top: 8px;">
			<div class="label">Normalize Input</div>
			<pre style="max-height: 60px;">{{index .Trace.Agent1.ToolBreakdown "normalize_input"}}</pre>
		</div>
		{{end}}
		{{if index .Trace.Agent1.ToolBreakdown "normalize_output"}}
		<div style="margin-top: 4px;">
			<div class="label">Normalize Output</div>
			<pre style="max-height: 60px;">{{index .Trace.Agent1.ToolBreakdown "normalize_output"}}</pre>
		</div>
		{{end}}
		{{if index .Trace.Agent1.ToolBreakdown "sql_filter"}}
		<div style="margin-top: 4px;">
			<div class="label">SQL Filter</div>
			<pre style="max-height: 60px;">{{index .Trace.Agent1.ToolBreakdown "sql_filter"}}</pre>
		</div>
		{{end}}
		{{if index .Trace.Agent1.ToolBreakdown "price_conversion"}}
		<div style="margin-top: 4px;">
			<div class="label">Price Conversion</div>
			<pre style="max-height: 40px;">{{index .Trace.Agent1.ToolBreakdown "price_conversion"}}</pre>
		</div>
		{{end}}
	</div>
	{{end}}
	{{else}}
	<div class="row" style="margin-top: 12px;">
		<div class="cell"><div class="label">Tool Called</div><div class="value err">NONE (stop={{.Trace.Agent1.StopReason}})</div></div>
	</div>
	{{end}}
</div>
{{end}}

<!-- State after Agent 1 -->
{{if .Trace.StateAfterAgent1}}
<h2>State after Agent 1</h2>
<div class="section">
	<div class="row">
		<div class="cell"><div class="label">Products</div><div class="value">{{.Trace.StateAfterAgent1.ProductCount}}</div></div>
		<div class="cell"><div class="label">Services</div><div class="value">{{.Trace.StateAfterAgent1.ServiceCount}}</div></div>
		<div class="cell"><div class="label">Deltas</div><div class="value">{{.Trace.StateAfterAgent1.DeltaCount}}</div></div>
		<div class="cell"><div class="label">Has Template</div><div class="value">{{.Trace.StateAfterAgent1.HasTemplate}}</div></div>
	</div>
	{{if .Trace.StateAfterAgent1.Fields}}
	<div class="row" style="margin-top: 8px;">
		<div class="cell"><div class="label">Fields</div><div class="value">{{range .Trace.StateAfterAgent1.Fields}}{{.}} {{end}}</div></div>
	</div>
	{{end}}
	{{if .Trace.StateAfterAgent1.Aliases}}
	<div class="row">
		<div class="cell"><div class="label">Aliases</div><div class="value">{{range $k, $v := .Trace.StateAfterAgent1.Aliases}}{{$k}}={{$v}} {{end}}</div></div>
	</div>
	{{end}}
	{{if .Trace.StateAfterAgent1.Deltas}}
	<div style="margin-top: 12px;">
		<div class="label">Deltas (this turn)</div>
		<table style="width: 100%; margin-top: 6px;">
		<tr>
			<th style="width:40px">Step</th>
			<th>Type</th>
			<th>Path</th>
			<th>Actor</th>
			<th>Tool</th>
			<th>Count</th>
			<th>Fields</th>
		</tr>
		{{range .Trace.StateAfterAgent1.Deltas}}
		<tr>
			<td>{{.Step}}</td>
			<td><span style="color:{{if eq .DeltaType "add"}}#9ece6a{{else if eq .DeltaType "remove"}}#ff6b6b{{else}}#e0af68{{end}}">{{.DeltaType}}</span></td>
			<td>{{.Path}}</td>
			<td>{{.ActorID}}</td>
			<td class="tool">{{.Tool}}</td>
			<td>{{if .Count}}{{.Count}}{{else}}-{{end}}</td>
			<td>{{range .Fields}}{{.}} {{end}}</td>
		</tr>
		{{end}}
		</table>
	</div>
	{{end}}
</div>
{{end}}

<!-- Agent 2 -->
{{if .Trace.Agent2}}
<h2>Agent 2 — Renderer</h2>
<div class="section">
	<div class="row">
		<div class="cell"><div class="label">Total</div><div class="value ms">{{.Trace.Agent2.TotalMs}}ms</div></div>
		<div class="cell"><div class="label">LLM</div><div class="value ms">{{.Trace.Agent2.LLMMs}}ms</div></div>
		<div class="cell"><div class="label">Model</div><div class="value">{{.Trace.Agent2.Model}}</div></div>
		<div class="cell"><div class="label">Cost</div><div class="value cost">${{printf "%.6f" .Trace.Agent2.CostUSD}}</div></div>
	</div>
	<div class="row">
		<div class="cell"><div class="label">Input Tokens</div><div class="value tokens">{{.Trace.Agent2.InputTokens}}</div></div>
		<div class="cell"><div class="label">Output Tokens</div><div class="value tokens">{{.Trace.Agent2.OutputTokens}}</div></div>
		<div class="cell"><div class="label">Cache Read</div><div class="value ok">{{.Trace.Agent2.CacheRead}}</div></div>
		<div class="cell"><div class="label">Cache Write</div><div class="value">{{.Trace.Agent2.CacheWrite}}</div></div>
	</div>
	{{if .Trace.Agent2.ToolName}}
	<div class="row" style="margin-top: 12px;">
		<div class="cell"><div class="label">Tool Called</div><div class="value tool">{{.Trace.Agent2.ToolName}}</div></div>
	</div>
	{{end}}
	{{if .Trace.Agent2.PromptSent}}
	<span class="expandable" onclick="toggle('a2prompt')">&#9654; Prompt Sent</span>
	<pre id="a2prompt" class="hidden">{{.Trace.Agent2.PromptSent}}</pre>
	{{end}}
	{{if .Trace.Agent2.RawResponse}}
	<span class="expandable" onclick="toggle('a2raw')">&#9654; Raw Response</span>
	<pre id="a2raw" class="hidden">{{.Trace.Agent2.RawResponse}}</pre>
	{{end}}
</div>
{{end}}

<!-- Formation -->
{{if .Trace.FormationResult}}
<h2>Formation Output</h2>
<div class="section">
	<div class="row">
		<div class="cell"><div class="label">Mode</div><div class="value">{{.Trace.FormationResult.Mode}}</div></div>
		<div class="cell"><div class="label">Widgets</div><div class="value">{{.Trace.FormationResult.WidgetCount}}</div></div>
		{{if .Trace.FormationResult.Cols}}<div class="cell"><div class="label">Cols</div><div class="value">{{.Trace.FormationResult.Cols}}</div></div>{{end}}
		{{if .Trace.FormationResult.FirstWidget}}<div class="cell"><div class="label">First Widget</div><div class="value">{{.Trace.FormationResult.FirstWidget}}</div></div>{{end}}
	</div>
</div>
{{end}}

<!-- Waterfall -->
{{if .Trace.Spans}}
<h2>Waterfall</h2>
<div class="section" style="padding: 16px;">
	<!-- Timeline ruler -->
	<div style="display: flex; margin-bottom: 6px;">
		<div style="width: 180px; flex-shrink: 0;"></div>
		<div style="flex: 1; display: flex; justify-content: space-between; font-size: 10px; color: #444; padding: 0 2px; border-bottom: 1px solid #1a1a2e;">
			<span>0ms</span>
			<span>{{divInt .Trace.TotalMs 2}}ms</span>
			<span>{{.Trace.TotalMs}}ms</span>
		</div>
		<div style="width: 180px; flex-shrink: 0;"></div>
	</div>
	{{range .Trace.Spans}}
	<div style="display: flex; align-items: center; margin-bottom: 2px; height: 24px;">
		<div style="width: 180px; flex-shrink: 0; font-size: 12px; text-align: right; padding-right: 12px; white-space: nowrap; padding-left: {{mult (spanDepth .Name) 12}}px;">
			<span style="color: {{spanColor .Name}}">{{spanLabel .Name}}</span>
		</div>
		<div style="flex: 1; position: relative; height: 18px; background: #06060f; border-radius: 3px; border: 1px solid #1a1a2e;">
			<div style="position: absolute; left: {{spanPercent .StartMs $.Trace.TotalMs}}%; width: {{spanPercent .DurationMs $.Trace.TotalMs}}%; min-width: 3px; height: 100%; background: {{spanColor .Name}}; border-radius: 2px; opacity: 0.8;" title="{{.Name}} {{.StartMs}}ms—{{.EndMs}}ms"></div>
		</div>
		<div style="width: 80px; flex-shrink: 0; font-size: 12px; color: #bb9af7; text-align: right; padding-left: 8px;">{{.DurationMs}}ms</div>
		<div style="width: 100px; flex-shrink: 0; font-size: 11px; color: #666; padding-left: 8px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;" title="{{.Detail}}">{{.Detail}}</div>
	</div>
	{{end}}
	<!-- Legend -->
	<div style="margin-top: 14px; padding-top: 10px; border-top: 1px solid #1a1a2e; font-size: 11px; color: #555; display: flex; gap: 18px; flex-wrap: wrap;">
		<span><span style="color: #e0af68;">&#9644;</span> agent</span>
		<span><span style="color: #7aa2f7;">&#9644;</span> LLM call</span>
		<span><span style="color: #7dcfff;">&#9644;</span> LLM thinking (TTFB)</span>
		<span><span style="color: #5a7abf;">&#9644;</span> response body</span>
		<span><span style="color: #9ece6a;">&#9644;</span> tool</span>
		<span><span style="color: #73c991;">&#9644;</span> embedding</span>
		<span><span style="color: #e0af68;">&#9644;</span> SQL</span>
		<span><span style="color: #b877db;">&#9644;</span> pgvector</span>
		<span><span style="color: #565680;">&#9644;</span> state</span>
		<span><span style="color: #bb9af7;">&#9644;</span> pipeline</span>
	</div>
</div>
{{end}}

<!-- Raw JSON -->
<h2>Full Trace JSON</h2>
<span class="expandable" onclick="toggle('rawjson')">&#9654; Show raw JSON</span>
<pre id="rawjson" class="hidden">{{.RawJSON}}</pre>

</body>
</html>`))
