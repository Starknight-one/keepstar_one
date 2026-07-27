package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"

	"keepstar_v5/internal/ports"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// onboardCookieName is the HttpOnly cookie the R5 gate endpoint sets.
	onboardCookieName = "ks_onboard"
	// onboardHMACMessage is the fixed message HMAC'd with ONBOARDING_PASSWORD
	// to derive the cookie value (R5). Rotating the password invalidates
	// every outstanding cookie by construction.
	onboardHMACMessage = "keepstar-onboard-v1"
	// onboardingTenantSlug is the pre-seeded system tenant every onboarding
	// session lives under (admin-owned data seed, RUNTIME_SPEC.md §3.4).
	onboardingTenantSlug = "keepstar-onboarding"
)

// expectedOnboardCookie derives the one valid ks_onboard cookie value:
// hex(HMAC-SHA256("keepstar-onboard-v1", key=ONBOARDING_PASSWORD)).
func expectedOnboardCookie(password string) string {
	mac := hmac.New(sha256.New, []byte(password))
	mac.Write([]byte(onboardHMACMessage))
	return hex.EncodeToString(mac.Sum(nil))
}

// checkOnboardCookie gates the /api/v1/onboard/* routes (R5). Empty
// ONBOARDING_PASSWORD env ⇒ 503 (onboarding disabled, never accidentally
// open — mirrors checkInternalKey). Absent or wrong cookie ⇒ 403.
// hmac.Equal is constant-time.
func checkOnboardCookie(w http.ResponseWriter, r *http.Request) bool {
	password := os.Getenv("ONBOARDING_PASSWORD")
	if password == "" {
		http.Error(w, "onboarding disabled", http.StatusServiceUnavailable)
		return false
	}
	ck, err := r.Cookie(onboardCookieName)
	if err != nil || ck.Value == "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	if !hmac.Equal([]byte(ck.Value), []byte(expectedOnboardCookie(password))) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

// OnboardHandler owns the /api/v1/onboard/* routes. v1 of this file ships
// session creation only; the gate endpoint that SETS the ks_onboard cookie
// (POST /api/v1/onboard/gate), GET session resume, upload and step-submit
// land in M3 (RUNTIME_SPEC.md §7 M3).
//
// These routes are exempt from WithTenant (middleware_tenant.go): the
// onboarding surface is tenant-agnostic to the caller — the system tenant
// is resolved server-side, never from the X-Tenant-Slug header.
type OnboardHandler struct {
	state   ports.StatePort
	catalog ports.CatalogPort
	pool    *pgxpool.Pool // v5_chat_sessions INSERT sits outside StatePort (see SessionHandler)
	log     *slog.Logger
}

// NewOnboardHandler constructs the handler.
func NewOnboardHandler(state ports.StatePort, catalog ports.CatalogPort, pool *pgxpool.Pool, log *slog.Logger) *OnboardHandler {
	return &OnboardHandler{state: state, catalog: catalog, pool: pool, log: log}
}

// CreateSession handles POST /api/v1/onboard/session — the ONLY way a
// mode='onboarding' session comes into existence (R17). Cookie-gated per R5.
// The session is created under the keepstar-onboarding system tenant with
// role=visitor (R14: meta-operations are min_role=visitor — the cookie gate
// does the work).
//
// Response: {sessionId, mode, tenant: {slug, name}}.
func (h *OnboardHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	if !checkOnboardCookie(w, r) {
		return
	}

	tenant, err := h.catalog.GetTenantBySlug(r.Context(), onboardingTenantSlug)
	if err != nil || tenant == nil {
		// The system tenant is an admin-owned seed (§3.4) — its absence is a
		// deploy-order problem, not a client error. Fail honest, not open.
		h.log.Error("onboarding_tenant_unresolved", "slug", onboardingTenantSlug, "err", err)
		http.Error(w, "onboarding tenant unavailable", http.StatusServiceUnavailable)
		return
	}

	var sessionID string
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO v5_chat_sessions (tenant_id, mode, role) VALUES ($1::uuid, 'onboarding', 'visitor') RETURNING id`,
		tenant.ID,
	).Scan(&sessionID)
	if err != nil {
		http.Error(w, "session create failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := h.state.CreateState(r.Context(), sessionID); err != nil {
		http.Error(w, "state init failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessionId": sessionID,
		"mode":      "onboarding",
		"tenant": map[string]string{
			"slug": tenant.Slug,
			"name": tenant.Name,
		},
	})
}
