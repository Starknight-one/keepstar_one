package handlers

import (
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"keepstar/internal/domain"
	"keepstar/internal/ports"
	"keepstar/internal/presets"
	"keepstar/internal/tools"
)

// PipelineMetrics stores metrics from last pipeline execution
type PipelineMetrics struct {
	SessionID     string         `json:"sessionId"`
	Query         string         `json:"query"`
	Timestamp     time.Time      `json:"timestamp"`
	Agent1Metrics *AgentMetrics  `json:"agent1"`
	Agent2Metrics *AgentMetrics  `json:"agent2"`
	TotalMs       int            `json:"totalMs"`
	Formation     *FormationInfo `json:"formation,omitempty"`
}

// AgentMetrics stores metrics for a single agent
type AgentMetrics struct {
	DurationMs   int      `json:"durationMs"`
	LLMCallMs    int64    `json:"llmCallMs"`
	ToolMs       int64    `json:"toolMs,omitempty"`
	InputTokens  int      `json:"inputTokens"`
	OutputTokens int      `json:"outputTokens"`
	TotalTokens  int      `json:"totalTokens"`
	CostUSD      float64  `json:"costUsd"`
	Model        string   `json:"model"`
	// Cache metrics
	CacheCreationInputTokens int     `json:"cacheCreationInputTokens,omitempty"`
	CacheReadInputTokens     int     `json:"cacheReadInputTokens,omitempty"`
	CacheHitRate             float64 `json:"cacheHitRate,omitempty"` // percentage
	// Agent 1 specific
	ToolCalled    string `json:"toolCalled,omitempty"`
	ToolInput     string `json:"toolInput,omitempty"`
	ToolResult    string `json:"toolResult,omitempty"`
	ProductsFound int    `json:"productsFound,omitempty"`
	// Agent 2 specific
	PromptSent   string   `json:"promptSent,omitempty"`
	RawResponse  string   `json:"rawResponse,omitempty"`
	TemplateJSON string   `json:"templateJson,omitempty"`
	MetaCount    int      `json:"metaCount,omitempty"`
	MetaFields   []string `json:"metaFields,omitempty"`
}

// FormationInfo stores formation summary
type FormationInfo struct {
	Mode        string `json:"mode"`
	WidgetCount int    `json:"widgetCount"`
	Cols        int    `json:"cols,omitempty"`
}

// MetricsStore stores recent pipeline metrics (in-memory)
type MetricsStore struct {
	mu      sync.RWMutex
	metrics map[string]*PipelineMetrics // sessionID -> metrics
}

// NewMetricsStore creates a new metrics store
func NewMetricsStore() *MetricsStore {
	return &MetricsStore{
		metrics: make(map[string]*PipelineMetrics),
	}
}

// Store stores metrics for a session
func (s *MetricsStore) Store(m *PipelineMetrics) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics[m.SessionID] = m
}

// Get retrieves metrics for a session
func (s *MetricsStore) Get(sessionID string) *PipelineMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.metrics[sessionID]
}

// GetAll returns all stored metrics
func (s *MetricsStore) GetAll() []*PipelineMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*PipelineMetrics, 0, len(s.metrics))
	for _, m := range s.metrics {
		result = append(result, m)
	}
	return result
}

// DebugHandler handles debug/monitoring requests
type DebugHandler struct {
	statePort      ports.StatePort
	cachePort      ports.CachePort
	metricsStore   *MetricsStore
	presetRegistry *presets.PresetRegistry
}

// NewDebugHandler creates a debug handler
func NewDebugHandler(statePort ports.StatePort, cachePort ports.CachePort, metricsStore *MetricsStore) *DebugHandler {
	return &DebugHandler{
		statePort:      statePort,
		cachePort:      cachePort,
		metricsStore:   metricsStore,
		presetRegistry: presets.NewPresetRegistry(),
	}
}

