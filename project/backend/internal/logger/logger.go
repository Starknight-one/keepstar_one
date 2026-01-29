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
