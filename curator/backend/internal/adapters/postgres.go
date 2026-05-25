// Package adapters — single-file Postgres adapter for curator. Direct SQL on
// catalog.* tables (no admin/internal import — curator is a separate Go module).
package adapters

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"keepstar-curator/internal/domain"
)

type Client struct {
	Pool *pgxpool.Pool
}

func NewClient(ctx context.Context, dsn string) (*Client, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	// Tuned for Neon serverless autosuspend (default 5 min). MinConns=0 + short
	// idle timeout lets the pool collapse to zero on quiet periods so Neon can
	// suspend the compute. HealthCheckPeriod is intentionally long so the pool
	// doesn't ping idle connections back into the warm path.
	config.MaxConns = 10
	config.MinConns = 0
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = time.Minute
	config.HealthCheckPeriod = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Client{Pool: pool}, nil
}

func (c *Client) Close() { c.Pool.Close() }

// RunMigrations creates curator schema (users + sessions). Idempotent.
func (c *Client) RunMigrations(ctx context.Context) error {
	migrations := []string{
		`CREATE SCHEMA IF NOT EXISTS curator;`,
		`CREATE TABLE IF NOT EXISTS curator.users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'curator',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE TABLE IF NOT EXISTS curator.sessions (
			token_hash TEXT PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES curator.users(id) ON DELETE CASCADE,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE INDEX IF NOT EXISTS idx_curator_sessions_user
			ON curator.sessions(user_id);`,
	}
	for i, m := range migrations {
		if _, err := c.Pool.Exec(ctx, m); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}
	return nil
}

// --- Users + sessions ---

var ErrInvalidCredentials = errors.New("invalid credentials")

func (c *Client) CreateUser(ctx context.Context, email, password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return "", err
	}
	var id string
	err = c.Pool.QueryRow(ctx, `
		INSERT INTO curator.users (email, password_hash) VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash
		RETURNING id`, strings.ToLower(email), string(hash)).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

