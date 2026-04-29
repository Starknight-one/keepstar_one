package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"keepstar-admin/internal/crypto/secretbox"
	"keepstar-admin/internal/domain"
)

// IntegrationsAdapter persists tenant integrations with encryption-at-rest
// for OAuth tokens and API keys. All Create/Update calls seal plaintext
// credentials before writing; all reads decrypt before returning.
type IntegrationsAdapter struct {
	client *Client
	box    *secretbox.Box
}

func NewIntegrationsAdapter(client *Client, box *secretbox.Box) *IntegrationsAdapter {
	return &IntegrationsAdapter{client: client, box: box}
}

func (a *IntegrationsAdapter) Create(ctx context.Context, in *domain.Integration) (*domain.Integration, error) {
	cfgJSON, err := json.Marshal(nonNilMap(in.Config))
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	var encrypted *string
	if in.Credentials != "" {
		envelope, err := a.box.Seal([]byte(in.Credentials))
		if err != nil {
			return nil, fmt.Errorf("encrypt credentials: %w", err)
		}
		encrypted = &envelope
	}
	query := `INSERT INTO admin.tenant_integrations
		(tenant_id, kind, status, display_name, external_id, credentials_encrypted, config)
		VALUES ($1, $2, $3, NULLIF($4,''), NULLIF($5,''), $6, $7)
		ON CONFLICT (tenant_id, kind, external_id) DO UPDATE SET
			status = EXCLUDED.status,
			display_name = EXCLUDED.display_name,
			credentials_encrypted = COALESCE(EXCLUDED.credentials_encrypted, admin.tenant_integrations.credentials_encrypted),
			config = EXCLUDED.config,
			updated_at = NOW()
		RETURNING id, created_at, updated_at`
	err = a.client.pool.QueryRow(ctx, query,
		in.TenantID, string(in.Kind), string(in.Status),
		in.DisplayName, in.ExternalID, encrypted, cfgJSON,
	).Scan(&in.ID, &in.CreatedAt, &in.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create integration: %w", err)
	}
	return in, nil
}

func (a *IntegrationsAdapter) Get(ctx context.Context, tenantID, id string) (*domain.Integration, error) {
	return a.queryOne(ctx,
		`WHERE id = $1 AND tenant_id = $2`,
		id, tenantID)
}

func (a *IntegrationsAdapter) GetByKindAndExternalID(ctx context.Context, tenantID string, kind domain.IntegrationKind, externalID string) (*domain.Integration, error) {
	return a.queryOne(ctx,
		`WHERE tenant_id = $1 AND kind = $2 AND external_id = $3`,
		tenantID, string(kind), externalID)
}

func (a *IntegrationsAdapter) GetByShopDomain(ctx context.Context, shopDomain string) (*domain.Integration, error) {
	return a.queryOne(ctx,
		`WHERE kind = 'shopify' AND external_id = $1`,
		shopDomain)
}

