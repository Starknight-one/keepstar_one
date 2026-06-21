//go:build integration

// Run with: TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration ./internal/adapters/postgres/...
//
// Exercises ThemeStore against live Neon: a tenant with no v5_themes row
// resolves to the canonical defaults (IsDefault=true), and an upserted override
// survives a round-trip (merged over defaults, IsDefault=false). Cleans up the
// row it writes so the suite is idempotent.

package postgres

import (
	"context"
	"testing"

	"keepstar_v5/internal/domain"
)

func TestThemeStore_NoRowReturnsDefaults(t *testing.T) {
	c := setupClient(t)
	ctx := context.Background()
	if err := c.RunThemeMigrations(ctx); err != nil {
		t.Fatalf("theme migration: %v", err)
	}

	slug := pickHeybabesOrAnyTenantSlug(t, c)
	store := NewThemeStore(c)

	// Ensure no row for a clean defaults assertion.
	tenantID, err := resolveTenantID(ctx, c, slug)
	if err != nil {
		t.Fatalf("resolve tenant: %v", err)
	}
	if _, err := c.pool.Exec(ctx, `DELETE FROM v5_themes WHERE tenant_id = $1::uuid`, tenantID); err != nil {
		t.Fatalf("pre-clean: %v", err)
	}

	got, err := store.GetTheme(ctx, slug)
	if err != nil {
		t.Fatalf("GetTheme: %v", err)
	}
	if !got.IsDefault {
		t.Errorf("no-row GetTheme.IsDefault = false, want true")
	}
	if got.Tokens.Colors["blue"] != "#5BA4D9" {
		t.Errorf("no-row default colors.blue = %q, want #5BA4D9", got.Tokens.Colors["blue"])
	}
	want := domain.DefaultThemeTokens()
	if got.Tokens.Colors["text"] != want.Colors["text"] || got.Tokens.Radius["base"] != want.Radius["base"] {
		t.Errorf("no-row tokens drifted from defaults: %#v", got.Tokens)
	}
}

func TestThemeStore_UpsertRoundTrip(t *testing.T) {
	c := setupClient(t)
	ctx := context.Background()
	if err := c.RunThemeMigrations(ctx); err != nil {
		t.Fatalf("theme migration: %v", err)
	}

	slug := pickHeybabesOrAnyTenantSlug(t, c)
	store := NewThemeStore(c)

	tenantID, err := resolveTenantID(ctx, c, slug)
	if err != nil {
		t.Fatalf("resolve tenant: %v", err)
	}
	defer c.pool.Exec(ctx, `DELETE FROM v5_themes WHERE tenant_id = $1::uuid`, tenantID)

	override := domain.ThemeTokens{
		Colors:   map[string]string{"blue": "#0A0B0C"},
		FontSize: map[string]int{"xl": 99},
	}
	if err := store.UpsertTheme(ctx, slug, override); err != nil {
		t.Fatalf("UpsertTheme: %v", err)
	}

	got, err := store.GetTheme(ctx, slug)
	if err != nil {
		t.Fatalf("GetTheme after upsert: %v", err)
	}
	if got.IsDefault {
		t.Errorf("after upsert IsDefault = true, want false")
	}
	if got.Tokens.Colors["blue"] != "#0A0B0C" {
		t.Errorf("override colors.blue = %q, want #0A0B0C", got.Tokens.Colors["blue"])
	}
	if got.Tokens.FontSize["xl"] != 99 {
		t.Errorf("override fontSize.xl = %d, want 99", got.Tokens.FontSize["xl"])
	}
	// Non-overridden keys merged from defaults.
	if got.Tokens.Colors["orange"] != "#F0924A" {
		t.Errorf("merged default colors.orange = %q, want #F0924A", got.Tokens.Colors["orange"])
	}

	// Second upsert bumps version and replaces tokens.
	override2 := domain.ThemeTokens{Colors: map[string]string{"blue": "#FFFFFF"}}
	if err := store.UpsertTheme(ctx, slug, override2); err != nil {
		t.Fatalf("second UpsertTheme: %v", err)
	}
	var version int
	if err := c.pool.QueryRow(ctx, `SELECT version FROM v5_themes WHERE tenant_id = $1::uuid`, tenantID).Scan(&version); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version != 2 {
		t.Errorf("version after 2 upserts = %d, want 2", version)
	}
	got2, _ := store.GetTheme(ctx, slug)
	if got2.Tokens.Colors["blue"] != "#FFFFFF" {
		t.Errorf("second upsert colors.blue = %q, want #FFFFFF", got2.Tokens.Colors["blue"])
	}
	// fontSize.xl from the first upsert is gone (tokens replaced, not merged-in DB).
	if got2.Tokens.FontSize["xl"] != domain.DefaultThemeTokens().FontSize["xl"] {
		t.Errorf("second upsert should reset fontSize.xl to default, got %d", got2.Tokens.FontSize["xl"])
	}
}
