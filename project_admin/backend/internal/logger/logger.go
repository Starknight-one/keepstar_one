package logger

import (
	"context"
	"log/slog"
	"os"
)

type Logger struct {
	*slog.Logger
}

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
	ctxKeyRequestID ctxKey = "request_id"
	ctxKeyUserID    ctxKey = "user_id"
	ctxKeyTenantID  ctxKey = "tenant_id"
)

// WithRequestID attaches a request ID to the context
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID, id)
}

// WithUserID attaches a user ID to the context
func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyUserID, id)
}

// WithTenantID attaches a tenant ID to the context
func WithTenantID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyTenantID, id)
}

// RequestIDFrom retrieves the request ID from context
func RequestIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyRequestID).(string)
	return v
}

// UserIDFrom retrieves the user ID from context
func UserIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyUserID).(string)
	return v
}

// TenantIDFrom retrieves the tenant ID from context
func TenantIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyTenantID).(string)
	return v
}

// FromContext returns a logger enriched with fields from the context
func (l *Logger) FromContext(ctx context.Context) *Logger {
	args := make([]any, 0, 6)
	if rid := RequestIDFrom(ctx); rid != "" {
		args = append(args, "request_id", rid)
	}
	if uid := UserIDFrom(ctx); uid != "" {
		args = append(args, "user_id", uid)
	}
	if tid := TenantIDFrom(ctx); tid != "" {
		args = append(args, "tenant_id", tid)
	}
	if len(args) == 0 {
		return l
	}
	return l.With(args...)
}

// AdminAction logs an admin action with structured fields
func (l *Logger) AdminAction(action, userID, tenantID, detail string) {
	l.Info("admin_action",
		"action", action,
		"user_id", userID,
		"tenant_id", tenantID,
		"detail", detail,
	)
}