func (a *IntegrationsAdapter) ListByTenant(ctx context.Context, tenantID string) ([]domain.Integration, error) {
	rows, err := a.client.pool.Query(ctx, a.selectPrefix()+
		` WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list integrations: %w", err)
	}
	defer rows.Close()
	var out []domain.Integration
	for rows.Next() {
		in, err := a.scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *in)
	}
	return out, nil
}

func (a *IntegrationsAdapter) Update(ctx context.Context, in *domain.Integration) error {
	cfgJSON, err := json.Marshal(nonNilMap(in.Config))
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	var encrypted *string
	if in.Credentials != "" {
		envelope, err := a.box.Seal([]byte(in.Credentials))
		if err != nil {
			return fmt.Errorf("encrypt credentials: %w", err)
		}
		encrypted = &envelope
	}
	query := `UPDATE admin.tenant_integrations SET
		status = $1,
		display_name = NULLIF($2, ''),
		config = $3,
		credentials_encrypted = COALESCE($4, credentials_encrypted),
		last_error = NULLIF($5, ''),
		updated_at = NOW()
		WHERE id = $6 AND tenant_id = $7`
	tag, err := a.client.pool.Exec(ctx, query,
		string(in.Status), in.DisplayName, cfgJSON, encrypted,
		in.LastError, in.ID, in.TenantID)
	if err != nil {
		return fmt.Errorf("update integration: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("integration not found")
	}
	return nil
}

func (a *IntegrationsAdapter) UpdateStatus(ctx context.Context, id string, status domain.IntegrationStatus, lastError string) error {
	_, err := a.client.pool.Exec(ctx,
		`UPDATE admin.tenant_integrations SET status = $1, last_error = NULLIF($2,''), updated_at = NOW() WHERE id = $3`,
		string(status), lastError, id)
	if err != nil {
		return fmt.Errorf("update integration status: %w", err)
	}
	return nil
}

func (a *IntegrationsAdapter) UpdateLastSync(ctx context.Context, id string, jobID string) error {
	var jobIDParam any
	if jobID != "" {
		jobIDParam = jobID
	}
	_, err := a.client.pool.Exec(ctx,
		`UPDATE admin.tenant_integrations SET last_sync_at = NOW(), last_sync_job_id = $1, updated_at = NOW() WHERE id = $2`,
		jobIDParam, id)
	if err != nil {
		return fmt.Errorf("update integration last_sync: %w", err)
	}
	return nil
}

func (a *IntegrationsAdapter) Delete(ctx context.Context, tenantID, id string) error {
	tag, err := a.client.pool.Exec(ctx,
		`DELETE FROM admin.tenant_integrations WHERE id = $1 AND tenant_id = $2`,
		id, tenantID)
	if err != nil {
		return fmt.Errorf("delete integration: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("integration not found")
	}
	return nil
}

// --- OAuth state ---

func (a *IntegrationsAdapter) CreateOAuthState(ctx context.Context, s *domain.OAuthState) error {
	flowKind := s.FlowKind
	if flowKind == "" {
		flowKind = domain.OAuthFlowConnect
	}
	_, err := a.client.pool.Exec(ctx,
		`INSERT INTO admin.oauth_states (state, tenant_id, kind, shop_domain, flow_kind, expires_at)
		VALUES ($1, $2, $3, NULLIF($4,''), $5, $6)`,
		s.State, s.TenantID, s.Kind, s.ShopDomain, flowKind, s.ExpiresAt)
	if err != nil {
		return fmt.Errorf("create oauth state: %w", err)
	}
	return nil
}

func (a *IntegrationsAdapter) ConsumeOAuthState(ctx context.Context, state string) (*domain.OAuthState, error) {
	var s domain.OAuthState
	var shop *string
	err := a.client.pool.QueryRow(ctx,
		`DELETE FROM admin.oauth_states
		WHERE state = $1 AND expires_at > NOW()
		RETURNING state, tenant_id, kind, shop_domain, flow_kind, created_at, expires_at`,
		state,
	).Scan(&s.State, &s.TenantID, &s.Kind, &shop, &s.FlowKind, &s.CreatedAt, &s.ExpiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("oauth state not found or expired")
		}
		return nil, fmt.Errorf("consume oauth state: %w", err)
	}
	if shop != nil {
		s.ShopDomain = *shop
	}
	return &s, nil
}

func (a *IntegrationsAdapter) CleanupExpiredOAuthStates(ctx context.Context) (int64, error) {
	tag, err := a.client.pool.Exec(ctx,
		`DELETE FROM admin.oauth_states WHERE expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("cleanup oauth states: %w", err)
	}
	return tag.RowsAffected(), nil
}

// --- helpers ---

func (a *IntegrationsAdapter) selectPrefix() string {
	return `SELECT id, tenant_id, kind, status,
		COALESCE(display_name, ''), COALESCE(external_id, ''),
		credentials_encrypted, config,
		last_sync_at, COALESCE(last_sync_job_id::text, ''),
		COALESCE(last_error, ''),
		created_at, updated_at
		FROM admin.tenant_integrations`
}

func (a *IntegrationsAdapter) queryOne(ctx context.Context, whereClause string, args ...any) (*domain.Integration, error) {
	row := a.client.pool.QueryRow(ctx, a.selectPrefix()+" "+whereClause, args...)
	return a.scanRow(row)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (a *IntegrationsAdapter) scanRow(row rowScanner) (*domain.Integration, error) {
	var (
		in        domain.Integration
		kind      string
		status    string
		encrypted *string
		cfgJSON   []byte
		lastSync  *time.Time
	)
	err := row.Scan(
		&in.ID, &in.TenantID, &kind, &status,
		&in.DisplayName, &in.ExternalID,
		&encrypted, &cfgJSON,
		&lastSync, &in.LastSyncJobID,
		&in.LastError,
		&in.CreatedAt, &in.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("integration not found")
		}
		return nil, fmt.Errorf("scan integration: %w", err)
	}
	in.Kind = domain.IntegrationKind(kind)
	in.Status = domain.IntegrationStatus(status)
	if lastSync != nil {
		in.LastSyncAt = lastSync
	}
	if encrypted != nil && *encrypted != "" {
		plain, err := a.box.Open(*encrypted)
		if err != nil {
			// Fail-safe: surface decryption failure as error status and empty
			// credentials. Caller should treat as disconnected.
			in.Status = domain.IntegrationStatusError
			in.LastError = fmt.Sprintf("decryption failed: %v", err)
		} else {
			in.Credentials = string(plain)
		}
	}
	in.Config = map[string]any{}
	if len(cfgJSON) > 0 {
		_ = json.Unmarshal(cfgJSON, &in.Config)
	}
	return &in, nil
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