func (c *Client) Login(ctx context.Context, email, password string) (token string, user domain.CuratorUser, err error) {
	var hash string
	err = c.Pool.QueryRow(ctx, `
		SELECT id, email, role, password_hash, created_at
		FROM curator.users WHERE email = $1`, strings.ToLower(email)).Scan(
		&user.ID, &user.Email, &user.Role, &hash, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.CuratorUser{}, ErrInvalidCredentials
	}
	if err != nil {
		return "", domain.CuratorUser{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", domain.CuratorUser{}, ErrInvalidCredentials
	}
	plain, hashed, err := generateSessionToken()
	if err != nil {
		return "", domain.CuratorUser{}, err
	}
	_, err = c.Pool.Exec(ctx, `
		INSERT INTO curator.sessions (token_hash, user_id, expires_at)
		VALUES ($1, $2, $3)`, hashed, user.ID, time.Now().Add(7*24*time.Hour))
	if err != nil {
		return "", domain.CuratorUser{}, fmt.Errorf("create session: %w", err)
	}
	return plain, user, nil
}

func (c *Client) Logout(ctx context.Context, plain string) error {
	hashed := hashToken(plain)
	_, err := c.Pool.Exec(ctx, `DELETE FROM curator.sessions WHERE token_hash = $1`, hashed)
	return err
}

func (c *Client) ResolveSession(ctx context.Context, plain string) (domain.CuratorUser, error) {
	hashed := hashToken(plain)
	var u domain.CuratorUser
	err := c.Pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.role, u.created_at
		FROM curator.sessions s
		JOIN curator.users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > NOW()`, hashed).Scan(
		&u.ID, &u.Email, &u.Role, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CuratorUser{}, ErrInvalidCredentials
	}
	return u, err
}

// --- Audit reads on catalog.* ---

func (c *Client) ListAudit(ctx context.Context, limit int) ([]domain.AuditEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := c.Pool.Query(ctx, `
		SELECT id, COALESCE(tenant_id::text, ''), actor_kind, COALESCE(actor_id, ''),
			entity_kind, entity_id, action,
			COALESCE(field_changes, 'null'::jsonb), COALESCE(aggregate_meta, 'null'::jsonb),
			created_at
		FROM catalog.audit_log
		ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AuditEntry
	for rows.Next() {
		var a domain.AuditEntry
		var fcJSON, agJSON []byte
		if err := rows.Scan(&a.ID, &a.TenantID, &a.ActorKind, &a.ActorID,
			&a.EntityKind, &a.EntityID, &a.Action, &fcJSON, &agJSON, &a.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(fcJSON, &a.FieldChanges)
		_ = json.Unmarshal(agJSON, &a.AggregateMeta)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (c *Client) WriteAudit(ctx context.Context, e domain.AuditEntry) error {
	fc, _ := json.Marshal(e.FieldChanges)
	ag, _ := json.Marshal(e.AggregateMeta)
	tenantArg := any(nil)
	if e.TenantID != "" {
		tenantArg = e.TenantID
	}
	_, err := c.Pool.Exec(ctx, `
		INSERT INTO catalog.audit_log
			(tenant_id, actor_kind, actor_id, entity_kind, entity_id,
				action, field_changes, aggregate_meta)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8)`,
		tenantArg, e.ActorKind, e.ActorID, e.EntityKind, e.EntityID,
		e.Action, fc, ag)
	return err
}

// --- Tenants & catalog browse (read-only) ---

// ListTenants returns operations-overview rows for /curator/tenants.
// Joins catalog.tenants with product counts, master-link coverage, integrations,
// and tenant_catalog_schema status. Single round-trip via subqueries.
func (c *Client) ListTenants(ctx context.Context) ([]domain.TenantSummary, error) {
	rows, err := c.Pool.Query(ctx, `
		SELECT
			t.id, t.slug, t.name, COALESCE(t.type, ''),
			COALESCE(p.products_count, 0),
			COALESCE(p.master_linked_count, 0),
			COALESCE(s.status, ''),
			COALESCE(s.artifact_version, 0),
			t.created_at
		FROM catalog.tenants t
		LEFT JOIN (
			SELECT tenant_id,
				COUNT(*) AS products_count,
				COUNT(*) FILTER (WHERE master_product_id IS NOT NULL OR master_variant_id IS NOT NULL) AS master_linked_count
			FROM catalog.products
			WHERE deleted_at IS NULL
			GROUP BY tenant_id
		) p ON p.tenant_id = t.id
		LEFT JOIN catalog.tenant_catalog_schema s ON s.tenant_id = t.id
		ORDER BY p.products_count DESC NULLS LAST, t.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenants []domain.TenantSummary
	tenantIDs := make([]string, 0)
	for rows.Next() {
		var t domain.TenantSummary
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.Type,
			&t.ProductsCount, &t.MasterLinkedCount,
			&t.SchemaStatus, &t.SchemaVersion, &t.CreatedAt); err != nil {
			return nil, err
		}
		if t.ProductsCount > 0 {
			t.MasterLinkCoverage = float64(t.MasterLinkedCount) / float64(t.ProductsCount)
		}
		t.Integrations = []domain.IntegrationSummary{}
		tenants = append(tenants, t)
		tenantIDs = append(tenantIDs, t.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(tenantIDs) == 0 {
		return tenants, nil
	}
	// Hydrate integrations in one query.
	integrationsByTenant, err := c.listIntegrationsForTenants(ctx, tenantIDs)
	if err != nil {
		return nil, err
	}
	for i := range tenants {
		if list, ok := integrationsByTenant[tenants[i].ID]; ok {
			tenants[i].Integrations = list
		}
	}
	return tenants, nil
}

func (c *Client) listIntegrationsForTenants(ctx context.Context, tenantIDs []string) (map[string][]domain.IntegrationSummary, error) {
	rows, err := c.Pool.Query(ctx, `
		SELECT tenant_id::text, kind, COALESCE(status, ''), COALESCE(display_name, ''),
			COALESCE(external_id, ''), last_sync_at
		FROM admin.tenant_integrations
		WHERE tenant_id = ANY($1::uuid[])
		ORDER BY tenant_id, created_at DESC`, tenantIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]domain.IntegrationSummary{}
	for rows.Next() {
		var tid string
		var s domain.IntegrationSummary
		var lastSync *time.Time
		if err := rows.Scan(&tid, &s.Kind, &s.Status, &s.DisplayName, &s.ExternalID, &lastSync); err != nil {
			return nil, err
		}
		s.LastSyncAt = lastSync
		out[tid] = append(out[tid], s)
	}
	return out, rows.Err()
}

// GetTenant returns a single tenant with the same shape as ListTenants rows.
func (c *Client) GetTenant(ctx context.Context, id string) (domain.TenantSummary, error) {
	var t domain.TenantSummary
	err := c.Pool.QueryRow(ctx, `
		SELECT
			t.id, t.slug, t.name, COALESCE(t.type, ''),
			COALESCE(p.products_count, 0),
			COALESCE(p.master_linked_count, 0),
			COALESCE(s.status, ''),
			COALESCE(s.artifact_version, 0),
			t.created_at
		FROM catalog.tenants t
		LEFT JOIN (
			SELECT tenant_id,
				COUNT(*) AS products_count,
				COUNT(*) FILTER (WHERE master_product_id IS NOT NULL OR master_variant_id IS NOT NULL) AS master_linked_count
			FROM catalog.products
			WHERE deleted_at IS NULL AND tenant_id = $1
			GROUP BY tenant_id
		) p ON p.tenant_id = t.id
		LEFT JOIN catalog.tenant_catalog_schema s ON s.tenant_id = t.id
		WHERE t.id = $1`, id).Scan(
		&t.ID, &t.Slug, &t.Name, &t.Type,
		&t.ProductsCount, &t.MasterLinkedCount,
		&t.SchemaStatus, &t.SchemaVersion, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, fmt.Errorf("tenant not found")
	}
	if err != nil {
		return t, err
	}
	if t.ProductsCount > 0 {
		t.MasterLinkCoverage = float64(t.MasterLinkedCount) / float64(t.ProductsCount)
	}
	t.Integrations = []domain.IntegrationSummary{}
	intMap, err := c.listIntegrationsForTenants(ctx, []string{id})
	if err != nil {
		return t, err
	}
	if list, ok := intMap[id]; ok {
		t.Integrations = list
	}
	return t, nil
}

// ListTenantProducts — read-only catalog browse for a single tenant.
// COALESCE name resolution mirrors what V4 chat sees (M6 read-path).
func (c *Client) ListTenantProducts(ctx context.Context, tenantID, search string, limit, offset int) ([]domain.TenantProductRow, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	args := []any{tenantID}
	whereSearch := ""
	if search != "" {
		args = append(args, "%"+strings.ToLower(search)+"%")
		whereSearch = `AND (LOWER(COALESCE(p.display_name, p.original_name, p.name)) LIKE $2
			OR LOWER(COALESCE(p.raw_attributes->>'sku', '')) LIKE $2
			OR LOWER(COALESCE(mp.brand, '')) LIKE $2)`
	}
	args = append(args, limit, offset)
	limitArg := fmt.Sprintf("$%d", len(args)-1)
	offsetArg := fmt.Sprintf("$%d", len(args))

	q := `
		SELECT
			p.id::text,
			COALESCE(p.display_name, p.original_name, p.name, ''),
			COALESCE(p.original_name, ''),
			COALESCE(p.raw_attributes->>'sku', ''),
			COALESCE(mp.brand, p.raw_attributes->>'vendor', ''),
			COALESCE(p.price, 0),
			COALESCE(p.currency, ''),
			COALESCE(p.stock_quantity, 0),
			COALESCE(
				p.media->0->>'url',
				CASE WHEN jsonb_typeof(mp.images->0) = 'string' THEN mp.images->>0 ELSE mp.images->0->>'url' END,
				CASE WHEN jsonb_typeof(p.images->0) = 'string' THEN p.images->>0 ELSE p.images->0->>'url' END,
				''),
			(p.master_variant_id IS NOT NULL OR p.master_product_id IS NOT NULL),
			COALESCE(p.master_variant_id::text, p.master_product_id::text, ''),
			COALESCE(p.source_system, '')
		FROM catalog.products p
		LEFT JOIN catalog.master_variants mv ON mv.id = p.master_variant_id
		LEFT JOIN catalog.master_products mp ON mp.id = COALESCE(p.master_product_id, mv.master_product_id)
		WHERE p.tenant_id = $1 AND p.deleted_at IS NULL ` + whereSearch + `
		ORDER BY p.updated_at DESC NULLS LAST, p.created_at DESC
		LIMIT ` + limitArg + ` OFFSET ` + offsetArg
	rows, err := c.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.TenantProductRow
	for rows.Next() {
		var r domain.TenantProductRow
		if err := rows.Scan(&r.ID, &r.Name, &r.OriginalName, &r.SKU, &r.Brand,
			&r.PriceCents, &r.Currency, &r.Stock, &r.ImageURL,
			&r.HasMasterLink, &r.MasterID, &r.SourceSystem); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	// Total count for pagination — separate query, same WHERE as above.
	countArgs := []any{tenantID}
	countWhereSearch := ""
	if search != "" {
		countArgs = append(countArgs, "%"+strings.ToLower(search)+"%")
		countWhereSearch = `AND (LOWER(COALESCE(p.display_name, p.original_name, p.name)) LIKE $2
			OR LOWER(COALESCE(p.raw_attributes->>'sku', '')) LIKE $2
			OR LOWER(COALESCE(mp.brand, '')) LIKE $2)`
	}
	var total int
	err = c.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM catalog.products p
		LEFT JOIN catalog.master_variants mv ON mv.id = p.master_variant_id
		LEFT JOIN catalog.master_products mp ON mp.id = COALESCE(p.master_product_id, mv.master_product_id)
		WHERE p.tenant_id = $1 AND p.deleted_at IS NULL `+countWhereSearch, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// GetTenantSchema reads catalog.tenant_catalog_schema (mapping artifact + status).
// Returns empty (status="") if no row — that's a tenant that hasn't been discovered yet.
func (c *Client) GetTenantSchema(ctx context.Context, tenantID string) (domain.TenantSchemaSummary, error) {
	var s domain.TenantSchemaSummary
	s.TenantID = tenantID
	var maJSON, vrJSON []byte
	var discoveredAt, validatedAt *time.Time
	err := c.Pool.QueryRow(ctx, `
		SELECT COALESCE(status, ''), COALESCE(artifact_version, 0),
			discovered_at, validated_at,
			COALESCE(mapping_artifact, 'null'::jsonb),
			COALESCE(validation_report, 'null'::jsonb)
		FROM catalog.tenant_catalog_schema
		WHERE tenant_id = $1`, tenantID).Scan(
		&s.Status, &s.ArtifactVersion, &discoveredAt, &validatedAt, &maJSON, &vrJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	s.DiscoveredAt = discoveredAt
	s.ValidatedAt = validatedAt
	_ = json.Unmarshal(maJSON, &s.MappingArtifact)
	_ = json.Unmarshal(vrJSON, &s.ValidationReport)
	return s, nil
}

// --- Master catalog browse ---

// ListMasterProducts — global master catalog browse with vertical filter and search.
// listingCount is computed via subquery, capped to active (deleted_at IS NULL) tenant listings.
func (c *Client) ListMasterProducts(ctx context.Context, vertical, search string, limit, offset int) ([]domain.MasterProductSummary, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	args := []any{}
	conds := []string{}
	if vertical != "" {
		args = append(args, vertical)
		conds = append(conds, fmt.Sprintf("mp.vertical = $%d", len(args)))
	}
	if search != "" {
		args = append(args, "%"+strings.ToLower(search)+"%")
		conds = append(conds, fmt.Sprintf("(LOWER(mp.name) LIKE $%d OR LOWER(COALESCE(mp.brand, '')) LIKE $%d)", len(args), len(args)))
	}
	whereClause := ""
	if len(conds) > 0 {
		whereClause = "WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, limit, offset)
	limitArg := fmt.Sprintf("$%d", len(args)-1)
	offsetArg := fmt.Sprintf("$%d", len(args))

	q := `
		SELECT
			mp.id::text, COALESCE(mp.name, ''), COALESCE(mp.brand, ''),
			COALESCE(mp.vertical, ''),
			(mp.embedding IS NOT NULL),
			(mp.skin_type IS NOT NULL OR mp.concern IS NOT NULL OR mp.key_ingredients IS NOT NULL),
			(mp.description IS NOT NULL AND mp.description <> ''),
			COALESCE(CASE WHEN jsonb_typeof(mp.images->0) = 'string' THEN mp.images->>0 ELSE mp.images->0->>'url' END, ''),
			COALESCE(t.slug, ''),
			mp.created_at,
			COALESCE(lc.cnt, 0)
		FROM catalog.master_products mp
		LEFT JOIN catalog.tenants t ON t.id = mp.owner_tenant_id
		LEFT JOIN (
			SELECT COALESCE(p.master_product_id, mv.master_product_id) AS mpid, COUNT(*) AS cnt
			FROM catalog.products p
			LEFT JOIN catalog.master_variants mv ON mv.id = p.master_variant_id
			WHERE p.deleted_at IS NULL
			GROUP BY COALESCE(p.master_product_id, mv.master_product_id)
		) lc ON lc.mpid = mp.id
		` + whereClause + `
		ORDER BY mp.updated_at DESC NULLS LAST, mp.created_at DESC
		LIMIT ` + limitArg + ` OFFSET ` + offsetArg
	rows, err := c.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.MasterProductSummary
	for rows.Next() {
		var s domain.MasterProductSummary
		if err := rows.Scan(&s.ID, &s.Name, &s.Brand, &s.Vertical,
			&s.HasEmbedding, &s.HasPIM, &s.HasDescription, &s.ImageURL,
			&s.OwnerTenantSlug, &s.CreatedAt, &s.ListingCount); err != nil {
			return nil, 0, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	// Count for pagination.
	countArgs := args[:len(args)-2] // strip limit/offset
	countSQL := `SELECT COUNT(*) FROM catalog.master_products mp ` + whereClause
	var total int
	if err := c.Pool.QueryRow(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// GetMasterProduct returns a master_product detail with variants and linked listings.
// PIM fields are flat (skin_type, concern, key_ingredients, etc.) — copy the array
// columns into a generic map so the curator UI can render them dynamically.
func (c *Client) GetMasterProduct(ctx context.Context, id string) (domain.MasterProductDetail, error) {
	var d domain.MasterProductDetail
	d.Variants = []domain.MasterVariantSummary{}
	d.LinkedListings = []domain.TenantProductRow{}
	d.PIMFields = map[string]interface{}{}

	var skinType, concern, keyIngredients, targetArea, freeFrom, benefits []string
	var ownerSlug string
	err := c.Pool.QueryRow(ctx, `
		SELECT
			mp.id::text, COALESCE(mp.name, ''), COALESCE(mp.brand, ''),
			COALESCE(mp.vertical, ''),
			(mp.embedding IS NOT NULL),
			(mp.skin_type IS NOT NULL OR mp.concern IS NOT NULL OR mp.key_ingredients IS NOT NULL),
			(mp.description IS NOT NULL AND mp.description <> ''),
			COALESCE(mp.description, ''),
			COALESCE(CASE WHEN jsonb_typeof(mp.images->0) = 'string' THEN mp.images->>0 ELSE mp.images->0->>'url' END, ''),
			COALESCE(t.slug, ''),
			mp.created_at,
			COALESCE(mp.skin_type, '{}'::text[]),
			COALESCE(mp.concern, '{}'::text[]),
			COALESCE(mp.key_ingredients, '{}'::text[]),
			COALESCE(mp.target_area, '{}'::text[]),
			COALESCE(mp.free_from, '{}'::text[]),
			COALESCE(mp.benefits, '{}'::text[])
		FROM catalog.master_products mp
		LEFT JOIN catalog.tenants t ON t.id = mp.owner_tenant_id
		WHERE mp.id = $1`, id).Scan(
		&d.ID, &d.Name, &d.Brand, &d.Vertical,
		&d.HasEmbedding, &d.HasPIM, &d.HasDescription,
		&d.Description, &d.ImageURL, &ownerSlug, &d.CreatedAt,
		&skinType, &concern, &keyIngredients, &targetArea, &freeFrom, &benefits)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, fmt.Errorf("master product not found")
	}
	if err != nil {
		return d, err
	}
	d.OwnerTenantSlug = ownerSlug
	addPIM := func(k string, v []string) {
		if len(v) > 0 {
			d.PIMFields[k] = v
		}
	}
	addPIM("skin_type", skinType)
	addPIM("concern", concern)
	addPIM("key_ingredients", keyIngredients)
	addPIM("target_area", targetArea)
	addPIM("free_from", freeFrom)
	addPIM("benefits", benefits)

	// Variants.
	vrows, err := c.Pool.Query(ctx, `
		SELECT id::text, COALESCE(sku, ''),
			COALESCE(gtins, '{}'::text[]),
			COALESCE(color, ''), COALESCE(size, ''), COALESCE(material, ''),
			COALESCE(weight_g, 0), COALESCE(volume_ml, 0),
			COALESCE(image_url, '')
		FROM catalog.master_variants
		WHERE master_product_id = $1
		ORDER BY created_at`, id)
	if err != nil {
		return d, err
	}
	defer vrows.Close()
	for vrows.Next() {
		var v domain.MasterVariantSummary
		if err := vrows.Scan(&v.ID, &v.SKU, &v.GTINs, &v.Color, &v.Size, &v.Material,
			&v.WeightG, &v.VolumeML, &v.ImageURL); err != nil {
			return d, err
		}
		d.Variants = append(d.Variants, v)
	}
	if err := vrows.Err(); err != nil {
		return d, err
	}

	// Linked listings (any tenant linking to this master_product directly OR via variant).
	lrows, err := c.Pool.Query(ctx, `
		SELECT
			p.id::text,
			COALESCE(p.display_name, p.original_name, p.name, ''),
			COALESCE(p.original_name, ''),
			COALESCE(p.raw_attributes->>'sku', ''),
			COALESCE(p.raw_attributes->>'vendor', ''),
			COALESCE(p.price, 0),
			COALESCE(p.currency, ''),
			COALESCE(p.stock_quantity, 0),
			COALESCE(p.media->0->>'url', CASE WHEN jsonb_typeof(p.images->0) = 'string' THEN p.images->>0 ELSE p.images->0->>'url' END, ''),
			TRUE,
			COALESCE(p.master_variant_id::text, p.master_product_id::text, ''),
			COALESCE(p.source_system, '')
		FROM catalog.products p
		LEFT JOIN catalog.master_variants mv ON mv.id = p.master_variant_id
		WHERE (p.master_product_id = $1 OR mv.master_product_id = $1) AND p.deleted_at IS NULL
		ORDER BY p.updated_at DESC NULLS LAST
		LIMIT 100`, id)
	if err != nil {
		return d, err
	}
	defer lrows.Close()
	for lrows.Next() {
		var r domain.TenantProductRow
		if err := lrows.Scan(&r.ID, &r.Name, &r.OriginalName, &r.SKU, &r.Brand,
			&r.PriceCents, &r.Currency, &r.Stock, &r.ImageURL,
			&r.HasMasterLink, &r.MasterID, &r.SourceSystem); err != nil {
			return d, err
		}
		d.LinkedListings = append(d.LinkedListings, r)
	}
	return d, lrows.Err()
}

// --- helpers ---

var allowedColumnTypes = map[string]string{
	"text":       "TEXT",
	"text_array": "TEXT[]",
	"integer":    "INTEGER",
	"numeric":    "NUMERIC",
	"boolean":    "BOOLEAN",
}

// validIdent allows snake_case lowercase ascii — strict to keep ALTER TABLE
// safe against SQL injection through the candidate's free-text key.
func validIdent(s string) bool {
	if s == "" || len(s) > 60 {
		return false
	}
	for i, r := range s {
		if r == '_' {
			continue
		}
		if r >= '0' && r <= '9' {
			if i == 0 {
				return false
			}
			continue
		}
		if r >= 'a' && r <= 'z' {
			continue
		}
		return false
	}
	return true
}

func generateSessionToken() (plain, hashed string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return
	}
	plain = "cs_" + base64.RawURLEncoding.EncodeToString(buf)
	hashed = hashToken(plain)
	return
}

func hashToken(plain string) string {
	// Sessions are short-lived high-entropy tokens; deterministic SHA-256 lets
	// us look them up by primary key on every request. Bcrypt would be wrong
	// here because each call salts differently — Resolve couldn't find them.
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
