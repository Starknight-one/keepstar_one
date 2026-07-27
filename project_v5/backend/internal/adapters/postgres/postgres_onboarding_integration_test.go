//go:build integration

// Run with: TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration ./internal/adapters/postgres/...
//
// Proves the onboarding persistence against a real Postgres: migration
// idempotency (the deploy story), the manifest zone round trip + its delta,
// and the door-token lifecycle. Cleans up via ON DELETE CASCADE on the
// session row plus explicit token deletes.

package postgres

import (
	"context"
	"testing"
	"time"

	"keepstar_v5/internal/domain"
)

func setupOnboarding(t *testing.T) *Client {
	t.Helper()
	c := setupClient(t)
	ctx := context.Background()
	// Idempotency IS the deploy contract (R3): run twice, no error.
	if err := c.RunOnboardingMigrations(ctx); err != nil {
		t.Fatalf("onboarding migration: %v", err)
	}
	if err := c.RunOnboardingMigrations(ctx); err != nil {
		t.Fatalf("onboarding migration re-run (idempotency): %v", err)
	}
	return c
}

func TestOnboardingZoneRoundTrip(t *testing.T) {
	c := setupOnboarding(t)
	a := NewStateAdapter(c, nil)
	ctx := context.Background()
	sessID := newTestSession(t, c)

	if _, err := a.CreateState(ctx, sessID); err != nil {
		t.Fatalf("create state: %v", err)
	}

	// Fresh session: zone exists, manifest is nil (not an error).
	m, err := a.GetOnboarding(ctx, sessID)
	if err != nil {
		t.Fatalf("get empty onboarding: %v", err)
	}
	if m != nil {
		t.Fatalf("fresh zone = %+v, want nil", m)
	}

	staged := &domain.OnboardingManifest{
		Version: 1,
		Tenant:  domain.ManifestTenant{Name: "Acme Realty", Vertical: "real estate agency"},
		Steps: []domain.ManifestStep{
			{ID: "s1", Op: "create_tenant", Status: domain.ManifestStepProposed,
				Params: map[string]any{"name": "Acme Realty", "vertical": "real estate agency"}},
		},
		UpdatedAt: time.Now(),
	}
	step, err := a.UpdateOnboarding(ctx, sessID, staged, domain.DeltaInfo{
		Trigger:   domain.TriggerSystem,
		Source:    domain.SourceSystem,
		ActorID:   "manifest_applier",
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "onboarding",
		Action:    domain.Action{Type: domain.ActionType("ONBOARDING_STEP"), Tool: "create_tenant", Params: map[string]any{"stepId": "s1"}},
	})
	if err != nil {
		t.Fatalf("update onboarding: %v", err)
	}
	if step < 1 {
		t.Fatalf("assigned step = %d, want >= 1", step)
	}

	got, err := a.GetOnboarding(ctx, sessID)
	if err != nil {
		t.Fatalf("get onboarding: %v", err)
	}
	if got == nil || len(got.Steps) != 1 || got.Steps[0].Op != "create_tenant" || got.Tenant.Name != "Acme Realty" {
		t.Fatalf("round trip lost data: %+v", got)
	}

	// The zone write appended its delta in the same tx.
	deltas, err := a.GetDeltasSince(ctx, sessID, 0)
	if err != nil {
		t.Fatalf("get deltas: %v", err)
	}
	found := false
	for _, d := range deltas {
		if d.Path == "onboarding" && d.Action.Type == domain.ActionType("ONBOARDING_STEP") {
			found = true
		}
	}
	if !found {
		t.Fatalf("onboarding delta missing from stream: %+v", deltas)
	}
}

func TestIngestTokenLifecycle(t *testing.T) {
	c := setupOnboarding(t)
	o := NewOnboardingAdapter(c)
	ctx := context.Background()
	sessID := newTestSession(t, c)
	const tenantID = "9e0f1a2b-3c4d-5e6f-a1b2-c3d4e5f60789"

	tok, err := o.MintIngestToken(ctx, sessID, tenantID, []string{"csv"}, 2*time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	t.Cleanup(func() {
		_, _ = c.pool.Exec(context.Background(), `DELETE FROM v5_ingest_tokens WHERE token = $1::uuid`, tok.Token)
	})
	if tok.Token == "" || tok.ExpiresAt.Before(time.Now().Add(time.Hour)) {
		t.Fatalf("minted token shape wrong: %+v", tok)
	}

	got, err := o.GetIngestToken(ctx, tok.Token)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SessionID != sessID || got.TenantID != tenantID || len(got.Formats) != 1 || got.Formats[0] != "csv" {
		t.Fatalf("token round trip: %+v", got)
	}
	// No catalog.tenants row for the random UUID → slug empty, not an error.
	if got.TenantSlug != "" {
		t.Fatalf("slug for unknown tenant = %q, want empty", got.TenantSlug)
	}
	if !got.Valid(time.Now()) || got.UsedAt != nil {
		t.Fatalf("fresh token invalid: %+v", got)
	}

	// Consume once; the second mark is a no-op, and the stamp survives.
	if err := o.MarkIngestTokenUsed(ctx, tok.Token); err != nil {
		t.Fatalf("mark used: %v", err)
	}
	if err := o.MarkIngestTokenUsed(ctx, tok.Token); err != nil {
		t.Fatalf("mark used again (idempotent): %v", err)
	}
	got, err = o.GetIngestToken(ctx, tok.Token)
	if err != nil {
		t.Fatalf("get after use: %v", err)
	}
	if got.UsedAt == nil || got.Valid(time.Now()) {
		t.Fatalf("used token still valid: %+v", got)
	}

	// Malformed token string → clean error, no panic.
	if _, err := o.GetIngestToken(ctx, "not-a-uuid"); err == nil {
		t.Fatalf("malformed token resolved")
	}
}

func TestSurfaceTokenMint(t *testing.T) {
	c := setupOnboarding(t)
	o := NewOnboardingAdapter(c)
	ctx := context.Background()
	const tenantID = "9e0f1a2b-3c4d-5e6f-a1b2-c3d4e5f60789"

	tok, err := o.MintSurfaceToken(ctx, tenantID)
	if err != nil {
		t.Fatalf("mint surface token: %v", err)
	}
	t.Cleanup(func() {
		_, _ = c.pool.Exec(context.Background(), `DELETE FROM v5_surface_tokens WHERE token = $1`, tok)
	})
	if len(tok) != 64 { // 32 random bytes, hex
		t.Fatalf("token length = %d, want 64", len(tok))
	}
	var surface string
	err = c.pool.QueryRow(ctx,
		`SELECT surface FROM v5_surface_tokens WHERE token = $1 AND revoked_at IS NULL`, tok,
	).Scan(&surface)
	if err != nil || surface != "crm" {
		t.Fatalf("surface row: surface=%q err=%v", surface, err)
	}
}
