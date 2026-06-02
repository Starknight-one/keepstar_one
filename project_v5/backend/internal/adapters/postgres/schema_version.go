package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// CatalogSchemaVersion returns the version stamped in catalog.schema_version by
// admin (the sole owner of catalog.* migrations). present=false (with no error)
// means the table or row does not exist yet — callers should treat that as
// "unknown / fail open", not a hard failure, so the engine never self-outages
// over a missing version row during a rollout.
func (c *Client) CatalogSchemaVersion(ctx context.Context) (version int, present bool, err error) {
	err = c.pool.QueryRow(ctx, `SELECT version FROM catalog.schema_version WHERE id = true`).Scan(&version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P01" { // undefined_table
			return 0, false, nil
		}
		return 0, false, err
	}
	return version, true, nil
}
