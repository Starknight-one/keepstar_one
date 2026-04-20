package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"keepstar_v4/internal/ports"
)

// CanvasDesignContextAdapter reads the tenant's design context version counter
// and design tokens from the admin storage layer. Both methods are slug/UUID
// tolerant via a catalog.tenants JOIN, mirroring CanvasPresetAdapter.
//
// READ-ONLY from the V4 chat backend's perspective.
type CanvasDesignContextAdapter struct {
	client *Client
}

// NewCanvasDesignContextAdapter constructs the adapter.
func NewCanvasDesignContextAdapter(client *Client) *CanvasDesignContextAdapter {
	return &CanvasDesignContextAdapter{client: client}
}

// compile-time interface assertion.
var _ ports.CanvasDesignContextPort = (*CanvasDesignContextAdapter)(nil)

// GetVersion returns admin.tenant_design_context_version.version for the
// tenant. Returns (0, nil) when the row is missing — this is the "never
// published" case, equivalent to version 0.
func (a *CanvasDesignContextAdapter) GetVersion(ctx context.Context, tenantIDOrSlug string) (int64, error) {
	if tenantIDOrSlug == "" {
		return 0, nil
	}

	query := `
		SELECT v.version
		FROM admin.tenant_design_context_version v
		JOIN catalog.tenants t ON t.id = v.tenant_id
		WHERE v.tenant_id::text = $1 OR t.slug = $1
	`

	var version int64
	err := a.client.pool.QueryRow(ctx, query, tenantIDOrSlug).Scan(&version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("get design context version: %w", err)
	}
	return version, nil
}

// ListDesignTokens returns every token row for the tenant, ordered by
// category, name, theme_axis, theme_value. Returns (nil, nil) for an empty
// tenant string.
func (a *CanvasDesignContextAdapter) ListDesignTokens(ctx context.Context, tenantIDOrSlug string) ([]ports.CanvasDesignTokenView, error) {
	if tenantIDOrSlug == "" {
		return nil, nil
	}

	query := `
		SELECT dt.category, dt.name, dt.value,
		       COALESCE(dt.theme_axis, ''), COALESCE(dt.theme_value, '')
		FROM admin.tenant_design_tokens dt
		JOIN catalog.tenants t ON t.id = dt.tenant_id
		WHERE dt.tenant_id::text = $1 OR t.slug = $1
		ORDER BY dt.category, dt.name, dt.theme_axis, dt.theme_value
	`

	rows, err := a.client.pool.Query(ctx, query, tenantIDOrSlug)
	if err != nil {
		return nil, fmt.Errorf("list design tokens: %w", err)
	}
	defer rows.Close()

	var out []ports.CanvasDesignTokenView
	for rows.Next() {
		var tok ports.CanvasDesignTokenView
		if err := rows.Scan(
			&tok.Category,
			&tok.Name,
			&tok.Value,
			&tok.ThemeAxis,
			&tok.ThemeValue,
		); err != nil {
			return nil, fmt.Errorf("scan design token: %w", err)
		}
		out = append(out, tok)
	}
	return out, rows.Err()
}
