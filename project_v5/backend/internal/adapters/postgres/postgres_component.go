package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"keepstar_v5/internal/domain"
)

// ComponentAdapter implements ports.ComponentPort against v5_components +
// v5_component_versions. Mirrors PresetAdapter shape — read-only until the
// canvas-microservice chunk adds the write side.
type ComponentAdapter struct {
	client *Client
}

func NewComponentAdapter(client *Client) *ComponentAdapter {
	return &ComponentAdapter{client: client}
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
			return nil, domain.ErrComponentNotFound
		}
		return nil, fmt.Errorf("query component: %w", err)
	}
	return comp, nil
}

func (a *ComponentAdapter) ListPublishedComponents(ctx context.Context, tenantSlugOrID string) ([]domain.Component, error) {
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
	for rows.Next() {
		comp, err := scanComponent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *comp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter components: %w", err)
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
