package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/ports"
)

// CanvasAdapter persists KeepstarCanvas presets, components, tokens and the
// per-tenant design-context version counter. All queries are tenant-scoped;
// the usecase layer is responsible for passing the tenant ID from the JWT.
type CanvasAdapter struct {
	client *Client
}

func NewCanvasAdapter(client *Client) *CanvasAdapter {
	return &CanvasAdapter{client: client}
}

// compile-time check: CanvasAdapter must satisfy ports.CanvasPort.
var _ ports.CanvasPort = (*CanvasAdapter)(nil)

func (a *CanvasAdapter) pool() *pgxpool.Pool { return a.client.pool }

var errPresetNotFound = errors.New("canvas: preset not found")
var errComponentNotFound = errors.New("canvas: component not found")

// ErrCanvasNotFound is returned when the requested canvas entity does not
// exist (or belongs to another tenant).
var ErrCanvasNotFound = errors.New("canvas: not found")

// -------------------- presets --------------------

func (a *CanvasAdapter) ListPresets(ctx context.Context, tenantID string) ([]domain.TenantPreset, error) {
	rows, err := a.pool().Query(ctx, `
		SELECT p.id, p.tenant_id, p.name, p.category, p.default_replicate,
		       p.entity_type, p.description, COALESCE(p.latest_version_id::text, ''),
		       p.created_at, p.updated_at,
		       v.id, v.version, v.status, v.ops_json, v.published_at, v.created_at
		FROM admin.tenant_presets p
		LEFT JOIN admin.tenant_preset_versions v ON v.id = p.latest_version_id
		WHERE p.tenant_id = $1
		ORDER BY p.updated_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list presets: %w", err)
	}
	defer rows.Close()

	out := make([]domain.TenantPreset, 0, 16)
	for rows.Next() {
		var p domain.TenantPreset
		var vID, vStatus *string
		var vVersion *int
		var vOps []byte
		var vPublishedAt *time.Time
		var vCreatedAt *time.Time
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.Name, &p.Category, &p.DefaultReplicate,
			&p.EntityType, &p.Description, &p.LatestVersionID,
			&p.CreatedAt, &p.UpdatedAt,
			&vID, &vVersion, &vStatus, &vOps, &vPublishedAt, &vCreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan preset: %w", err)
		}
		if vID != nil {
			ver := &domain.PresetVersion{
				ID:          *vID,
				PresetID:    p.ID,
				Version:     *vVersion,
				Status:      domain.PresetStatus(*vStatus),
				PublishedAt: vPublishedAt,
			}
			if vCreatedAt != nil {
				ver.CreatedAt = *vCreatedAt
			}
			if len(vOps) > 0 {
				ver.OpsJSON = json.RawMessage(vOps)
			} else {
				ver.OpsJSON = json.RawMessage("[]")
			}
			p.LatestVersion = ver
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (a *CanvasAdapter) GetPreset(ctx context.Context, tenantID, presetID string) (*domain.TenantPreset, error) {
	return a.getPresetBy(ctx, tenantID, "p.id = $2", presetID)
}

func (a *CanvasAdapter) GetPresetByName(ctx context.Context, tenantID, name string) (*domain.TenantPreset, error) {
	return a.getPresetBy(ctx, tenantID, "p.name = $2", name)
}

func (a *CanvasAdapter) getPresetBy(ctx context.Context, tenantID, whereClause string, arg any) (*domain.TenantPreset, error) {
	query := fmt.Sprintf(`
		SELECT p.id, p.tenant_id, p.name, p.category, p.default_replicate,
		       p.entity_type, p.description, COALESCE(p.latest_version_id::text, ''),
		       p.created_at, p.updated_at
		FROM admin.tenant_presets p
		WHERE p.tenant_id = $1 AND %s`, whereClause)

	var p domain.TenantPreset
	err := a.pool().QueryRow(ctx, query, tenantID, arg).Scan(
		&p.ID, &p.TenantID, &p.Name, &p.Category, &p.DefaultReplicate,
		&p.EntityType, &p.Description, &p.LatestVersionID,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCanvasNotFound
		}
		return nil, fmt.Errorf("get preset: %w", err)
	}
	if p.LatestVersionID != "" {
		ver, err := a.loadPresetVersion(ctx, p.LatestVersionID)
		if err != nil {
			return nil, err
		}
		p.LatestVersion = ver
	}
	return &p, nil
}

func (a *CanvasAdapter) loadPresetVersion(ctx context.Context, versionID string) (*domain.PresetVersion, error) {
	var v domain.PresetVersion
	var authorID *string
	var ops []byte
	err := a.pool().QueryRow(ctx, `
		SELECT id, preset_id, version, status, ops_json, author_user_id, published_at, created_at
		FROM admin.tenant_preset_versions WHERE id = $1`, versionID).Scan(
		&v.ID, &v.PresetID, &v.Version, &v.Status, &ops, &authorID, &v.PublishedAt, &v.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("load preset version: %w", err)
	}
	if authorID != nil {
		v.AuthorUserID = *authorID
	}
	if len(ops) == 0 {
		ops = []byte("[]")
	}
	v.OpsJSON = json.RawMessage(ops)
	return &v, nil
}

func (a *CanvasAdapter) CreatePreset(ctx context.Context, tenantID string, in domain.PresetCreateInput, authorUserID string) (*domain.TenantPreset, error) {
	replicate := true
	if in.DefaultReplicate != nil {
		replicate = *in.DefaultReplicate
	}
	category := in.Category
	if category == "" {
		category = "product"
	}
	entityType := in.EntityType
	if entityType == "" {
		entityType = "product"
	}
	ops := normalizeOps(in.Ops)

	tx, err := a.pool().Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("create preset begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var presetID string
	err = tx.QueryRow(ctx, `
		INSERT INTO admin.tenant_presets
			(tenant_id, name, category, default_replicate, entity_type, description)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		tenantID, in.Name, category, replicate, entityType, in.Description,
	).Scan(&presetID)
	if err != nil {
		return nil, fmt.Errorf("insert preset: %w", err)
	}

	var versionID string
	err = tx.QueryRow(ctx, `
		INSERT INTO admin.tenant_preset_versions
			(preset_id, version, status, ops_json, author_user_id)
		VALUES ($1, 1, 'draft', $2, $3)
		RETURNING id`,
		presetID, ops, nullIfEmpty(authorUserID),
	).Scan(&versionID)
	if err != nil {
		return nil, fmt.Errorf("insert initial preset version: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE admin.tenant_presets SET latest_version_id = $1, updated_at = NOW()
		WHERE id = $2`, versionID, presetID); err != nil {
		return nil, fmt.Errorf("link latest version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create preset: %w", err)
	}
	return a.GetPreset(ctx, tenantID, presetID)
}

func (a *CanvasAdapter) UpdatePresetMetadata(ctx context.Context, tenantID, presetID string, in domain.PresetUpdateInput) (*domain.TenantPreset, error) {
	// Metadata-only update (name/description/etc.). Ops must go through
	// SaveDraftOps so version forking is handled correctly.
	set := ""
	args := []any{tenantID, presetID}
	idx := 3
	appendSet := func(col string, val any) {
		if set != "" {
			set += ", "
		}
		set += fmt.Sprintf("%s = $%d", col, idx)
		args = append(args, val)
		idx++
	}
	if in.Name != nil {
		appendSet("name", *in.Name)
	}
	if in.Category != nil {
		appendSet("category", *in.Category)
	}
	if in.EntityType != nil {
		appendSet("entity_type", *in.EntityType)
	}
	if in.Description != nil {
		appendSet("description", *in.Description)
	}
	if in.DefaultReplicate != nil {
		appendSet("default_replicate", *in.DefaultReplicate)
	}
	if set == "" {
		return a.GetPreset(ctx, tenantID, presetID)
	}
	query := fmt.Sprintf(`
		UPDATE admin.tenant_presets SET %s, updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2`, set)
	ct, err := a.pool().Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update preset metadata: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return nil, ErrCanvasNotFound
	}
	return a.GetPreset(ctx, tenantID, presetID)
}

// SaveDraftOps updates the preset's working draft. If the latest version is
// already draft, ops are overwritten on that row. If the latest version is
// published, a new draft row is forked at (max_version + 1).
func (a *CanvasAdapter) SaveDraftOps(ctx context.Context, tenantID, presetID string, ops []byte, authorUserID string) (*domain.PresetVersion, error) {
	tx, err := a.pool().Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("save draft begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Resolve the preset + latest version under row lock so concurrent edits
	// don't race on the fork decision.
	var latestID *string
	var maxVersion int
	err = tx.QueryRow(ctx, `
		SELECT latest_version_id::text
		FROM admin.tenant_presets
		WHERE tenant_id = $1 AND id = $2
		FOR UPDATE`, tenantID, presetID).Scan(&latestID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCanvasNotFound
		}
		return nil, fmt.Errorf("lock preset: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) FROM admin.tenant_preset_versions WHERE preset_id = $1`,
		presetID).Scan(&maxVersion); err != nil {
		return nil, fmt.Errorf("query max version: %w", err)
	}

	ops = normalizeOps(ops)

	var latestStatus string
	var versionID string
	if latestID != nil && *latestID != "" {
		if err := tx.QueryRow(ctx, `
			SELECT status FROM admin.tenant_preset_versions WHERE id = $1`,
			*latestID).Scan(&latestStatus); err != nil {
			return nil, fmt.Errorf("query latest status: %w", err)
		}
	}

	if latestStatus == string(domain.PresetStatusDraft) && latestID != nil {
		// Mutate in place.
		if _, err := tx.Exec(ctx, `
			UPDATE admin.tenant_preset_versions
			SET ops_json = $1, author_user_id = COALESCE($2, author_user_id)
			WHERE id = $3`,
			ops, nullIfEmpty(authorUserID), *latestID,
		); err != nil {
			return nil, fmt.Errorf("update draft ops: %w", err)
		}
		versionID = *latestID
	} else {
		// Fork new draft.
		if err := tx.QueryRow(ctx, `
			INSERT INTO admin.tenant_preset_versions
				(preset_id, version, status, ops_json, author_user_id)
			VALUES ($1, $2, 'draft', $3, $4)
			RETURNING id`,
			presetID, maxVersion+1, ops, nullIfEmpty(authorUserID),
		).Scan(&versionID); err != nil {
			return nil, fmt.Errorf("fork draft: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE admin.tenant_presets SET latest_version_id = $1, updated_at = NOW()
		WHERE id = $2`, versionID, presetID); err != nil {
		return nil, fmt.Errorf("update latest version pointer: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit save draft: %w", err)
	}
	return a.loadPresetVersion(ctx, versionID)
}

// PublishPreset flips the latest version to published if it's currently a
// draft. Published versions are immutable — any subsequent edit will fork a
// fresh draft via SaveDraftOps.
func (a *CanvasAdapter) PublishPreset(ctx context.Context, tenantID, presetID string) (*domain.PresetVersion, error) {
	tx, err := a.pool().Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("publish begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var latestID *string
	err = tx.QueryRow(ctx, `
		SELECT latest_version_id::text FROM admin.tenant_presets
		WHERE tenant_id = $1 AND id = $2 FOR UPDATE`,
		tenantID, presetID).Scan(&latestID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCanvasNotFound
		}
		return nil, fmt.Errorf("lock preset for publish: %w", err)
	}
	if latestID == nil || *latestID == "" {
		return nil, fmt.Errorf("publish: preset has no version")
	}

	if _, err := tx.Exec(ctx, `
		UPDATE admin.tenant_preset_versions
		SET status = 'published', published_at = NOW()
		WHERE id = $1 AND status = 'draft'`, *latestID); err != nil {
		return nil, fmt.Errorf("update version status: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE admin.tenant_presets SET updated_at = NOW() WHERE id = $1`,
		presetID); err != nil {
		return nil, fmt.Errorf("touch preset after publish: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit publish: %w", err)
	}
	return a.loadPresetVersion(ctx, *latestID)
}

// ForkPreset creates a new draft row copying ops from the current latest
// version, even if the latest is already a draft. Useful for "edit published"
// flows the UI surfaces as a distinct affordance.
func (a *CanvasAdapter) ForkPreset(ctx context.Context, tenantID, presetID, authorUserID string) (*domain.PresetVersion, error) {
	tx, err := a.pool().Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("fork begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var latestID *string
	err = tx.QueryRow(ctx, `
		SELECT latest_version_id::text FROM admin.tenant_presets
		WHERE tenant_id = $1 AND id = $2 FOR UPDATE`,
		tenantID, presetID).Scan(&latestID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCanvasNotFound
		}
		return nil, fmt.Errorf("lock preset for fork: %w", err)
	}
	if latestID == nil || *latestID == "" {
		return nil, fmt.Errorf("fork: preset has no version")
	}

	var ops []byte
	if err := tx.QueryRow(ctx, `
		SELECT ops_json FROM admin.tenant_preset_versions WHERE id = $1`,
		*latestID).Scan(&ops); err != nil {
		return nil, fmt.Errorf("load ops for fork: %w", err)
	}

	var maxVersion int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) FROM admin.tenant_preset_versions WHERE preset_id = $1`,
		presetID).Scan(&maxVersion); err != nil {
		return nil, fmt.Errorf("query max version: %w", err)
	}

	var newID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO admin.tenant_preset_versions
			(preset_id, version, status, ops_json, author_user_id)
		VALUES ($1, $2, 'draft', $3, $4)
		RETURNING id`,
		presetID, maxVersion+1, normalizeOps(ops), nullIfEmpty(authorUserID),
	).Scan(&newID); err != nil {
		return nil, fmt.Errorf("insert forked draft: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE admin.tenant_presets SET latest_version_id = $1, updated_at = NOW()
		WHERE id = $2`, newID, presetID); err != nil {
		return nil, fmt.Errorf("update latest after fork: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit fork: %w", err)
	}
	return a.loadPresetVersion(ctx, newID)
}

func (a *CanvasAdapter) DeletePreset(ctx context.Context, tenantID, presetID string) error {
	ct, err := a.pool().Exec(ctx, `
		DELETE FROM admin.tenant_presets WHERE tenant_id = $1 AND id = $2`,
		tenantID, presetID)
	if err != nil {
		return fmt.Errorf("delete preset: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrCanvasNotFound
	}
	return nil
}

// GetLatestPublishedOps returns the newest *published* version's ops blob for
// a tenant + preset name. Used by the V4 chat backend's TenantPresetLoader —
// global registry is the fallback when this returns found=false.
func (a *CanvasAdapter) GetLatestPublishedOps(ctx context.Context, tenantID, name string) ([]byte, bool, error) {
	var ops []byte
	err := a.pool().QueryRow(ctx, `
		SELECT v.ops_json
		FROM admin.tenant_presets p
		JOIN admin.tenant_preset_versions v ON v.preset_id = p.id
		WHERE p.tenant_id = $1 AND p.name = $2 AND v.status = 'published'
		ORDER BY v.version DESC LIMIT 1`, tenantID, name).Scan(&ops)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("query published ops: %w", err)
	}
	return ops, true, nil
}

// -------------------- components --------------------

func (a *CanvasAdapter) ListComponents(ctx context.Context, tenantID string) ([]domain.TenantComponent, error) {
	rows, err := a.pool().Query(ctx, `
		SELECT c.id, c.tenant_id, c.name, c.category, c.description,
		       COALESCE(c.latest_version_id::text, ''),
		       c.created_at, c.updated_at
		FROM admin.tenant_components c
		WHERE c.tenant_id = $1
		ORDER BY c.updated_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list components: %w", err)
	}
	defer rows.Close()

	out := make([]domain.TenantComponent, 0, 16)
	for rows.Next() {
		var c domain.TenantComponent
		if err := rows.Scan(&c.ID, &c.TenantID, &c.Name, &c.Category, &c.Description,
			&c.LatestVersionID, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan component: %w", err)
		}
		if c.LatestVersionID != "" {
			if ver, err := a.loadComponentVersion(ctx, c.LatestVersionID); err == nil {
				c.LatestVersion = ver
			}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (a *CanvasAdapter) GetComponent(ctx context.Context, tenantID, componentID string) (*domain.TenantComponent, error) {
	var c domain.TenantComponent
	err := a.pool().QueryRow(ctx, `
		SELECT id, tenant_id, name, category, description,
		       COALESCE(latest_version_id::text, ''),
		       created_at, updated_at
		FROM admin.tenant_components
		WHERE tenant_id = $1 AND id = $2`,
		tenantID, componentID).Scan(
		&c.ID, &c.TenantID, &c.Name, &c.Category, &c.Description,
		&c.LatestVersionID, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCanvasNotFound
		}
		return nil, fmt.Errorf("get component: %w", err)
	}
	if c.LatestVersionID != "" {
		ver, err := a.loadComponentVersion(ctx, c.LatestVersionID)
		if err != nil {
			return nil, err
		}
		c.LatestVersion = ver
	}
	return &c, nil
}

func (a *CanvasAdapter) loadComponentVersion(ctx context.Context, versionID string) (*domain.ComponentVersion, error) {
	var v domain.ComponentVersion
	var authorID *string
	var ops []byte
	err := a.pool().QueryRow(ctx, `
		SELECT id, component_id, version, status, ops_json, author_user_id, published_at, created_at
		FROM admin.tenant_component_versions WHERE id = $1`, versionID).Scan(
		&v.ID, &v.ComponentID, &v.Version, &v.Status, &ops, &authorID, &v.PublishedAt, &v.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("load component version: %w", err)
	}
	if authorID != nil {
		v.AuthorUserID = *authorID
	}
	if len(ops) == 0 {
		ops = []byte("[]")
	}
	v.OpsJSON = json.RawMessage(ops)
	return &v, nil
}

func (a *CanvasAdapter) CreateComponent(ctx context.Context, tenantID string, in domain.ComponentCreateInput, authorUserID string) (*domain.TenantComponent, error) {
	category := in.Category
	if category == "" {
		category = "atom"
	}
	ops := normalizeOps(in.Ops)

	tx, err := a.pool().Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("create component begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var componentID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO admin.tenant_components
			(tenant_id, name, category, description)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		tenantID, in.Name, category, in.Description,
	).Scan(&componentID); err != nil {
		return nil, fmt.Errorf("insert component: %w", err)
	}

	var versionID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO admin.tenant_component_versions
			(component_id, version, status, ops_json, author_user_id)
		VALUES ($1, 1, 'draft', $2, $3)
		RETURNING id`,
		componentID, ops, nullIfEmpty(authorUserID),
	).Scan(&versionID); err != nil {
		return nil, fmt.Errorf("insert initial component version: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE admin.tenant_components SET latest_version_id = $1, updated_at = NOW()
		WHERE id = $2`, versionID, componentID); err != nil {
		return nil, fmt.Errorf("link component latest: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create component: %w", err)
	}
	return a.GetComponent(ctx, tenantID, componentID)
}

func (a *CanvasAdapter) UpdateComponentMetadata(ctx context.Context, tenantID, componentID string, in domain.ComponentUpdateInput) (*domain.TenantComponent, error) {
	set := ""
	args := []any{tenantID, componentID}
	idx := 3
	appendSet := func(col string, val any) {
		if set != "" {
			set += ", "
		}
		set += fmt.Sprintf("%s = $%d", col, idx)
		args = append(args, val)
		idx++
	}
	if in.Name != nil {
		appendSet("name", *in.Name)
	}
	if in.Category != nil {
		appendSet("category", *in.Category)
	}
	if in.Description != nil {
		appendSet("description", *in.Description)
	}
	if set == "" {
		return a.GetComponent(ctx, tenantID, componentID)
	}
	query := fmt.Sprintf(`
		UPDATE admin.tenant_components SET %s, updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2`, set)
	ct, err := a.pool().Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update component metadata: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return nil, ErrCanvasNotFound
	}
	return a.GetComponent(ctx, tenantID, componentID)
}

func (a *CanvasAdapter) SaveComponentDraftOps(ctx context.Context, tenantID, componentID string, ops []byte, authorUserID string) (*domain.ComponentVersion, error) {
	tx, err := a.pool().Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("save component draft begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var latestID *string
	err = tx.QueryRow(ctx, `
		SELECT latest_version_id::text
		FROM admin.tenant_components
		WHERE tenant_id = $1 AND id = $2
		FOR UPDATE`, tenantID, componentID).Scan(&latestID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCanvasNotFound
		}
		return nil, fmt.Errorf("lock component: %w", err)
	}

	var maxVersion int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) FROM admin.tenant_component_versions WHERE component_id = $1`,
		componentID).Scan(&maxVersion); err != nil {
		return nil, fmt.Errorf("query max component version: %w", err)
	}

	ops = normalizeOps(ops)

	var latestStatus string
	if latestID != nil && *latestID != "" {
		if err := tx.QueryRow(ctx, `
			SELECT status FROM admin.tenant_component_versions WHERE id = $1`,
			*latestID).Scan(&latestStatus); err != nil {
			return nil, fmt.Errorf("query component latest status: %w", err)
		}
	}

	var versionID string
	if latestStatus == string(domain.PresetStatusDraft) && latestID != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE admin.tenant_component_versions
			SET ops_json = $1, author_user_id = COALESCE($2, author_user_id)
			WHERE id = $3`, ops, nullIfEmpty(authorUserID), *latestID); err != nil {
			return nil, fmt.Errorf("update component draft: %w", err)
		}
		versionID = *latestID
	} else {
		if err := tx.QueryRow(ctx, `
			INSERT INTO admin.tenant_component_versions
				(component_id, version, status, ops_json, author_user_id)
			VALUES ($1, $2, 'draft', $3, $4)
			RETURNING id`,
			componentID, maxVersion+1, ops, nullIfEmpty(authorUserID),
		).Scan(&versionID); err != nil {
			return nil, fmt.Errorf("fork component draft: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE admin.tenant_components SET latest_version_id = $1, updated_at = NOW()
		WHERE id = $2`, versionID, componentID); err != nil {
		return nil, fmt.Errorf("update component latest pointer: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit component draft: %w", err)
	}
	return a.loadComponentVersion(ctx, versionID)
}

func (a *CanvasAdapter) PublishComponent(ctx context.Context, tenantID, componentID string) (*domain.ComponentVersion, error) {
	tx, err := a.pool().Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("publish component begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var latestID *string
	err = tx.QueryRow(ctx, `
		SELECT latest_version_id::text FROM admin.tenant_components
		WHERE tenant_id = $1 AND id = $2 FOR UPDATE`,
		tenantID, componentID).Scan(&latestID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCanvasNotFound
		}
		return nil, fmt.Errorf("lock component for publish: %w", err)
	}
	if latestID == nil || *latestID == "" {
		return nil, fmt.Errorf("publish component: no version")
	}

	if _, err := tx.Exec(ctx, `
		UPDATE admin.tenant_component_versions
		SET status = 'published', published_at = NOW()
		WHERE id = $1 AND status = 'draft'`, *latestID); err != nil {
		return nil, fmt.Errorf("update component version status: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE admin.tenant_components SET updated_at = NOW() WHERE id = $1`,
		componentID); err != nil {
		return nil, fmt.Errorf("touch component: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit publish component: %w", err)
	}
	return a.loadComponentVersion(ctx, *latestID)
}

func (a *CanvasAdapter) DeleteComponent(ctx context.Context, tenantID, componentID string) error {
	ct, err := a.pool().Exec(ctx, `
		DELETE FROM admin.tenant_components WHERE tenant_id = $1 AND id = $2`,
		tenantID, componentID)
	if err != nil {
		return fmt.Errorf("delete component: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrCanvasNotFound
	}
	return nil
}

// -------------------- design tokens --------------------

func (a *CanvasAdapter) ListDesignTokens(ctx context.Context, tenantID string) ([]domain.DesignToken, error) {
	rows, err := a.pool().Query(ctx, `
		SELECT id, tenant_id, category, name, value, theme_axis, theme_value, updated_at
		FROM admin.tenant_design_tokens
		WHERE tenant_id = $1
		ORDER BY category, name, theme_axis, theme_value`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	defer rows.Close()

	out := make([]domain.DesignToken, 0, 32)
	for rows.Next() {
		var t domain.DesignToken
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Category, &t.Name, &t.Value,
			&t.ThemeAxis, &t.ThemeValue, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (a *CanvasAdapter) UpsertDesignToken(ctx context.Context, tenantID string, in domain.DesignTokenUpsertInput) (*domain.DesignToken, error) {
	var t domain.DesignToken
	err := a.pool().QueryRow(ctx, `
		INSERT INTO admin.tenant_design_tokens
			(tenant_id, category, name, value, theme_axis, theme_value)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (tenant_id, category, name, theme_axis, theme_value)
		DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
		RETURNING id, tenant_id, category, name, value, theme_axis, theme_value, updated_at`,
		tenantID, in.Category, in.Name, in.Value, in.ThemeAxis, in.ThemeValue,
	).Scan(&t.ID, &t.TenantID, &t.Category, &t.Name, &t.Value, &t.ThemeAxis, &t.ThemeValue, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert token: %w", err)
	}
	return &t, nil
}

func (a *CanvasAdapter) DeleteDesignToken(ctx context.Context, tenantID, tokenID string) error {
	ct, err := a.pool().Exec(ctx, `
		DELETE FROM admin.tenant_design_tokens WHERE tenant_id = $1 AND id = $2`,
		tenantID, tokenID)
	if err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrCanvasNotFound
	}
	return nil
}

// -------------------- design context version counter --------------------

func (a *CanvasAdapter) BumpDesignContextVersion(ctx context.Context, tenantID string) (int64, error) {
	var version int64
	err := a.pool().QueryRow(ctx, `
		INSERT INTO admin.tenant_design_context_version (tenant_id, version)
		VALUES ($1, 1)
		ON CONFLICT (tenant_id)
		DO UPDATE SET version = admin.tenant_design_context_version.version + 1,
		              updated_at = NOW()
		RETURNING version`, tenantID).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("bump design context version: %w", err)
	}
	return version, nil
}

func (a *CanvasAdapter) GetDesignContextVersion(ctx context.Context, tenantID string) (int64, error) {
	var version int64
	err := a.pool().QueryRow(ctx, `
		SELECT version FROM admin.tenant_design_context_version WHERE tenant_id = $1`,
		tenantID).Scan(&version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("get design context version: %w", err)
	}
	return version, nil
}

// -------------------- helpers --------------------

func normalizeOps(ops []byte) []byte {
	if len(ops) == 0 {
		return []byte("[]")
	}
	// Ensure it's valid JSON; if caller passed a raw non-JSON blob, default.
	var probe any
	if err := json.Unmarshal(ops, &probe); err != nil {
		return []byte("[]")
	}
	return ops
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