// HandleDebugPage serves the debug HTML page
func (h *DebugHandler) HandleDebugPage(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from path: /debug/session/{id}
	path := strings.TrimPrefix(r.URL.Path, "/debug/session/")
	sessionID := strings.TrimSuffix(path, "/")

	if sessionID == "" {
		// Show list of all sessions
		h.handleSessionList(w)
		return
	}

	h.handleSessionDetail(w, r, sessionID)
}

func (h *DebugHandler) handleSessionList(w http.ResponseWriter) {
	metrics := h.metricsStore.GetAll()

	data := struct {
		Sessions []*PipelineMetrics
	}{
		Sessions: metrics,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	listTemplate.Execute(w, data)
}

func (h *DebugHandler) handleSessionDetail(w http.ResponseWriter, r *http.Request, sessionID string) {
	ctx := r.Context()

	// Get state
	state, stateErr := h.statePort.GetState(ctx, sessionID)

	// Get deltas
	var deltas []domain.Delta
	if stateErr == nil {
		deltas, _ = h.statePort.GetDeltas(ctx, sessionID)
	}

	// Get metrics
	metrics := h.metricsStore.Get(sessionID)

	// Check format
	if r.URL.Query().Get("format") == "json" {
		data := map[string]any{
			"sessionId": sessionID,
			"state":     state,
			"deltas":    deltas,
			"metrics":   metrics,
			"error":     nil,
		}
		if stateErr != nil {
			data["error"] = stateErr.Error()
		}
		writeJSON(w, http.StatusOK, data)
		return
	}

	// HTML response
	data := struct {
		SessionID    string
		State        *domain.SessionState
		StateError   string
		Deltas       []domain.Delta
		Metrics      *PipelineMetrics
		ProductCount int
		HasTemplate  bool
	}{
		SessionID: sessionID,
		State:     state,
		Deltas:    deltas,
		Metrics:   metrics,
	}

	if stateErr != nil {
		data.StateError = stateErr.Error()
	}
	if state != nil {
		data.ProductCount = len(state.Current.Data.Products)
		data.HasTemplate = state.Current.Template != nil
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	detailTemplate.Execute(w, data)
}

// HandleDebugAPI returns JSON debug info
func (h *DebugHandler) HandleDebugAPI(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"sessions": h.metricsStore.GetAll(),
		})
		return
	}

	ctx := r.Context()
	state, _ := h.statePort.GetState(ctx, sessionID)
	deltas, _ := h.statePort.GetDeltas(ctx, sessionID)
	metrics := h.metricsStore.Get(sessionID)

	writeJSON(w, http.StatusOK, map[string]any{
		"sessionId": sessionID,
		"state":     state,
		"deltas":    deltas,
		"metrics":   metrics,
	})
}

// SeedStateResponse is the response for seed state endpoint
type SeedStateResponse struct {
	SessionID string                      `json:"sessionId"`
	Formation *FormationResponse          `json:"formation"`
	Products  int                         `json:"products"`
	Message   string                      `json:"message"`
}

