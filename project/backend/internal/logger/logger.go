package logger

import (
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
