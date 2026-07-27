package ports

import (
	"context"
	"time"

	"keepstar_v5/internal/domain"
)

// Onboarding persistence ports (RUNTIME_SPEC.md §4.3, §3.3, R5/R25).
//
// OnboardingStatePort is the spec's "StatePort extension" carved out as its
// own narrow port so the onboarding workstream does not widen the shared
// StatePort mid-flight: the postgres StateAdapter satisfies BOTH interfaces.
// Integration note: folding these two methods into StatePort proper is a
// mechanical follow-up (interface lines + the usecases mock), integrator's
// call.

// OnboardingStepAction is the delta vocabulary for onboarding-zone writes —
// the curator-visible assembly log (spec §4.3 "Delta kind onboarding_step").
// Lives here (not domain/delta.go) so this lane touches no shared contract
// file; promoting it to the domain delta vocabulary is an integration nicety.
const OnboardingStepAction domain.ActionType = "ONBOARDING_STEP"

// OnboardingStatePort reads/writes the v5_chat_session_state.onboarding zone.
type OnboardingStatePort interface {
	// GetOnboarding returns the session's staged manifest, or nil when the
	// session exists but has no manifest yet. Unknown session → error.
	GetOnboarding(ctx context.Context, sessionID string) (*domain.OnboardingManifest, error)
	// UpdateOnboarding replaces the manifest zone AND appends the matching
	// delta in one tx (zone-write discipline, same as every other zone).
	// Returns the assigned step.
	UpdateOnboarding(ctx context.Context, sessionID string, m *domain.OnboardingManifest, info domain.DeltaInfo) (int, error)
}

// IngestToken is one v5_ingest_tokens row (spec §3.3): a session-bound,
// single-use, expiring door for the universal uploader (R25). TenantSlug is
// resolved at read time (catalog.tenants join) so the upload proxy can
// address the admin service route without a second lookup.
type IngestToken struct {
	Token      string
	SessionID  string
	TenantID   string
	TenantSlug string
	Formats    []string
	UsedAt     *time.Time
	ExpiresAt  time.Time
	CreatedAt  time.Time
}

// Valid reports whether the token is usable now: unexpired and not yet
// consumed. Format allowance is the caller's check (Formats).
func (t *IngestToken) Valid(now time.Time) bool {
	return t != nil && t.UsedAt == nil && now.Before(t.ExpiresAt)
}

// AllowsFormat reports whether the token admits the given upload format
// ("csv" | "json").
func (t *IngestToken) AllowsFormat(format string) bool {
	for _, f := range t.Formats {
		if f == format {
			return true
		}
	}
	return false
}

// OnboardingTokenPort mints and consumes the onboarding door tokens:
// v5_ingest_tokens (upload door, R25) and v5_surface_tokens (CRM access,
// R13 — minted by the issue_surface_urls applier step).
type OnboardingTokenPort interface {
	// MintIngestToken inserts a fresh upload token bound to the session and
	// tenant. ttl <= 0 uses the table default (24h).
	MintIngestToken(ctx context.Context, sessionID, tenantID string, formats []string, ttl time.Duration) (*IngestToken, error)
	// GetIngestToken resolves a token string; unknown token → error.
	GetIngestToken(ctx context.Context, token string) (*IngestToken, error)
	// MarkIngestTokenUsed stamps used_at (idempotent — already-used is not an
	// error).
	MarkIngestTokenUsed(ctx context.Context, token string) error
	// MintSurfaceToken inserts a CRM surface token for the tenant and returns
	// its value (the ?k= URL capability).
	MintSurfaceToken(ctx context.Context, tenantID string) (string, error)
}
