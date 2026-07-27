package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/usecases"

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
	// onboardCookieMaxAge is the R5 cookie lifetime: 7 days.
	onboardCookieMaxAge = 7 * 24 * 3600
	// onboardGateRatePerMin is the R5 gate rate limit: 5/min/IP. Fixed by
	// the spec — no env knob.
	onboardGateRatePerMin = 5
	// OnboardingTurnCap caps one onboarding session at 60 turns (§6.3). The
	// pipeline handler enforces it for mode=onboarding sessions via
	// OnboardingTurnCapReached and answers with graceful text, not an error.
	OnboardingTurnCap = 60
)

// expectedOnboardCookie derives the one valid ks_onboard cookie value:
// hex(HMAC-SHA256("keepstar-onboard-v1", key=ONBOARDING_PASSWORD)).
func expectedOnboardCookie(password string) string {
	mac := hmac.New(sha256.New, []byte(password))
	mac.Write([]byte(onboardHMACMessage))
	return hex.EncodeToString(mac.Sum(nil))
}

// checkOnboardCookie gates the /api/v1/onboard/* routes (R5) — and, via the
// pipeline handler, every pipeline call on a mode=onboarding session
// (§6.3). Empty ONBOARDING_PASSWORD env ⇒ 503 (onboarding disabled, never
// accidentally open — mirrors checkInternalKey). Absent or wrong cookie ⇒
// 403. hmac.Equal is constant-time.
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

// OnboardingTurnsUsed counts the user turns in a session's conversation
// history (one Role=="user" non-tool message per turn — tool_result
// messages also carry role user on the wire but are internal).
func OnboardingTurnsUsed(history []domain.LLMMessage) int {
	n := 0
	for _, msg := range history {
		if msg.Role == "user" && msg.ToolResult == nil {
			n++
		}
	}
	return n
}

// OnboardingTurnCapReached reports whether the session exhausted its 60-turn
// budget (§6.3).
func OnboardingTurnCapReached(history []domain.LLMMessage) bool {
	return OnboardingTurnsUsed(history) >= OnboardingTurnCap
}

// OnboardHandler owns the /api/v1/onboard/* routes: the R5 gate, session
// create/resume, the R25 upload door (handler_onboard_upload.go) and the R6
// step-submit PII choke (handler_onboard_step.go).
//
// These routes are exempt from WithTenant (middleware_tenant.go): the
// onboarding surface is tenant-agnostic to the caller — the system tenant
// is resolved server-side, never from the X-Tenant-Slug header.
type OnboardHandler struct {
	state   ports.StatePort
	catalog ports.CatalogPort
	pool    *pgxpool.Pool // v5_chat_sessions INSERT sits outside StatePort (see SessionHandler)
	log     *slog.Logger

	// gateGuard rate-limits POST /onboard/gate at the spec-fixed 5/min/IP
	// (R5). Constructed internally — tests may swap it.
	gateGuard *PipelineGuard

	// Runtime deps (SetOnboardingDeps). Endpoints needing an unset dep
	// answer 503 honestly — never a silent no-op.
	onboardState ports.OnboardingStatePort
	tokens       ports.OnboardingTokenPort
	gateway      ports.AdminGatewayPort
	applier      *usecases.ManifestApplier
	presets      ports.PresetPort
	components   ports.ComponentPort
	themes       ports.ThemePort
	cheapGuard   *PipelineGuard
}

// NewOnboardHandler constructs the handler. The M3 runtime dependencies
// (manifest state, tokens, admin gateway, applier, plaque render ports)
// arrive via SetOnboardingDeps — the split keeps this constructor signature
// stable for main.go (setter precedent: EntityWrite.SetAutomationRunner).
func NewOnboardHandler(state ports.StatePort, catalog ports.CatalogPort, pool *pgxpool.Pool, log *slog.Logger) *OnboardHandler {
	return &OnboardHandler{
		state:     state,
		catalog:   catalog,
		pool:      pool,
		log:       log,
		gateGuard: NewPipelineGuard(onboardGateRatePerMin, 0, log),
	}
}

// OnboardDeps carries the M3 runtime dependencies for SetOnboardingDeps.
type OnboardDeps struct {
	OnboardState ports.OnboardingStatePort // manifest zone (resume + upload completion)
	Tokens       ports.OnboardingTokenPort // v5_ingest_tokens validation
	Gateway      ports.AdminGatewayPort    // admin service routes (import proxy)
	Applier      *usecases.ManifestApplier // step transitions + step-submit
	Presets      ports.PresetPort          // success_plaque zero-LLM render
	Components   ports.ComponentPort       // optional component merge in the render
	Themes       ports.ThemePort           // theme attach on the rendered block
	CheapGuard   *PipelineGuard            // §6.3 shared cheap bucket; nil disables
}

