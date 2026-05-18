package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"keepstar-admin/internal/ports"
)

type SessionsRepo struct {
	client *Client
}

func NewSessionsRepo(client *Client) *SessionsRepo {
	return &SessionsRepo{client: client}
}

// sessionColumns matches scan() exactly. Bumping it requires updating both.
const sessionColumns = `id, user_id::text, COALESCE(tenant_id::text,''), refresh_token_hash,
	COALESCE(user_agent,''), COALESCE(host(ip),''),
	COALESCE(browser_name,''), COALESCE(browser_version,''),
	COALESCE(os_name,''), COALESCE(device_kind,''),
	COALESCE(geo_country,''), COALESCE(geo_city,''),
	created_at, expires_at, revoked_at`

func (r *SessionsRepo) Create(ctx context.Context, s *ports.Session) error {
	q := `INSERT INTO admin.sessions
			(user_id, tenant_id, refresh_token_hash, user_agent, ip,
			 browser_name, browser_version, os_name, device_kind,
			 geo_country, geo_city, expires_at)
		  VALUES ($1, NULLIF($2,'')::uuid, $3, NULLIF($4,''), NULLIF($5,'')::inet,
			NULLIF($6,''), NULLIF($7,''), NULLIF($8,''), NULLIF($9,''),
			NULLIF($10,''), NULLIF($11,''), $12)
		  RETURNING id, created_at`
	return r.client.pool.QueryRow(ctx, q,
		s.UserID, s.TenantID, s.TokenHash, s.UserAgent, s.IP,
		s.BrowserName, s.BrowserVersion, s.OSName, s.DeviceKind,
		s.GeoCountry, s.GeoCity, s.ExpiresAt,
	).Scan(&s.ID, &s.CreatedAt)
}

func (r *SessionsRepo) FindActive(ctx context.Context, tokenHash string) (*ports.Session, error) {
	q := `SELECT ` + sessionColumns + `
		  FROM admin.sessions
		  WHERE refresh_token_hash = $1`
	return r.scan(ctx, q, tokenHash)
}

func (r *SessionsRepo) FindByID(ctx context.Context, id string) (*ports.Session, error) {
	q := `SELECT ` + sessionColumns + `
		  FROM admin.sessions WHERE id = $1`
	return r.scan(ctx, q, id)
}

func (r *SessionsRepo) scan(ctx context.Context, q string, args ...any) (*ports.Session, error) {
	var s ports.Session
	err := r.client.pool.QueryRow(ctx, q, args...).Scan(
		&s.ID, &s.UserID, &s.TenantID, &s.TokenHash,
		&s.UserAgent, &s.IP,
		&s.BrowserName, &s.BrowserVersion, &s.OSName, &s.DeviceKind,
		&s.GeoCountry, &s.GeoCity,
		&s.CreatedAt, &s.ExpiresAt, &s.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find session: %w", err)
	}
	return &s, nil
}

func (r *SessionsRepo) Revoke(ctx context.Context, id string) error {
	q := `UPDATE admin.sessions SET revoked_at = NOW()
		  WHERE id = $1 AND revoked_at IS NULL`
	_, err := r.client.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (r *SessionsRepo) RevokeAllForUser(ctx context.Context, userID string) error {
	q := `UPDATE admin.sessions SET revoked_at = NOW()
		  WHERE user_id = $1 AND revoked_at IS NULL`
	_, err := r.client.pool.Exec(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return nil
}

func (r *SessionsRepo) ListActiveForUser(ctx context.Context, userID string) ([]*ports.Session, error) {
	q := `SELECT ` + sessionColumns + `
		  FROM admin.sessions
		  WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > NOW()
		  ORDER BY created_at DESC`
	rows, err := r.client.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var out []*ports.Session
	for rows.Next() {
		var s ports.Session
		if err := rows.Scan(
			&s.ID, &s.UserID, &s.TenantID, &s.TokenHash,
			&s.UserAgent, &s.IP,
			&s.BrowserName, &s.BrowserVersion, &s.OSName, &s.DeviceKind,
			&s.GeoCountry, &s.GeoCity,
			&s.CreatedAt, &s.ExpiresAt, &s.RevokedAt,
		); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}
