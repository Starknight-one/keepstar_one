package postgres

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestIsUndefinedTable guards the discriminator every catalog.* fail-open
// path relies on: only a Postgres 42P01 (undefined_table) — even when wrapped
// — should degrade a read to empty; anything else must still surface as an
// error so real failures aren't silently swallowed.
func TestIsUndefinedTable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("boom"), false},
		{"42P01 undefined_table", &pgconn.PgError{Code: "42P01"}, true},
		{"42P01 wrapped", fmt.Errorf("query products: %w", &pgconn.PgError{Code: "42P01"}), true},
		{"42703 undefined_column", &pgconn.PgError{Code: "42703"}, false},
		{"23505 unique_violation", &pgconn.PgError{Code: "23505"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isUndefinedTable(c.err); got != c.want {
				t.Errorf("isUndefinedTable(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}
