package domain

import "time"

type IntegrationKind string

const (
	IntegrationKindShopify     IntegrationKind = "shopify"
	IntegrationKindCSV         IntegrationKind = "csv"
	IntegrationKindGoogleSheets IntegrationKind = "google_sheets"
)

type IntegrationStatus string

const (
	IntegrationStatusConnected    IntegrationStatus = "connected"
	IntegrationStatusSyncing      IntegrationStatus = "syncing"
	IntegrationStatusError        IntegrationStatus = "error"
	IntegrationStatusDisconnected IntegrationStatus = "disconnected"
)

// Integration describes one catalog source connected to a tenant. Credentials
// are stored encrypted via internal/crypto/secretbox and only decrypted when
// a usecase needs them — callers get the plaintext in Credentials.
type Integration struct {
	ID             string            `json:"id"`
	TenantID       string            `json:"tenantId"`
	Kind           IntegrationKind   `json:"kind"`
	Status         IntegrationStatus `json:"status"`
	DisplayName    string            `json:"displayName,omitempty"`
	ExternalID     string            `json:"externalId,omitempty"`
	Credentials    string            `json:"-"` // never serialised in API
	Config         map[string]any    `json:"config"`
	LastSyncAt     *time.Time        `json:"lastSyncAt,omitempty"`
	LastSyncJobID  string            `json:"lastSyncJobId,omitempty"`
	LastError      string            `json:"lastError,omitempty"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
}

// OAuthFlowKind discriminates the two entry points into Shopify OAuth:
//   - "connect": user logged into our admin clicks Connect Shopify on an
//     existing tenant. Tenant pre-exists in state.TenantID.
//   - "install": Shopify hands off after the merchant installs our app from
//     the App Store / Partner Dashboard / their Shopify admin Apps page.
//     Tenant is auto-provisioned at install entry (state.TenantID points at
//     the new row). After OAuth completes we additionally provision an
//     admin_user from the shop owner's email and email a magic-link.
const (
	OAuthFlowConnect = "connect"
	OAuthFlowInstall = "install"
)

// OAuthState is the short-lived nonce issued at OAuth Install start and
// verified in the callback. Single-use — callback deletes after match.
type OAuthState struct {
	State      string
	TenantID   string
	Kind       string
	ShopDomain string
	FlowKind   string // OAuthFlowConnect | OAuthFlowInstall — empty defaults to connect for legacy rows
	CreatedAt  time.Time
	ExpiresAt  time.Time
}
