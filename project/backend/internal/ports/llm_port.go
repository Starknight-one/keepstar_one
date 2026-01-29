package ports

import "context"

// LLMPort defines the interface for LLM providers
type LLMPort interface {
	// AnalyzeQuery sends user query to LLM for intent analysis (Agent 1)
	AnalyzeQuery(ctx context.Context, req AnalyzeQueryRequest) (*AnalyzeQueryResponse, error)

	// ComposeWidgets sends data to LLM for layout decision (Agent 2)
	ComposeWidgets(ctx context.Context, req ComposeWidgetsRequest) (*ComposeWidgetsResponse, error)
}

// AnalyzeQueryRequest is input for Agent 1
type AnalyzeQueryRequest struct {
	Query       string
	SessionID   string
	ChatHistory []string
}

// AnalyzeQueryResponse is output from Agent 1
type AnalyzeQueryResponse struct {
	Intent       string
	SearchParams map[string]any
	Tokens       int
}

// ComposeWidgetsRequest is input for Agent 2
type ComposeWidgetsRequest struct {
	Query        string
	ProductCount int
	ProductNames []string
}

// ComposeWidgetsResponse is output from Agent 2
type ComposeWidgetsResponse struct {
	WidgetType    string
	FormationType string
	Columns       int
	Tokens        int
}
