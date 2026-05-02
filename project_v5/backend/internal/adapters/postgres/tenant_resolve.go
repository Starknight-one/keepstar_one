package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"keepstar_v5/internal/domain"
)

// resolveTenantID accepts either a slug or a UUID. UUIDs pass through
// (any UUID-shaped string is treated as already-resolved); slugs are
// looked up against catalog.tenants. Shared by PresetAdapter and
// ComponentAdapter — both expose slug-or-id-agnostic methods to callers.
func resolveTenantID(ctx context.Context, c *Client, slugOrID string) (string, error) {
	if isUUID(slugOrID) {
		return slugOrID, nil
	}
	var id string
	err := c.pool.QueryRow(ctx,
		`SELECT id::text FROM catalog.tenants WHERE slug = $1`,
		slugOrID,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", domain.ErrTenantNotFound
		}
		return "", fmt.Errorf("resolve tenant: %w", err)
	}
	return id, nil
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
}
