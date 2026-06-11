package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine/presets"
)

// PresetAdapter implements ports.PresetPort against v5_presets +
// v5_preset_versions. Read-only — write side will land with the future
// v9-canvas microservice (Stream B).
//
// When the DB returns ErrPresetNotFound (tenant has not authored the
// requested preset), the adapter falls back to systemRegistry to serve
// one of the in-process system presets shipped with the binary. DB
// hits always win — the registry is a back-stop, not an override.
// systemRegistry is optional: nil means "no fallback" (used by tests
// that exercise pure DB behaviour).
type PresetAdapter struct {
	client         *Client
	systemRegistry *presets.SystemPresetRegistry
}

// NewPresetAdapter constructs an adapter without a system fallback.
// Useful for tests of the raw DB path.
func NewPresetAdapter(client *Client) *PresetAdapter {
	return &PresetAdapter{client: client}
}

// NewPresetAdapterWithSystem constructs an adapter with a system preset
// fallback wired in. Production code path; main.go uses this.
func NewPresetAdapterWithSystem(client *Client, registry *presets.SystemPresetRegistry) *PresetAdapter {
	return &PresetAdapter{client: client, systemRegistry: registry}
}

// resolveTenantID + isUUID moved to tenant_resolve.go (shared with
// ComponentAdapter in chunk 5).

// presetSelect is the JOIN both Get and List share. Column order is
// preserved by scanPreset.
const presetSelect = `
    SELECT
        p.id::text, p.tenant_id::text, p.name, p.category, p.entity_type,
        p.description, p.default_replicate,
        v.version, v.status, v.doc_json,
        v.published_at, p.created_at, p.updated_at
    FROM v5_presets p
    JOIN v5_preset_versions v ON v.preset_id = p.id
`

func (a *PresetAdapter) GetPublishedPreset(ctx context.Context, tenantSlugOrID string, name string) (*domain.Preset, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		end := sc.Start("postgres.GetPublishedPreset")
		defer end(name)
	}
	tenantID, err := resolveTenantID(ctx, a.client, tenantSlugOrID)
	if err != nil {
		return nil, err
	}
	row := a.client.pool.QueryRow(ctx, presetSelect+`
        WHERE p.tenant_id = $1::uuid
          AND p.name = $2
          AND v.status = 'published'
        ORDER BY v.version DESC
        LIMIT 1
    `, tenantID, name)

	preset, err := scanPreset(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// DB miss → try system fallback before declaring «not found».
			if sys := a.systemPresetFallback(name); sys != nil {
				slog.Debug("preset.system_fallback", "name", name, "tenant", tenantSlugOrID)
				return sys, nil
			}
			return nil, domain.ErrPresetNotFound
		}
		return nil, fmt.Errorf("query preset: %w", err)
	}
	return preset, nil
}

// systemPresetFallback wraps the named system-registry payload (if any)
// as a domain.Preset. Used by GetPublishedPreset when DB has no row;
// IsSystem flag lets downstream observers (tracing, future canvas)
// distinguish defaults from tenant-authored versions.
func (a *PresetAdapter) systemPresetFallback(name string) *domain.Preset {
	if a.systemRegistry == nil {
		return nil
	}
	body, ok := a.systemRegistry.Get(name)
	if !ok {
		return nil
	}
	now := time.Now().UTC()
	// Real Agent2-visible description; generic string only if the name is
	// somehow missing from the map (taxonomy guard test prevents drift).
	description := presets.SystemPresetDescriptions[name]
	if description == "" {
		description = "system-default preset"
	}
	return &domain.Preset{
		ID:               "system:" + name,
		TenantID:         "system",
		Name:             name,
		Category:         "system",
		EntityType:       "product",
		Description:      description,
		DefaultReplicate: a.systemRegistry.DefaultReplicate(name),
		Version:          0,
		Status:           domain.PresetStatusPublished,
		DocumentJSON:     json.RawMessage(append([]byte(nil), body...)),
		PublishedAt:      &now,
		CreatedAt:        now,
		UpdatedAt:        now,
		IsSystem:         true,
	}
}

func (a *PresetAdapter) ListPublishedPresets(ctx context.Context, tenantSlugOrID string) ([]domain.Preset, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		end := sc.Start("postgres.ListPublishedPresets")
		defer end()
	}
	tenantID, err := resolveTenantID(ctx, a.client, tenantSlugOrID)
	if err != nil {
		return nil, err
	}
	// One row per preset: latest published version. DISTINCT ON drives the
	// dedup; ORDER BY p.id forces the index path; we re-sort by name in Go.
	rows, err := a.client.pool.Query(ctx, `
        SELECT DISTINCT ON (p.id)
            p.id::text, p.tenant_id::text, p.name, p.category, p.entity_type,
            p.description, p.default_replicate,
            v.version, v.status, v.doc_json,
            v.published_at, p.created_at, p.updated_at
        FROM v5_presets p
        JOIN v5_preset_versions v ON v.preset_id = p.id
        WHERE p.tenant_id = $1::uuid
          AND v.status = 'published'
        ORDER BY p.id, v.version DESC
    `, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list presets: %w", err)
	}
	defer rows.Close()

	var out []domain.Preset
	dbHave := map[string]bool{}
	for rows.Next() {
		preset, err := scanPreset(rows)
		if err != nil {
			return nil, err
		}
		dbHave[preset.Name] = true
		out = append(out, *preset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter presets: %w", err)
	}
	// Union with system registry — DB rows already win because we skip
	// any registry name the DB returned.
	if a.systemRegistry != nil {
		for _, name := range a.systemRegistry.List() {
			if dbHave[name] {
				continue
			}
			if sys := a.systemPresetFallback(name); sys != nil {
				out = append(out, *sys)
			}
		}
	}
	sortPresetsByName(out)
	return out, nil
}

func scanPreset(row rowScanner) (*domain.Preset, error) {
	var (
		p           domain.Preset
		docJSON     []byte
		publishedAt *time.Time
	)
	err := row.Scan(
		&p.ID, &p.TenantID, &p.Name, &p.Category, &p.EntityType,
		&p.Description, &p.DefaultReplicate,
		&p.Version, &p.Status, &docJSON,
		&publishedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(docJSON) > 0 {
		// Validate it parses as JSON before storing — surfaces shape bugs
		// at adapter boundary instead of way upstream in the engine layer.
		if !json.Valid(docJSON) {
			return nil, fmt.Errorf("doc_json is not valid JSON")
		}
		// Copy to detach from pgx-owned buffer.
		p.DocumentJSON = json.RawMessage(append([]byte(nil), docJSON...))
	}
	p.PublishedAt = publishedAt
	return &p, nil
}

// sortPresetsByName — tiny insertion sort. List is small (<50 per tenant),
// avoiding a sort import for one call site.
func sortPresetsByName(s []domain.Preset) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1].Name > s[j].Name; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
