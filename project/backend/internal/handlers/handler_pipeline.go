package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"keepstar/internal/domain"
	"keepstar/internal/logger"
	"keepstar/internal/usecases"
)

// PipelineHandler handles pipeline requests
type PipelineHandler struct {
	pipelineUC   *usecases.PipelineExecuteUseCase
	metricsStore *MetricsStore
	log          *logger.Logger
}

// NewPipelineHandler creates a pipeline handler
func NewPipelineHandler(pipelineUC *usecases.PipelineExecuteUseCase, metricsStore *MetricsStore, log *logger.Logger) *PipelineHandler {
	return &PipelineHandler{
		pipelineUC:   pipelineUC,
		metricsStore: metricsStore,
		log:          log,
	}
}

// PipelineRequest is the request body
type PipelineRequest struct {
	SessionID string `json:"sessionId"`
	Query     string `json:"query"`
}

// PipelineResponse is the response body
type PipelineResponse struct {
	SessionID          string                         `json:"sessionId"`
	Formation          *FormationResponse             `json:"formation,omitempty"`
	AdjacentTemplates  map[string]*FormationResponse  `json:"adjacentTemplates,omitempty"`
	Entities           *domain.StateData              `json:"entities,omitempty"`
	Agent1Ms           int                            `json:"agent1Ms"`
	Agent2Ms           int                            `json:"agent2Ms"`
	TotalMs            int                            `json:"totalMs"`
}

// FormationResponse is the JSON-friendly formation for HTTP response
type FormationResponse struct {
	Mode    string             `json:"mode"`
	Grid    *domain.GridConfig `json:"grid,omitempty"`
	Widgets []domain.Widget    `json:"widgets"`
}

// HandlePipeline handles POST /api/v1/pipeline
func (h *PipelineHandler) HandlePipeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("handler.pipeline")
		defer endSpan()
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		http.Error(w, "Query is required", http.StatusBadRequest)
		return
	}

	// Generate session ID if not provided
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	// Set session_id in context for logging
	ctx = logger.WithSessionID(ctx, sessionID)
	r = r.WithContext(ctx)

	// Get tenant from context (set by middleware)
	var tenantSlug string
	if tenant := GetTenantFromContext(r.Context()); tenant != nil {
		tenantSlug = tenant.Slug
	}

	reqLog := h.log.FromContext(ctx)
	reqLog.Info("pipeline_start", "query", req.Query)

	turnID := uuid.New().String()
	result, err := h.pipelineUC.Execute(r.Context(), usecases.PipelineExecuteRequest{
		SessionID:  sessionID,
		Query:      req.Query,
		TenantSlug: tenantSlug,
		TurnID:     turnID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Store metrics for debug page
	if h.metricsStore != nil {
		metrics := &PipelineMetrics{
			SessionID: sessionID,
			Query:     req.Query,
			Timestamp: time.Now(),
			TotalMs:   result.TotalMs,
			Agent1Metrics: &AgentMetrics{
				DurationMs:               result.Agent1Ms,
				LLMCallMs:                result.Agent1LLMMs,
				ToolMs:                   result.Agent1ToolMs,
				InputTokens:              result.Agent1Usage.InputTokens,
				OutputTokens:             result.Agent1Usage.OutputTokens,
				TotalTokens:              result.Agent1Usage.TotalTokens,
				CostUSD:                  result.Agent1Usage.CostUSD,
				Model:                    result.Agent1Usage.Model,
				CacheCreationInputTokens: result.Agent1Usage.CacheCreationInputTokens,
				CacheReadInputTokens:     result.Agent1Usage.CacheReadInputTokens,
				CacheHitRate:             cacheHitRate(result.Agent1Usage),
				ToolCalled:               result.ToolCalled,
				ToolInput:                result.ToolInput,
				ToolResult:               result.ToolResult,
				ProductsFound:            result.ProductsFound,
			},
			Agent2Metrics: &AgentMetrics{
				DurationMs:               result.Agent2Ms,
				LLMCallMs:                result.Agent2LLMMs,
				InputTokens:              result.Agent2Usage.InputTokens,
				OutputTokens:             result.Agent2Usage.OutputTokens,
				TotalTokens:              result.Agent2Usage.TotalTokens,
				CostUSD:                  result.Agent2Usage.CostUSD,
				Model:                    result.Agent2Usage.Model,
				CacheCreationInputTokens: result.Agent2Usage.CacheCreationInputTokens,
				CacheReadInputTokens:     result.Agent2Usage.CacheReadInputTokens,
				CacheHitRate:             cacheHitRate(result.Agent2Usage),
				PromptSent:               result.Agent2Prompt,
				RawResponse:              result.Agent2RawResp,
				TemplateJSON:             result.TemplateJSON,
				MetaCount:                result.MetaCount,
				MetaFields:               result.MetaFields,
			},
		}
		if result.Formation != nil {
			metrics.Formation = &FormationInfo{
				Mode:        string(result.Formation.Mode),
				WidgetCount: len(result.Formation.Widgets),
			}
			if result.Formation.Grid != nil {
				metrics.Formation.Cols = result.Formation.Grid.Cols
			}
		}
		h.metricsStore.Store(metrics)
	}

	resp := PipelineResponse{
		SessionID: sessionID,
		Agent1Ms:  result.Agent1Ms,
		Agent2Ms:  result.Agent2Ms,
		TotalMs:   result.TotalMs,
	}

	if result.Formation != nil {
		resp.Formation = &FormationResponse{
			Mode:    string(result.Formation.Mode),
			Grid:    result.Formation.Grid,
			Widgets: result.Formation.Widgets,
		}
	}

	// Serialize adjacent templates for instant expand (1 template per entity type)
	if len(result.AdjacentTemplates) > 0 {
		resp.AdjacentTemplates = make(map[string]*FormationResponse, len(result.AdjacentTemplates))
		for key, f := range result.AdjacentTemplates {
			resp.AdjacentTemplates[key] = &FormationResponse{
				Mode:    string(f.Mode),
				Grid:    f.Grid,
				Widgets: f.Widgets,
			}
		}
	}
	if result.Entities != nil {
		resp.Entities = result.Entities
	}

	writeJSON(w, http.StatusOK, resp)
}

func generateSessionID() string {
	return uuid.New().String()
}

// cacheHitRate calculates cache hit percentage from LLM usage
func cacheHitRate(usage domain.LLMUsage) float64 {
	total := usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens
	if total == 0 {
		return 0
	}
	return float64(usage.CacheReadInputTokens) / float64(total) * 100
}
