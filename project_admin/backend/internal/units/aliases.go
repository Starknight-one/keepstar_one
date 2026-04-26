package units

import (
	"context"
	"sync"
)

// AliasResolver maps a raw token (e.g. "tube" or "мл") to a canonical unit.
// Implementations:
//   - In-memory (tests, see InMemoryResolver).
//   - DB-backed via PostgresAliasResolver wrapping pgx.Pool.
//
// The resolver is consulted FIRST by the parser; the in-code rawTokenFactors
// map (units.go) is the global fallback. So a tenant can override "fl oz" →
// CanonicalG (silly but possible) without us editing code.
type AliasResolver interface {
	Resolve(rawToken string) (Canonical, bool)
}

// InMemoryResolver is a trivial map-backed resolver, useful in unit tests
// and as a fallback in callers that don't want a DB roundtrip.
type InMemoryResolver struct {
	m map[string]Canonical
}

func NewInMemoryResolver(m map[string]Canonical) *InMemoryResolver {
	if m == nil {
		m = map[string]Canonical{}
	}
	return &InMemoryResolver{m: m}
}

func (r *InMemoryResolver) Resolve(rawToken string) (Canonical, bool) {
	c, ok := r.m[normalizeToken(rawToken)]
	return c, ok
}

// PostgresAliasResolver consults catalog.unit_aliases with tenant-scoped
// preference: tenant-specific row wins over global (tenant_id IS NULL) row.
//
// Caches lookups per (tenant, token) for the lifetime of the resolver — caller
// reconstructs the resolver per import job, so cache TTL maps to job TTL.
type PostgresAliasResolver struct {
	q       AliasQuery
	tenantID string
	cache    sync.Map // map[string]Canonical, "" sentinel means "not found"
}

// AliasQuery is the minimal pgx-backed surface PostgresAliasResolver needs.
// We accept this as an interface so test code can inject a fake without
// importing pgx. Real implementation lives next to the catalog adapter.
type AliasQuery interface {
	LookupAlias(ctx context.Context, tenantID, rawToken string) (string, bool, error)
}

func NewPostgresAliasResolver(q AliasQuery, tenantID string) *PostgresAliasResolver {
	return &PostgresAliasResolver{q: q, tenantID: tenantID}
}

func (r *PostgresAliasResolver) Resolve(rawToken string) (Canonical, bool) {
	tok := normalizeToken(rawToken)
	if v, ok := r.cache.Load(tok); ok {
		c := v.(string)
		if c == "" {
			return "", false
		}
		return Canonical(c), true
	}
	canonical, found, err := r.q.LookupAlias(context.Background(), r.tenantID, tok)
	if err != nil || !found {
		r.cache.Store(tok, "")
		return "", false
	}
	r.cache.Store(tok, canonical)
	return Canonical(canonical), true
}