// HandleSeedState creates a session with mock products for testing without LLM
// POST /debug/seed - creates new session with products and returns formation
// Use this to test navigation (expand/back) without making LLM calls
func (h *DebugHandler) HandleSeedState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed. Use POST.", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	sessionID := uuid.New().String()

	// Create session in cache (required by FK constraint)
	session := &domain.Session{
		ID:             sessionID,
		Status:         domain.SessionStatus("active"),
		StartedAt:      time.Now(),
		LastActivityAt: time.Now(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := h.cachePort.SaveSession(ctx, session); err != nil {
		http.Error(w, "Failed to create session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create state with mock products
	state, err := h.statePort.CreateState(ctx, sessionID)
	if err != nil {
		http.Error(w, "Failed to create state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Add mock products (Nike-like data)
	state.Current.Data.Products = []domain.Product{
		{
			ID:            "prod-1",
			Name:          "Nike Air Max 90",
			Description:   "The Nike Air Max 90 stays true to its OG roots with the iconic Waffle sole.",
			Price:         12990,
			Currency:      "$",
			Images:        []string{"https://static.nike.com/a/images/t_PDP_1728_v1/f_auto,q_auto:eco/b7d9211c-26e7-431a-ac24-b0540fb3c00f/air-max-90-shoes.png"},
			Rating:        4.5,
			StockQuantity: 15,
			Brand:         "Nike",
			Category:      "Sneakers",
			Tags:          []string{"running", "classic", "air max"},
		},
		{
			ID:            "prod-2",
			Name:          "Nike Air Force 1 '07",
			Description:   "The radiance lives on in the Nike Air Force 1 '07.",
			Price:         9990,
			Currency:      "$",
			Images:        []string{"https://static.nike.com/a/images/t_PDP_1728_v1/f_auto,q_auto:eco/b7d9211c-26e7-431a-ac24-b0540fb3c00f/air-force-1-07-shoes.png"},
			Rating:        4.8,
			StockQuantity: 23,
			Brand:         "Nike",
			Category:      "Sneakers",
			Tags:          []string{"basketball", "classic", "white"},
		},
		{
			ID:            "prod-3",
			Name:          "Nike Dunk Low",
			Description:   "Created for the hardwood but taken to the streets.",
			Price:         10990,
			Currency:      "$",
			Images:        []string{"https://static.nike.com/a/images/t_PDP_1728_v1/f_auto,q_auto:eco/af407807-59eb-4a4b-b3c6-0a5f7d85f8b6/dunk-low-shoes.png"},
			Rating:        4.7,
			StockQuantity: 8,
			Brand:         "Nike",
			Category:      "Sneakers",
			Tags:          []string{"skateboard", "retro"},
		},
		{
			ID:            "prod-4",
			Name:          "Nike Air Jordan 1 Retro High OG",
			Description:   "The Air Jordan 1 Retro High remakes the classic sneaker.",
			Price:         17990,
			Currency:      "$",
			Images:        []string{"https://static.nike.com/a/images/t_PDP_1728_v1/f_auto,q_auto:eco/u_126ab356-44d8-4a06-89b4-fcdcc8df0245,c_scale,fl_relative,w_1.0,h_1.0,fl_layer_apply/air-jordan-1-retro-high-og-shoes.png"},
			Rating:        4.9,
			StockQuantity: 5,
			Brand:         "Jordan",
			Category:      "Sneakers",
			Tags:          []string{"basketball", "jordan", "retro", "high-top"},
		},
	}

	// Update meta
	state.Current.Meta = domain.StateMeta{
		Count:        len(state.Current.Data.Products),
		ProductCount: len(state.Current.Data.Products),
		Fields:       []string{"id", "name", "price", "images", "rating", "brand", "category", "description", "tags", "attributes", "stockQuantity"},
	}

	// Build formation using product_grid preset
	preset, _ := h.presetRegistry.Get(domain.PresetProductGrid)
	products := state.Current.Data.Products
	formation := tools.BuildFormation(preset, len(products), func(i int) (tools.FieldGetter, tools.CurrencyGetter, tools.IDGetter) {
		p := products[i]
		return productFieldGetterDebug(p), func() string { return p.Currency }, func() string { return p.ID }
	})

	// Save formation to state
	state.Current.Template = map[string]interface{}{
		"formation": formation,
	}
	state.View.Mode = domain.ViewModeGrid

	if err := h.statePort.UpdateState(ctx, state); err != nil {
		http.Error(w, "Failed to update state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create initial delta
	delta := &domain.Delta{
		Step:      1,
		Trigger:   domain.TriggerSystem,
		Source:    domain.SourceSystem,
		ActorID:   "debug_seed",
		DeltaType: domain.DeltaTypeAdd,
		Path:      "data.products",
		Action:    domain.Action{Type: domain.ActionSearch, Tool: "debug_seed"},
		Result:    domain.ResultMeta{Count: len(products), Fields: state.Current.Meta.Fields},
		CreatedAt: time.Now(),
	}
	h.statePort.AddDelta(ctx, sessionID, delta)
	// step is now auto-assigned by AddDelta and synced to state


	// Return response
	resp := SeedStateResponse{
		SessionID: sessionID,
		Formation: &FormationResponse{
			Mode:    string(formation.Mode),
			Grid:    formation.Grid,
			Widgets: formation.Widgets,
		},
		Products: len(products),
		Message:  "Session created with mock products. Use this sessionId for navigation testing.",
	}

	writeJSON(w, http.StatusOK, resp)
}

// productFieldGetterDebug is a simple field getter for debug seeding
func productFieldGetterDebug(p domain.Product) tools.FieldGetter {
	return func(fieldName string) interface{} {
		switch fieldName {
		case "id":
			return p.ID
		case "name":
			if p.Name == "" {
				return nil
			}
			return p.Name
		case "description":
			if p.Description == "" {
				return nil
			}
			return p.Description
		case "price":
			return p.Price
		case "images":
			if len(p.Images) == 0 {
				return nil
			}
			return p.Images
		case "rating":
			if p.Rating == 0 {
				return nil
			}
			return p.Rating
		case "brand":
			if p.Brand == "" {
				return nil
			}
			return p.Brand
		case "category":
			if p.Category == "" {
				return nil
			}
			return p.Category
		case "stockQuantity":
			if p.StockQuantity == 0 {
				return nil
			}
			return p.StockQuantity
		case "tags":
			if len(p.Tags) == 0 {
				return nil
			}
			return p.Tags
		default:
			return nil
		}
	}
}

var templateFuncs = template.FuncMap{
	"addFloat": func(a, b float64) float64 { return a + b },
	"addInt":   func(a, b int) int { return a + b },
}

var listTemplate = template.Must(template.New("list").Funcs(templateFuncs).Parse(`
<!DOCTYPE html>
<html>
<head>
    <title>Pipeline Debug</title>
    <style>
        body { font-family: system-ui, sans-serif; margin: 40px; background: #1a1a2e; color: #eee; }
        h1 { color: #00d4ff; }
        table { border-collapse: collapse; width: 100%; margin-top: 20px; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #333; }
        th { background: #16213e; color: #00d4ff; }
        tr:hover { background: #16213e; }
        a { color: #00d4ff; text-decoration: none; }
        a:hover { text-decoration: underline; }
        .cost { color: #ffd700; }
        .time { color: #90EE90; }
        .tokens { color: #DDA0DD; }
        .cache-hit { color: #00ff88; font-weight: bold; }
        .cache-write { color: #ff9800; }
        .empty { color: #666; font-style: italic; }
    </style>
</head>
<body>
    <h1>🔍 Pipeline Debug Console</h1>
    {{if .Sessions}}
    <table>
        <tr>
            <th>Session ID</th>
            <th>Query</th>
            <th>Agent 1</th>
            <th>Agent 2</th>
            <th>Cache</th>
            <th>Total</th>
            <th>Cost</th>
            <th>Time</th>
        </tr>
        {{range .Sessions}}
        <tr>
            <td><a href="/debug/session/{{.SessionID}}">{{printf "%.8s" .SessionID}}...</a></td>
            <td>{{.Query}}</td>
            <td>
                {{if .Agent1Metrics}}
                <span class="time">{{.Agent1Metrics.DurationMs}}ms</span> /
                <span class="tokens">{{.Agent1Metrics.TotalTokens}}tok</span>
                {{end}}
            </td>
            <td>
                {{if .Agent2Metrics}}
                <span class="time">{{.Agent2Metrics.DurationMs}}ms</span> /
                <span class="tokens">{{.Agent2Metrics.TotalTokens}}tok</span>
                {{end}}
            </td>
            <td>
                {{if .Agent1Metrics}}
                {{if gt .Agent1Metrics.CacheReadInputTokens 0}}<span class="cache-hit">HIT {{printf "%.0f" .Agent1Metrics.CacheHitRate}}%</span>
                {{else if gt .Agent1Metrics.CacheCreationInputTokens 0}}<span class="cache-write">WRITE</span>
                {{else}}<span style="color:#666">-</span>{{end}}
                {{end}}
            </td>
            <td class="time">{{.TotalMs}}ms</td>
            <td class="cost">
                {{if .Agent1Metrics}}${{printf "%.4f" .Agent1Metrics.CostUSD}}{{end}}
                {{if .Agent2Metrics}}+ ${{printf "%.4f" .Agent2Metrics.CostUSD}}{{end}}
            </td>
            <td>{{.Timestamp.Format "15:04:05"}}</td>
        </tr>
        {{end}}
    </table>
    {{else}}
    <p class="empty">No sessions yet. Send a query to /api/v1/pipeline first.</p>
    {{end}}
    <p style="margin-top: 30px; color: #666;">Auto-refresh: <a href="javascript:location.reload()">Refresh</a></p>
</body>
</html>
`))

var detailTemplate = template.Must(template.New("detail").Funcs(templateFuncs).Parse(`
<!DOCTYPE html>
<html>
<head>
    <title>Session {{.SessionID}}</title>
    <style>
        body { font-family: system-ui, sans-serif; margin: 40px; background: #1a1a2e; color: #eee; }
        h1, h2, h3 { color: #00d4ff; margin-top: 30px; }
        .back { color: #00d4ff; text-decoration: none; }
        .section { background: #16213e; padding: 20px; border-radius: 8px; margin: 20px 0; }
        .metric { display: inline-block; margin-right: 30px; margin-bottom: 15px; }
        .metric-value { font-size: 24px; font-weight: bold; }
        .metric-label { color: #888; font-size: 12px; }
        .cost { color: #ffd700; }
        .time { color: #90EE90; }
        .tokens { color: #DDA0DD; }
        .cache-hit { color: #00ff88; font-weight: bold; }
        .cache-write { color: #ff9800; }
        .error { color: #ff6b6b; }
        pre { background: #0f0f23; padding: 15px; border-radius: 4px; overflow-x: auto; font-size: 12px; max-height: 300px; overflow-y: auto; }
        table { border-collapse: collapse; width: 100%; }
        th, td { padding: 8px 12px; text-align: left; border-bottom: 1px solid #333; }
        th { color: #00d4ff; }
        .breakdown { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; margin: 20px 0; }
        .breakdown-item { background: #0f0f23; padding: 15px; border-radius: 8px; }
        .breakdown-title { color: #888; font-size: 11px; text-transform: uppercase; margin-bottom: 5px; }
        .breakdown-value { font-size: 18px; font-weight: bold; }
        .label { color: #888; font-size: 12px; margin-bottom: 5px; }
        .collapsible { cursor: pointer; user-select: none; }
        .collapsible:hover { color: #00d4ff; }
        .content { display: none; }
        .content.show { display: block; }
    </style>
</head>
<body>
    <a class="back" href="/debug/session/">← All Sessions</a>
    <h1>Session: {{printf "%.8s" .SessionID}}...</h1>
    <p style="color: #666; font-size: 12px;">{{.SessionID}}</p>

    {{if .StateError}}
    <div class="section error">
        <h2>⚠️ Error</h2>
        <p>{{.StateError}}</p>
    </div>
    {{end}}

    {{if .Metrics}}
    <div class="section">
        <h2>📊 Overview</h2>
        <p><strong>Query:</strong> {{.Metrics.Query}}</p>
        <p><strong>Time:</strong> {{.Metrics.Timestamp.Format "2006-01-02 15:04:05"}}</p>

        <div class="breakdown">
            <div class="breakdown-item">
                <div class="breakdown-title">Total Time</div>
                <div class="breakdown-value time">{{.Metrics.TotalMs}}ms</div>
            </div>
            {{if .Metrics.Agent1Metrics}}
            <div class="breakdown-item">
                <div class="breakdown-title">Total Cost</div>
                <div class="breakdown-value cost">${{printf "%.6f" (addFloat .Metrics.Agent1Metrics.CostUSD .Metrics.Agent2Metrics.CostUSD)}}</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Total Tokens</div>
                <div class="breakdown-value tokens">{{addInt .Metrics.Agent1Metrics.TotalTokens .Metrics.Agent2Metrics.TotalTokens}}</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Products Found</div>
                <div class="breakdown-value">{{.Metrics.Agent1Metrics.ProductsFound}}</div>
            </div>
            {{end}}
        </div>
    </div>

    {{if .Metrics.Agent1Metrics}}
    <div class="section">
        <h2>🤖 Agent 1 (Tool Caller)</h2>

        <div class="breakdown">
            <div class="breakdown-item">
                <div class="breakdown-title">Total Time</div>
                <div class="breakdown-value time">{{.Metrics.Agent1Metrics.DurationMs}}ms</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">LLM Call</div>
                <div class="breakdown-value time">{{.Metrics.Agent1Metrics.LLMCallMs}}ms</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Tool Execution</div>
                <div class="breakdown-value time">{{.Metrics.Agent1Metrics.ToolMs}}ms</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Model</div>
                <div class="breakdown-value" style="font-size: 14px;">{{.Metrics.Agent1Metrics.Model}}</div>
            </div>
        </div>

        <h3>Tokens & Cache</h3>
        <div class="breakdown">
            <div class="breakdown-item">
                <div class="breakdown-title">Input</div>
                <div class="breakdown-value tokens">{{.Metrics.Agent1Metrics.InputTokens}}</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Output</div>
                <div class="breakdown-value tokens">{{.Metrics.Agent1Metrics.OutputTokens}}</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Cache Write</div>
                <div class="breakdown-value" style="color: #ff9800;">{{.Metrics.Agent1Metrics.CacheCreationInputTokens}}</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Cache Read</div>
                <div class="breakdown-value" style="color: #00ff88;">{{.Metrics.Agent1Metrics.CacheReadInputTokens}}</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Cache Hit Rate</div>
                <div class="breakdown-value {{if gt .Metrics.Agent1Metrics.CacheHitRate 0.0}}cache-hit{{end}}">{{printf "%.1f" .Metrics.Agent1Metrics.CacheHitRate}}%</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Total</div>
                <div class="breakdown-value tokens">{{.Metrics.Agent1Metrics.TotalTokens}}</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Cost</div>
                <div class="breakdown-value cost">${{printf "%.6f" .Metrics.Agent1Metrics.CostUSD}}</div>
            </div>
        </div>

        <h3>Tool Call</h3>
        <p><strong>Tool:</strong> {{.Metrics.Agent1Metrics.ToolCalled}}</p>
        <p><strong>Result:</strong> {{.Metrics.Agent1Metrics.ToolResult}} ({{.Metrics.Agent1Metrics.ProductsFound}} products)</p>

        <p class="collapsible" onclick="toggle('tool-input')">▶ Tool Input (click to expand)</p>
        <div id="tool-input" class="content">
            <pre>{{.Metrics.Agent1Metrics.ToolInput}}</pre>
        </div>
    </div>
    {{end}}

    {{if .Metrics.Agent2Metrics}}
    <div class="section">
        <h2>🎨 Agent 2 (Template Builder)</h2>

        <div class="breakdown">
            <div class="breakdown-item">
                <div class="breakdown-title">Total Time</div>
                <div class="breakdown-value time">{{.Metrics.Agent2Metrics.DurationMs}}ms</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">LLM Call</div>
                <div class="breakdown-value time">{{.Metrics.Agent2Metrics.LLMCallMs}}ms</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Model</div>
                <div class="breakdown-value" style="font-size: 14px;">{{.Metrics.Agent2Metrics.Model}}</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Meta Count</div>
                <div class="breakdown-value">{{.Metrics.Agent2Metrics.MetaCount}}</div>
            </div>
        </div>

        <h3>Tokens & Cache</h3>
        <div class="breakdown">
            <div class="breakdown-item">
                <div class="breakdown-title">Input</div>
                <div class="breakdown-value tokens">{{.Metrics.Agent2Metrics.InputTokens}}</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Output</div>
                <div class="breakdown-value tokens">{{.Metrics.Agent2Metrics.OutputTokens}}</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Cache Write</div>
                <div class="breakdown-value" style="color: #ff9800;">{{.Metrics.Agent2Metrics.CacheCreationInputTokens}}</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Cache Read</div>
                <div class="breakdown-value" style="color: #00ff88;">{{.Metrics.Agent2Metrics.CacheReadInputTokens}}</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Cache Hit Rate</div>
                <div class="breakdown-value {{if gt .Metrics.Agent2Metrics.CacheHitRate 0.0}}cache-hit{{end}}">{{printf "%.1f" .Metrics.Agent2Metrics.CacheHitRate}}%</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Total</div>
                <div class="breakdown-value tokens">{{.Metrics.Agent2Metrics.TotalTokens}}</div>
            </div>
            <div class="breakdown-item">
                <div class="breakdown-title">Cost</div>
                <div class="breakdown-value cost">${{printf "%.6f" .Metrics.Agent2Metrics.CostUSD}}</div>
            </div>
        </div>

        {{if .Metrics.Agent2Metrics.MetaFields}}
        <p><strong>Meta Fields:</strong> {{range .Metrics.Agent2Metrics.MetaFields}}{{.}} {{end}}</p>
        {{end}}

        <p class="collapsible" onclick="toggle('prompt')">▶ Prompt Sent (click to expand)</p>
        <div id="prompt" class="content">
            <pre>{{.Metrics.Agent2Metrics.PromptSent}}</pre>
        </div>

        <p class="collapsible" onclick="toggle('template')">▶ Template JSON (click to expand)</p>
        <div id="template" class="content">
            <pre>{{.Metrics.Agent2Metrics.TemplateJSON}}</pre>
        </div>

        <p class="collapsible" onclick="toggle('raw-resp')">▶ Raw LLM Response (click to expand)</p>
        <div id="raw-resp" class="content">
            <pre>{{.Metrics.Agent2Metrics.RawResponse}}</pre>
        </div>
    </div>
    {{end}}

    {{if .Metrics.Formation}}
    <div class="section">
        <h2>🖼️ Formation Output</h2>
        <p><strong>Mode:</strong> {{.Metrics.Formation.Mode}}</p>
        <p><strong>Widgets:</strong> {{.Metrics.Formation.WidgetCount}}</p>
        {{if .Metrics.Formation.Cols}}<p><strong>Columns:</strong> {{.Metrics.Formation.Cols}}</p>{{end}}
    </div>
    {{end}}
    {{end}}

    {{if .Deltas}}
    <div class="section">
        <h2>📝 Deltas ({{len .Deltas}})</h2>
        <table>
            <tr><th>Step</th><th>Trigger</th><th>Action</th><th>Tool</th><th>Result Count</th></tr>
            {{range .Deltas}}
            <tr>
                <td>{{.Step}}</td>
                <td>{{.Trigger}}</td>
                <td>{{.Action.Type}}</td>
                <td>{{.Action.Tool}}</td>
                <td>{{.Result.Count}}</td>
            </tr>
            {{end}}
        </table>
    </div>
    {{end}}

    <p style="margin-top: 30px; color: #666;">
        <a href="?format=json" style="color: #00d4ff;">View as JSON</a>
    </p>

    <script>
    function toggle(id) {
        var el = document.getElementById(id);
        el.classList.toggle('show');
    }
    </script>
</body>
</html>
`))

