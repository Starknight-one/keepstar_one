package ports

import (
	"context"

	"keepstar_v5/internal/domain"
)

// TracePort persists a per-turn trace into v5_chat_session_traces.
// Best-effort by design: pipeline never blocks on the write, callers
// fire-and-forget via a goroutine. SaveTrace MUST be idempotent on
// (session_id, request_id) — handlers may retry or duplicate the
// same trace under retry semantics, and double-inserts must not
// surface as errors.
type TracePort interface {
	SaveTrace(ctx context.Context, row domain.TraceRow) error
}
