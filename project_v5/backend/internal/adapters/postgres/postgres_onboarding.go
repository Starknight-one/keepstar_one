package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// Onboarding persistence (RUNTIME_SPEC.md §3.3, §4.3, R25):
//   - the v5_chat_session_state.onboarding manifest zone
//     (StateAdapter.GetOnboarding / UpdateOnboarding — the spec's "StatePort
//     extension", exposed via ports.OnboardingStatePort), and
//   - the door tokens: v5_ingest_tokens (upload, R25) + v5_surface_tokens
//     (CRM ?k=, R13) via OnboardingAdapter (ports.OnboardingTokenPort).
//
// DDL lives here too (RunOnboardingMigrations): additive-only, idempotent,
// v5-boot-owned (R3). v5_surface_tokens itself is created by the state
// migration; this file only inserts into it.

// Compile-time port conformance.
var (
	_ ports.OnboardingStatePort = (*StateAdapter)(nil)
	_ ports.OnboardingTokenPort = (*OnboardingAdapter)(nil)
)

// migrationOnboarding: the onboarding manifest zone column + the upload
// token table (spec §3.3 state_migrations additions, kept in this file so
// the onboarding lane owns its DDL without editing state_migrations.go).
const migrationOnboarding = `
ALTER TABLE v5_chat_session_state ADD COLUMN IF NOT EXISTS onboarding JSONB;

CREATE TABLE IF NOT EXISTS v5_ingest_tokens (
  token UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id UUID NOT NULL,
  tenant_id UUID NOT NULL,
  formats TEXT[] NOT NULL DEFAULT '{csv,json}',
  used_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

// RunOnboardingMigrations applies the onboarding schema. Idempotent;
// ordered after RunStateMigrations (it ALTERs v5_chat_session_state and
// its sibling v5_surface_tokens table rides the state migration).
func (c *Client) RunOnboardingMigrations(ctx context.Context) error {
	if _, err := c.pool.Exec(ctx, migrationOnboarding); err != nil {
		return fmt.Errorf("v5 onboarding migration: %w", err)
	}
	return nil
}

// --- manifest zone (ports.OnboardingStatePort on *StateAdapter) ---

// GetOnboarding returns the session's staged manifest, or nil when the
// session has no manifest yet. Unknown session → error.
func (a *StateAdapter) GetOnboarding(ctx context.Context, sessionID string) (*domain.OnboardingManifest, error) {
	var raw []byte
	err := a.client.pool.QueryRow(ctx, `
		SELECT onboarding FROM v5_chat_session_state WHERE session_id = $1::uuid
	`, sessionID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get onboarding: session state not found: %s", sessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("get onboarding: %w", err)
	}
	if len(raw) == 0 {
		return nil, nil // session exists, nothing staged yet
	}
	var m domain.OnboardingManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("get onboarding: unmarshal manifest: %w", err)
	}
	return &m, nil
}

// UpdateOnboarding replaces the manifest zone and appends the matching
// delta in ONE tx (zone-write discipline — same machinery as every other
// zone). Returns the assigned step.
func (a *StateAdapter) UpdateOnboarding(ctx context.Context, sessionID string, m *domain.OnboardingManifest, info domain.DeltaInfo) (int, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return 0, fmt.Errorf("update onboarding: marshal manifest: %w", err)
	}
	delta := info.ToDelta()
	step, err := a.zoneWriteWithDelta(ctx, sessionID, delta, `
		UPDATE v5_chat_session_state
		SET onboarding = $1, updated_at = NOW()
		WHERE session_id = $2
	`, raw, sessionID)
	if err != nil {
		return 0, fmt.Errorf("update onboarding: %w", err)
	}
	return step, nil
}

// --- door tokens (ports.OnboardingTokenPort) ---

// OnboardingAdapter implements ports.OnboardingTokenPort.
type OnboardingAdapter struct {
	client *Client
}

// NewOnboardingAdapter constructs the token adapter.
func NewOnboardingAdapter(client *Client) *OnboardingAdapter {
	return &OnboardingAdapter{client: client}
}

// MintIngestToken inserts a fresh upload token (R25: session-bound,
// single-use, expiring). ttl <= 0 keeps the table default (24h).
func (o *OnboardingAdapter) MintIngestToken(ctx context.Context, sessionID, tenantID string, formats []string, ttl time.Duration) (*ports.IngestToken, error) {
	if len(formats) == 0 {
		formats = []string{"csv", "json"}
	}
	t := &ports.IngestToken{SessionID: sessionID, TenantID: tenantID, Formats: formats}
	var err error
	if ttl > 0 {
		err = o.client.pool.QueryRow(ctx, `
			INSERT INTO v5_ingest_tokens (session_id, tenant_id, formats, expires_at)
			VALUES ($1::uuid, $2::uuid, $3, NOW() + $4::interval)
			RETURNING token::text, expires_at, created_at
		`, sessionID, tenantID, formats, fmt.Sprintf("%d seconds", int(ttl.Seconds()))).
			Scan(&t.Token, &t.ExpiresAt, &t.CreatedAt)
	} else {
		err = o.client.pool.QueryRow(ctx, `
			INSERT INTO v5_ingest_tokens (session_id, tenant_id, formats)
			VALUES ($1::uuid, $2::uuid, $3)
			RETURNING token::text, expires_at, created_at
		`, sessionID, tenantID, formats).
			Scan(&t.Token, &t.ExpiresAt, &t.CreatedAt)
	}
	if err != nil {
		return nil, fmt.Errorf("mint ingest token: %w", err)
	}
	return t, nil
}

// GetIngestToken resolves a token, joining the tenant slug so the upload
// proxy can address admin's slug-keyed routes in one read. v5 reads
// catalog.tenants read-only — the join is a read, not a coupling widening.
func (o *OnboardingAdapter) GetIngestToken(ctx context.Context, token string) (*ports.IngestToken, error) {
	t := &ports.IngestToken{}
	err := o.client.pool.QueryRow(ctx, `
		SELECT it.token::text, it.session_id::text, it.tenant_id::text,
		       COALESCE(ct.slug, ''), it.formats, it.used_at, it.expires_at, it.created_at
		FROM v5_ingest_tokens it
		LEFT JOIN catalog.tenants ct ON ct.id = it.tenant_id
		WHERE it.token = $1::uuid
	`, token).Scan(&t.Token, &t.SessionID, &t.TenantID, &t.TenantSlug,
		&t.Formats, &t.UsedAt, &t.ExpiresAt, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("ingest token not found")
	}
	if err != nil {
		// A malformed (non-UUID) token string also lands here — same outcome
		// for the caller: not a valid token.
		return nil, fmt.Errorf("get ingest token: %w", err)
	}
	return t, nil
}

// MarkIngestTokenUsed stamps used_at once (idempotent: already-used rows are
// left untouched, no error).
func (o *OnboardingAdapter) MarkIngestTokenUsed(ctx context.Context, token string) error {
	_, err := o.client.pool.Exec(ctx, `
		UPDATE v5_ingest_tokens SET used_at = NOW()
		WHERE token = $1::uuid AND used_at IS NULL
	`, token)
	if err != nil {
		return fmt.Errorf("mark ingest token used: %w", err)
	}
	return nil
}

// MintSurfaceToken inserts a CRM surface token (R13; consumed by session
// init's ?k= validation) and returns its value: 32 random bytes, hex —
// unguessable-URL grade, said out loud in the spec's risk list.
func (o *OnboardingAdapter) MintSurfaceToken(ctx context.Context, tenantID string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("mint surface token: entropy: %w", err)
	}
	token := hex.EncodeToString(buf)
	if _, err := o.client.pool.Exec(ctx, `
		INSERT INTO v5_surface_tokens (tenant_id, surface, token)
		VALUES ($1::uuid, 'crm', $2)
	`, tenantID, token); err != nil {
		return "", fmt.Errorf("mint surface token: %w", err)
	}
	return token, nil
}
