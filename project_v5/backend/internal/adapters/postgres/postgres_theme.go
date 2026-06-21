package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"keepstar_v5/internal/domain"
)

// ThemeStore is the read/write surface over v5_themes. GetTheme always returns
// a COMPLETE token set (tenant override merged over canonical defaults), with
// IsDefault=true when the tenant has no row. UpsertTheme stores a tenant's
// override token set for later phases (admin GET/PUT theme).
//
// Tenant identity is the catalog.tenants(id) UUID, keyed on tenant.ID per the
// Phase 1 contract; slug is accepted too (resolveTenantID) for adapter-pattern
// parity with PresetAdapter/ComponentAdapter.
type ThemeStore struct {
	client *Client
}

// NewThemeStore constructs the store over the shared pg client.
func NewThemeStore(client *Client) *ThemeStore {
	return &ThemeStore{client: client}
}

// GetTheme returns the tenant's theme: the stored override token set merged
// over the canonical defaults (so the result is always complete), or the pure
// defaults with IsDefault=true when no row exists. A MISSING ROW is the
// default-token case — NOT an error. An unknown tenant slug surfaces
// domain.ErrTenantNotFound from resolveTenantID.
func (s *ThemeStore) GetTheme(ctx context.Context, tenantSlugOrID string) (domain.Theme, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		end := sc.Start("postgres.GetTheme")
		defer end("")
	}
	tenantID, err := resolveTenantID(ctx, s.client, tenantSlugOrID)
	if err != nil {
		return domain.Theme{}, err
	}

	var raw []byte
	err = s.client.pool.QueryRow(ctx,
		`SELECT tokens FROM v5_themes WHERE tenant_id = $1::uuid`,
		tenantID,
	).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No row → canonical defaults (byte-identical to widget.css).
			return domain.DefaultTheme(), nil
		}
		return domain.Theme{}, fmt.Errorf("get theme: %w", err)
	}

	var override domain.ThemeTokens
	if err := json.Unmarshal(raw, &override); err != nil {
		return domain.Theme{}, fmt.Errorf("get theme: unmarshal tokens: %w", err)
	}
	// A stored row is a tenant override; merge over defaults so missing keys
	// still resolve. IsDefault=false signals a real tenant theme is in play.
	return domain.Theme{
		Tokens:    domain.MergeThemeTokensOverDefaults(override),
		IsDefault: false,
	}, nil
}

// UpsertTheme stores the tenant's override token set (one row per tenant,
// version bumped on each write). Tokens are persisted as given — the override —
// not the merged-with-defaults set, so a later defaults change still flows
// through to keys the tenant didn't customise. Returns ErrTenantNotFound for an
// unknown slug; the FK guards against an unknown UUID.
func (s *ThemeStore) UpsertTheme(ctx context.Context, tenantSlugOrID string, tokens domain.ThemeTokens) error {
	tenantID, err := resolveTenantID(ctx, s.client, tenantSlugOrID)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(tokens)
	if err != nil {
		return fmt.Errorf("upsert theme: marshal tokens: %w", err)
	}
	_, err = s.client.pool.Exec(ctx,
		`INSERT INTO v5_themes (tenant_id, tokens, version, updated_at)
		 VALUES ($1::uuid, $2::jsonb, 1, NOW())
		 ON CONFLICT (tenant_id) DO UPDATE
		   SET tokens     = EXCLUDED.tokens,
		       version    = v5_themes.version + 1,
		       updated_at = NOW()`,
		tenantID, raw,
	)
	if err != nil {
		return fmt.Errorf("upsert theme: %w", err)
	}
	return nil
}
