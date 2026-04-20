package logger

import (
	"context"
	"log/slog"
	"os"
)

// Logger wraps slog with domain-specific methods
type Logger struct {
	*slog.Logger
}

// New creates a new logger
func New(level string) *Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return &Logger{slog.New(handler)}
}

// With returns a child logger with additional fields
func (l *Logger) With(args ...any) *Logger {
	return &Logger{l.Logger.With(args...)}
}

// Context keys for request-scoped data
type ctxKey string

const (
	ctxKeyRequestID  ctxKey = "request_id"
	ctxKeySessionID  ctxKey = "session_id"
	ctxKeyTenantSlug ctxKey = "tenant_slug"
)

// WithRequestID attaches a request ID to the context
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID, id)
}

// WithSessionID attaches a session ID to the context
func WithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeySessionID, id)
}

// WithTenantSlug attaches a tenant slug to the context
func WithTenantSlug(ctx context.Context, slug string) context.Context {
	return context.WithValue(ctx, ctxKeyTenantSlug, slug)
}

// RequestIDFrom retrieves the request ID from context
func RequestIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyRequestID).(string)
	return v
}

// SessionIDFrom retrieves the session ID from context
func SessionIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeySessionID).(string)
	return v
}

// TenantSlugFrom retrieves the tenant slug from context
func TenantSlugFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyTenantSlug).(string)
	return v
}

// FromContext returns a logger enriched with fields from the context
func (l *Logger) FromContext(ctx context.Context) *Logger {
	args := make([]any, 0, 6)
	if rid := RequestIDFrom(ctx); rid != "" {
		args = append(args, "request_id", rid)
	}
	if sid := SessionIDFrom(ctx); sid != "" {
		args = append(args, "session_id", sid)
	}
	if ts := TenantSlugFrom(ctx); ts != "" {
		args = append(args, "tenant_slug", ts)
	}
	if len(args) == 0 {
		return l
	}
	return l.With(args...)
}

// ChatMessageReceived logs incoming chat message
func (l *Logger) ChatMessageReceived(sessionID, message string) {
	l.Info("chat_message_received",
		"session_id", sessionID,
		"message_length", len(message),
	)
}

// LLMRequestStarted logs LLM request start
func (l *Logger) LLMRequestStarted(stage string) {
	l.Debug("llm_request_started", "stage", stage)
}

// LLMResponseReceived logs LLM response
func (l *Logger) LLMResponseReceived(stage string, tokens int, durationMs int64) {
	l.Info("llm_response_received",
		"stage", stage,
		"tokens", tokens,
		"duration_ms", durationMs,
	)
}

// LLMUsage logs detailed token usage and cost
func (l *Logger) LLMUsage(stage, model string, inputTokens, outputTokens int, costUSD float64, durationMs int64) {
	l.Info("llm_usage",
		"stage", stage,
		"model", model,
		"input_tokens", inputTokens,
		"output_tokens", outputTokens,
		"total_tokens", inputTokens+outputTokens,
		"cost_usd", costUSD,
		"duration_ms", durationMs,
	)
}

// ToolExecuted logs tool execution
func (l *Logger) ToolExecuted(toolName, sessionID, result string, durationMs int64) {
	l.Info("tool_executed",
		"tool", toolName,
		"session_id", sessionID,
		"result", result,
		"duration_ms", durationMs,
	)
}

// LLMUsageWithCache logs detailed token usage, cost, and cache metrics
func (l *Logger) LLMUsageWithCache(
	stage, model string,
	inputTokens, outputTokens int,
	cacheCreated, cacheRead int,
	costUSD float64,
	durationMs int64,
) {
	totalInput := inputTokens + cacheCreated + cacheRead
	var hitRate float64
	if totalInput > 0 {
		hitRate = float64(cacheRead) / float64(totalInput) * 100
	}

	l.Info("llm_usage",
		"stage", stage,
		"model", model,
		"input_tokens", inputTokens,
		"output_tokens", outputTokens,
		"cache_creation_input_tokens", cacheCreated,
		"cache_read_input_tokens", cacheRead,
		"cache_hit_rate", hitRate,
		"total_tokens", inputTokens+outputTokens+cacheCreated+cacheRead,
		"cost_usd", costUSD,
		"duration_ms", durationMs,
	)
}

// Agent1Completed logs Agent 1 completion with full metrics
func (l *Logger) Agent1Completed(sessionID string, toolCalled string, productsFound int, totalTokens int, costUSD float64, durationMs int64) {
	l.Info("agent1_completed",
		"session_id", sessionID,
		"tool_called", toolCalled,
		"products_found", productsFound,
		"total_tokens", totalTokens,
		"cost_usd", costUSD,
		"duration_ms", durationMs,
	)
}
