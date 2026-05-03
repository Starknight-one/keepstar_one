package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine/presets"
)

// ComponentAdapter implements ports.ComponentPort against v5_components +
// v5_component_versions. Mirrors PresetAdapter shape — read-only until the
// canvas-microservice chunk adds the write side.
//
// systemRegistry is optional: when non-nil and the DB has no row for
// the requested component (or for ListPublishedComponents — not in the
// returned set yet), the adapter falls back to the registry to serve
// the embedded system component. Mirrors PresetAdapter.systemRegistry
// — used for reusable subtrees that the system presets reference
// (price-rating-root, brand-badge-root).
type ComponentAdapter struct {
	client         *Client
	systemRegistry *presets.SystemComponentRegistry
}

// NewComponentAdapter returns an adapter without system fallback.
// Tests use this to keep DB-only behaviour explicit.
func NewComponentAdapter(client *Client) *ComponentAdapter {
	return &ComponentAdapter{client: client}
}

// NewComponentAdapterWithSystem constructs an adapter wired to a system
// component registry. Production main.go uses this so cards rendered
// from system presets can find their reusable subtrees even before any
// tenant has authored them.
func NewComponentAdapterWithSystem(client *Client, registry *presets.SystemComponentRegistry) *ComponentAdapter {
	return &ComponentAdapter{client: client, systemRegistry: registry}
}

const componentSelect = `
    SELECT
        c.id::text, c.tenant_id::text, c.name, c.category, c.description,
        v.version, v.status, v.doc_json,
        v.published_at, c.created_at, c.updated_at
    FROM v5_components c
    JOIN v5_component_versions v ON v.component_id = c.id
`

func (a *ComponentAdapter) GetPublishedComponent(ctx context.Context, tenantSlugOrID string, name string) (*domain.Component, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		end := sc.Start("postgres.GetPublishedComponent")
		defer end(name)
	}
	tenantID, err := resolveTenantID(ctx, a.client, tenantSlugOrID)
	if err != nil {
		return nil, err
	}
	row := a.client.pool.QueryRow(ctx, componentSelect+`
        WHERE c.tenant_id = $1::uuid
          AND c.name = $2
          AND v.status = 'published'
        ORDER BY v.version DESC
        LIMIT 1
    `, tenantID, name)

	comp, err := scanComponent(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if sys := a.systemComponentFallback(name); sys != nil {
				return sys, nil
			}
			return nil, domain.ErrComponentNotFound
		}
		return nil, fmt.Errorf("query component: %w", err)
	}
	return comp, nil
}

// systemComponentFallback wraps the named system-registry payload (if
// any) as a domain.Component. Mirrors PresetAdapter.systemPresetFallback
// — gives a synthetic identity (id=system:<name>, tenantID=system) so
// downstream observers can distinguish defaults from authored versions.
func (a *ComponentAdapter) systemComponentFallback(name string) *domain.Component {
	if a.systemRegistry == nil {
		return nil
	}
	body, ok := a.systemRegistry.Get(name)
	if !ok {
		return nil
	}
	now := time.Now().UTC()
	return &domain.Component{
		ID:           "system:" + name,
		TenantID:     "system",
		Name:         name,
		Category:     "system",
		Description:  "system-default component",
		Version:      0,
		Status:       domain.ComponentStatusPublished,
		DocumentJSON: json.RawMessage(append([]byte(nil), body...)),
		PublishedAt:  &now,
		CreatedAt:    now,
		UpdatedAt:    now,
		IsSystem:     true,
	}
}

func (a *ComponentAdapter) ListPublishedComponents(ctx context.Context, tenantSlugOrID string) ([]domain.Component, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		end := sc.Start("postgres.ListPublishedComponents")
		defer end()
	}
	tenantID, err := resolveTenantID(ctx, a.client, tenantSlugOrID)
	if err != nil {
		return nil, err
	}
	rows, err := a.client.pool.Query(ctx, `
        SELECT DISTINCT ON (c.id)
            c.id::text, c.tenant_id::text, c.name, c.category, c.description,
            v.version, v.status, v.doc_json,
            v.published_at, c.created_at, c.updated_at
        FROM v5_components c
        JOIN v5_component_versions v ON v.component_id = c.id
        WHERE c.tenant_id = $1::uuid
          AND v.status = 'published'
        ORDER BY c.id, v.version DESC
    `, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list components: %w", err)
	}
	defer rows.Close()

	var out []domain.Component
	dbHave := map[string]bool{}
	for rows.Next() {
		comp, err := scanComponent(rows)
		if err != nil {
			return nil, err
		}
		dbHave[comp.Name] = true
		out = append(out, *comp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter components: %w", err)
	}
	// Union with system registry — DB rows already win because we skip
	// any registry name the DB returned. Mirrors PresetAdapter union.
	if a.systemRegistry != nil {
		for _, name := range a.systemRegistry.List() {
			if dbHave[name] {
				continue
			}
			if sys := a.systemComponentFallback(name); sys != nil {
				out = append(out, *sys)
			}
		}
	}
	sortComponentsByName(out)
	return out, nil
}

func scanComponent(row rowScanner) (*domain.Component, error) {
	var (
		c           domain.Component
		docJSON     []byte
		publishedAt *time.Time
	)
	err := row.Scan(
		&c.ID, &c.TenantID, &c.Name, &c.Category, &c.Description,
		&c.Version, &c.Status, &docJSON,
		&publishedAt, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(docJSON) > 0 {
		if !json.Valid(docJSON) {
			return nil, fmt.Errorf("doc_json is not valid JSON")
		}
		c.DocumentJSON = json.RawMessage(append([]byte(nil), docJSON...))
	}
	c.PublishedAt = publishedAt
	return &c, nil
}

func sortComponentsByName(s []domain.Component) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1].Name > s[j].Name; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
