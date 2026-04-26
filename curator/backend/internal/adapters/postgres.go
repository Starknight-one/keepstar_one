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
	pool, err := pgxpool.New(ctx, dsn)
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

// --- Candidates / junk / audit reads on catalog.* ---

func (c *Client) ListAttributeCandidates(ctx context.Context, status string) ([]domain.AttributeCandidate, error) {
	if status == "" {
		status = "pending"
	}
	rows, err := c.Pool.Query(ctx, `
		SELECT id, key, vertical, seen_in_tenants,
			COALESCE(sample_values, '[]'::jsonb), COALESCE(proposed_type, ''),
			COALESCE(agent_meta, ''), status,
			COALESCE(promoted_to_column, ''), created_at
		FROM catalog.master_attribute_candidates
		WHERE status = $1
		ORDER BY seen_in_tenants DESC, created_at DESC`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AttributeCandidate
	for rows.Next() {
		var ac domain.AttributeCandidate
		var samplesJSON []byte
		if err := rows.Scan(&ac.ID, &ac.Key, &ac.Vertical, &ac.SeenInTenants,
			&samplesJSON, &ac.ProposedType, &ac.AgentMeta, &ac.Status,
			&ac.PromotedToColumn, &ac.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(samplesJSON, &ac.SampleValues)
		out = append(out, ac)
	}
	return out, rows.Err()
}

func (c *Client) ListCategoryCandidates(ctx context.Context, status string) ([]domain.CategoryCandidate, error) {
	if status == "" {
		status = "pending"
	}
	rows, err := c.Pool.Query(ctx, `
		SELECT id, name, COALESCE(proposed_parent, ''), seen_in_tenants,
			vertical, status, created_at
		FROM catalog.master_category_candidates
		WHERE status = $1
		ORDER BY seen_in_tenants DESC, created_at DESC`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.CategoryCandidate
	for rows.Next() {
		var cc domain.CategoryCandidate
		if err := rows.Scan(&cc.ID, &cc.Name, &cc.ProposedParent, &cc.SeenInTenants,
			&cc.Vertical, &cc.Status, &cc.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, cc)
	}
	return out, rows.Err()
}

func (c *Client) ListJunkCandidates(ctx context.Context, status string) ([]domain.JunkCandidate, error) {
	if status == "" {
		status = "pending"
	}
	rows, err := c.Pool.Query(ctx, `
		SELECT id, tenant_id, listing_id, COALESCE(detected_reason, '{}'::jsonb),
			classification, COALESCE(classified_by, ''), created_at
		FROM catalog.tenant_variant_candidates_junk
		WHERE classification = $1
		ORDER BY created_at DESC LIMIT 200`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.JunkCandidate
	for rows.Next() {
		var jc domain.JunkCandidate
		var reasonJSON []byte
		if err := rows.Scan(&jc.ID, &jc.TenantID, &jc.ListingID, &reasonJSON,
			&jc.Classification, &jc.ClassifiedBy, &jc.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(reasonJSON, &jc.DetectedReason)
		out = append(out, jc)
	}
	return out, rows.Err()
}

func (c *Client) ClassifyJunk(ctx context.Context, id, classification, classifiedBy string) error {
	_, err := c.Pool.Exec(ctx, `
		UPDATE catalog.tenant_variant_candidates_junk
		SET classification = $1, classified_at = NOW(), classified_by = $2
		WHERE id = $3`, classification, classifiedBy, id)
	return err
}

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

// PromoteAttribute runs the ALTER TABLE + bookkeeping inside a single tx.
// Vertical whitelist is intentionally narrow — only known per-vertical tables
// can receive new columns. SQL identifier validation is paranoid because we
// stamp the candidate's free-text key directly into ALTER TABLE.
func (c *Client) PromoteAttribute(ctx context.Context, candidateID, key, vertical, columnType, actorID string) error {
	verticalTable, ok := verticalTables[vertical]
	if !ok {
		return fmt.Errorf("unknown vertical %q (whitelist: %v)", vertical, verticalKeys())
	}
	if !validIdent(key) {
		return fmt.Errorf("invalid column name %q (must be snake_case [a-z0-9_])", key)
	}
	pgType, ok := allowedColumnTypes[columnType]
	if !ok {
		return fmt.Errorf("unsupported column type %q (allowed: text, integer, numeric, boolean, text_array)", columnType)
	}
	tx, err := c.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	alterSQL := fmt.Sprintf(`ALTER TABLE catalog.%s ADD COLUMN IF NOT EXISTS %s %s`,
		verticalTable, key, pgType)
	if _, err := tx.Exec(ctx, alterSQL); err != nil {
		return fmt.Errorf("alter table: %w", err)
	}
	_, err = tx.Exec(ctx, `
		UPDATE catalog.master_attribute_candidates
		SET status = 'promoted', promoted_to_column = $1, updated_at = NOW()
		WHERE id = $2`, fmt.Sprintf("%s.%s", verticalTable, key), candidateID)
	if err != nil {
		return fmt.Errorf("update candidate: %w", err)
	}
	// Mark all artifacts stale so next tenant import re-resolves the new column.
	_, err = tx.Exec(ctx, `
		UPDATE catalog.tenant_catalog_schema SET status = 'stale'
		WHERE mapping_artifact->'field_mapping' @> $1::jsonb`,
		fmt.Sprintf(`{"target":"candidate:%s"}`, key))
	if err != nil {
		return fmt.Errorf("mark artifacts stale: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	// Audit (best-effort, outside tx so rollback doesn't kill it).
	_ = c.WriteAudit(ctx, domain.AuditEntry{
		ActorKind:  "curator",
		ActorID:    actorID,
		EntityKind: "candidate",
		EntityID:   candidateID,
		Action:     "promote",
		AggregateMeta: map[string]interface{}{
			"migration": alterSQL,
			"vertical":  vertical,
			"key":       key,
		},
	})
	return nil
}

func (c *Client) DismissAttribute(ctx context.Context, candidateID, actorID string) error {
	_, err := c.Pool.Exec(ctx, `
		UPDATE catalog.master_attribute_candidates
		SET status = 'dismissed', updated_at = NOW() WHERE id = $1`, candidateID)
	if err != nil {
		return err
	}
	_ = c.WriteAudit(ctx, domain.AuditEntry{
		ActorKind: "curator", ActorID: actorID,
		EntityKind: "candidate", EntityID: candidateID, Action: "dismiss",
	})
	return nil
}

// --- helpers ---

var verticalTables = map[string]string{
	"cosmetics": "master_cosmetics",
	"laptops":   "master_laptops",
}

func verticalKeys() []string {
	out := make([]string, 0, len(verticalTables))
	for k := range verticalTables {
		out = append(out, k)
	}
	return out
}

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