// SetOnboardingDeps wires the runtime dependencies (boot, main.go).
func (h *OnboardHandler) SetOnboardingDeps(d OnboardDeps) {
	h.onboardState = d.OnboardState
	h.tokens = d.Tokens
	h.gateway = d.Gateway
	h.applier = d.Applier
	h.presets = d.Presets
	h.components = d.Components
	h.themes = d.Themes
	h.cheapGuard = d.CheapGuard
}

// checkCheap applies the §6.3 cheap bucket (nil guard allows everything).
func (h *OnboardHandler) checkCheap(w http.ResponseWriter, r *http.Request) bool {
	if ok, _ := h.cheapGuard.Allow(clientIP(r)); !ok {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return false
	}
	return true
}

// Gate handles POST /api/v1/onboard/gate (R5): {password} → constant-time
// compare vs env ONBOARDING_PASSWORD → HttpOnly ks_onboard cookie
// (SameSite=Lax, 7d). 503 when the env is unset; rate-limited 5/min/IP.
// There is no second secret and no token header — the HMAC cookie is
// derived from the password itself, so rotation kicks every cookie.
func (h *OnboardHandler) Gate(w http.ResponseWriter, r *http.Request) {
	if ok, _ := h.gateGuard.Allow(clientIP(r)); !ok {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	password := os.Getenv("ONBOARDING_PASSWORD")
	if password == "" {
		http.Error(w, "onboarding disabled", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(password)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     onboardCookieName,
		Value:    expectedOnboardCookie(password),
		Path:     "/",
		MaxAge:   onboardCookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsHTTPS(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// requestIsHTTPS detects TLS termination (direct or behind Railway's proxy)
// so the Secure cookie attribute is set exactly when it can work.
func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
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

// GetSession handles GET /api/v1/onboard/session?sessionId= — resume
// (refresh-safe, all truth in DB, §5.1): the staged manifest, the last
// rendered template and the chat transcript. Cookie-gated; the session must
// be a live mode=onboarding session (anything else → 404, no leak).
func (h *OnboardHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	if !checkOnboardCookie(w, r) {
		return
	}
	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		http.Error(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	var mode string
	var killedAt *time.Time
	err := h.pool.QueryRow(r.Context(),
		`SELECT COALESCE(mode, ''), killed_at FROM v5_chat_sessions WHERE id = $1::uuid`,
		sessionID,
	).Scan(&mode, &killedAt)
	if err != nil {
		// Unknown id and malformed UUID both land here — same 404.
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if killedAt != nil {
		http.Error(w, "session killed", http.StatusConflict)
		return
	}
	if mode != string(domain.ModeOnboarding) {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	state, err := h.state.GetState(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, domain.ErrSessionKilled) {
			http.Error(w, "session killed", http.StatusConflict)
			return
		}
		http.Error(w, "session state unavailable", http.StatusInternalServerError)
		return
	}

	var manifest *domain.OnboardingManifest
	if h.onboardState != nil {
		if m, err := h.onboardState.GetOnboarding(r.Context(), sessionID); err == nil {
			manifest = m
		} else {
			h.log.Warn("onboard resume: manifest read failed", "session", sessionID, "err", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sessionId":    sessionID,
		"mode":         string(domain.ModeOnboarding),
		"manifest":     manifest,
		"lastTemplate": state.Current.Template,
		"transcript":   transcriptOf(state.ConversationHistory),
		"turnsUsed":    OnboardingTurnsUsed(state.ConversationHistory),
		"turnCap":      OnboardingTurnCap,
	})
}

// transcriptOf projects the LLM conversation history onto the resume
// transcript: plain user/assistant text turns only — tool traffic stays
// server-side.
func transcriptOf(history []domain.LLMMessage) []map[string]string {
	out := make([]map[string]string, 0, len(history))
	for _, msg := range history {
		if msg.ToolResult != nil || len(msg.ToolCalls) > 0 {
			continue
		}
		if (msg.Role == "user" || msg.Role == "assistant") && msg.Content != "" {
			out = append(out, map[string]string{"role": msg.Role, "text": msg.Content})
		}
	}
	return out
}
