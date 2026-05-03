package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ReadyzHandler returns 200 if the backing Postgres pool can serve a ping
// inside the timeout, otherwise 503. Distinct from /healthz (process-alive
// only) — Railway uses /healthz for liveness; /readyz exists for our own
// smoke against a freshly-deployed image to confirm DB plumbing landed.
//
// Closes over the pool so the route registration stays a `func(w,r)` and the
// pool wiring is explicit at construction time.
func ReadyzHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
		defer cancel()

		if err := pool.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "not_ready",
				"reason": err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}
