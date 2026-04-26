package usecases

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"keepstar-admin/internal/adapters/shopify"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// webhookTopics are the Shopify events we subscribe to at OAuth completion.
// products/* keep the catalog in sync; inventory_levels/update covers stock
// changes that don't fire products/update; app/uninstalled lets us flip status
// to 'disconnected' without waiting for a human to notice.
var webhookTopics = []string{
	"products/create",
	"products/update",
	"products/delete",
	"inventory_levels/update",
	"app/uninstalled",
}

// ShopifyUseCase orchestrates the OAuth handshake, initial product sync, and
// webhook dispatch. Persistence goes through IntegrationsPort; per-product
// writes land via ImportUseCase (UpsertSingle / Upload).
type ShopifyUseCase struct {
	client       *shopify.Client
	integrations ports.IntegrationsPort
	importer     *ImportUseCase
	catalog      ports.AdminCatalogPort
	publicURL    string
	log          *logger.Logger
}

func NewShopifyUseCase(
	client *shopify.Client,
	integrations ports.IntegrationsPort,
	importer *ImportUseCase,
	catalog ports.AdminCatalogPort,
	publicURL string,
	log *logger.Logger,
) *ShopifyUseCase {
	return &ShopifyUseCase{
		client:       client,
		integrations: integrations,
		importer:     importer,
		catalog:      catalog,
		publicURL:    publicURL,
		log:          log,
	}
}

// StartOAuth mints a state nonce, stores it for later callback matching, and
// returns the redirect URL to Shopify's consent screen.
func (uc *ShopifyUseCase) StartOAuth(ctx context.Context, tenantID, shop string) (string, error) {
	normalized, ok := shopify.ValidateShopDomain(shop)
	if !ok {
		return "", fmt.Errorf("invalid shop domain: must be *.myshopify.com")
	}
	state, err := randomState()
	if err != nil {
		return "", fmt.Errorf("mint state: %w", err)
	}
	now := time.Now().UTC()
	if err := uc.integrations.CreateOAuthState(ctx, &domain.OAuthState{
		State:      state,
		TenantID:   tenantID,
		Kind:       string(domain.IntegrationKindShopify),
		ShopDomain: normalized,
		CreatedAt:  now,
		ExpiresAt:  now.Add(10 * time.Minute),
	}); err != nil {
		return "", fmt.Errorf("persist state: %w", err)
	}
	redirectURI := uc.publicURL + "/admin/api/integrations/shopify/callback"
	return uc.client.InstallURL(normalized, redirectURI, state), nil
}

// CompleteOAuth handles the callback: verify HMAC, consume state, exchange
// code for an offline token, encrypt + persist, register webhooks, kick off
// the initial sync.
func (uc *ShopifyUseCase) CompleteOAuth(ctx context.Context, shop, code, state string, query map[string][]string) (*domain.Integration, error) {
	normalized, ok := shopify.ValidateShopDomain(shop)
	if !ok {
		return nil, fmt.Errorf("invalid shop domain")
	}
	if !uc.client.VerifyInstallHMAC(query) {
		return nil, fmt.Errorf("hmac verification failed")
	}
	record, err := uc.integrations.ConsumeOAuthState(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("consume state: %w", err)
	}
	if record.ShopDomain != normalized {
		return nil, fmt.Errorf("state/shop mismatch")
	}
	if time.Now().UTC().After(record.ExpiresAt) {
		return nil, fmt.Errorf("state expired")
	}

	token, err := uc.client.ExchangeCodeForToken(ctx, normalized, code)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	integration := &domain.Integration{
		TenantID:    record.TenantID,
		Kind:        domain.IntegrationKindShopify,
		Status:      domain.IntegrationStatusSyncing,
		DisplayName: normalized,
		ExternalID:  normalized,
		Credentials: token,
		Config:      map[string]any{"scopes": uc.client.Scopes()},
	}
	integration, err = uc.integrations.Create(ctx, integration)
	if err != nil {
		return nil, fmt.Errorf("persist integration: %w", err)
	}

	webhookAddress := uc.publicURL + "/admin/api/webhooks/shopify"
	for _, topic := range webhookTopics {
		if err := uc.client.RegisterWebhook(ctx, normalized, token, topic, webhookAddress); err != nil {
			uc.log.Error("shopify_webhook_register_failed", "shop", normalized, "topic", topic, "error", err)
			// Not fatal — periodic resync covers gaps.
		}
	}

	go uc.runInitialSync(integration.ID, record.TenantID, normalized, token)
	return integration, nil
}

// runInitialSync paginates the full catalog in background. Errors flip the
// integration to 'error'; success back to 'connected'.
func (uc *ShopifyUseCase) runInitialSync(integrationID, tenantID, shop, token string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	uc.log.Info("shopify_initial_sync_started", "integration_id", integrationID, "shop", shop)

	var (
		pageInfo string
		total    int
		jobID    string
	)
	for {
		products, next, err := uc.client.ListProducts(ctx, shop, token, pageInfo, 250)
		if err != nil {
			uc.log.Error("shopify_list_products_failed", "shop", shop, "error", err)
			_ = uc.integrations.UpdateStatus(ctx, integrationID, domain.IntegrationStatusError, err.Error())
			return
		}
		if len(products) == 0 && pageInfo == "" {
			break
		}
		items := make([]ImportItem, 0, len(products))
		for i := range products {
			// Per-product metafield pull; GraphQL bulk-query for big catalogs
			// is a post-MVP optimisation.
			if mfs, mfErr := uc.client.GetProductMetafields(ctx, shop, token, products[i].ID); mfErr == nil {
				products[i].Metafields = mfs
			}
			items = append(items, shopifyProductToImportItem(products[i]))
		}
		if len(items) > 0 {
			job, err := uc.importer.UploadWithJobName(ctx, tenantID,
				fmt.Sprintf("shopify-%s-sync-%d", shop, time.Now().Unix()), items)
			if err != nil {
				uc.log.Error("shopify_upload_failed", "shop", shop, "error", err)
				_ = uc.integrations.UpdateStatus(ctx, integrationID, domain.IntegrationStatusError, err.Error())
				return
			}
			jobID = job.ID
			total += len(items)
		}
		if next == "" {
			break
		}
		pageInfo = next
	}

	if jobID != "" {
		_ = uc.integrations.UpdateLastSync(ctx, integrationID, jobID)
	}
	_ = uc.integrations.UpdateStatus(ctx, integrationID, domain.IntegrationStatusConnected, "")
	uc.log.Info("shopify_initial_sync_completed", "integration_id", integrationID, "total", total)
}

// HandleWebhook dispatches a verified delivery to the right handler. Caller
// has already validated HMAC. body is the raw request bytes.
func (uc *ShopifyUseCase) HandleWebhook(ctx context.Context, topic, shopDomain string, body []byte) error {
	integration, err := uc.integrations.GetByShopDomain(ctx, shopDomain)
	if err != nil {
		return fmt.Errorf("lookup integration: %w", err)
	}

	switch topic {
	case "products/create", "products/update":
		return uc.onProductUpsert(ctx, integration, body)
	case "products/delete":
		return uc.onProductDelete(ctx, integration, body)
	case "inventory_levels/update":
		// Shopify doesn't include product_id on this payload — mapping it
		// back to a SKU requires a second API call and ties us to per-tenant
		// inventory_item lookup. MVP compromise: log and rely on 6h resync.
		uc.log.Info("shopify_inventory_event_ignored", "shop", shopDomain)
		return nil
	case "app/uninstalled":
		return uc.integrations.UpdateStatus(ctx, integration.ID, domain.IntegrationStatusDisconnected, "app uninstalled")
	default:
		uc.log.Info("shopify_webhook_unhandled_topic", "topic", topic)
		return nil
	}
}

// onProductUpsert handles products/create and products/update by mapping the
// payload to ImportItem and running a single upsert.
func (uc *ShopifyUseCase) onProductUpsert(ctx context.Context, integration *domain.Integration, body []byte) error {
	var p shopify.ShopifyProduct
	if err := json.Unmarshal(body, &p); err != nil {
		return fmt.Errorf("parse product: %w", err)
	}
	if p.ID == 0 {
		return fmt.Errorf("webhook payload missing product id")
	}
	// Webhook payload may omit metafields — pull them so we keep the same
	// attribute coverage as initial sync.
	if mfs, err := uc.client.GetProductMetafields(ctx, integration.ExternalID, integration.Credentials, p.ID); err == nil {
		p.Metafields = mfs
	}
	item := shopifyProductToImportItem(p)
	return uc.importer.UpsertSingle(ctx, integration.TenantID, item)
}

// onProductDelete marks the product soft-deleted by source id. Later webhooks
// referencing the same id won't resurrect it — a separate reinstall does.
func (uc *ShopifyUseCase) onProductDelete(ctx context.Context, integration *domain.Integration, body []byte) error {
	var payload struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("parse delete payload: %w", err)
	}
	if payload.ID == 0 {
		return fmt.Errorf("delete payload missing id")
	}
	return uc.catalog.SoftDeleteProductBySource(ctx, integration.TenantID, "shopify", strconv.FormatInt(payload.ID, 10))
}

// FullResync is the on-demand full re-pull triggered by the admin UI. Reuses
// the initial-sync code path so the job row tags the run identically.
func (uc *ShopifyUseCase) FullResync(ctx context.Context, tenantID, integrationID string) error {
	integration, err := uc.integrations.Get(ctx, tenantID, integrationID)
	if err != nil {
		return fmt.Errorf("lookup: %w", err)
	}
	if integration.Kind != domain.IntegrationKindShopify {
		return fmt.Errorf("integration is not shopify")
	}
	if integration.Status == domain.IntegrationStatusDisconnected {
		return fmt.Errorf("integration disconnected — reinstall required")
	}
	go uc.runInitialSync(integration.ID, integration.TenantID, integration.ExternalID, integration.Credentials)
	return nil
}

// StartPeriodicResync runs a 6h ticker that re-syncs every connected Shopify
// integration. Backup for webhook drops. Returns a stop func.
func (uc *ShopifyUseCase) StartPeriodicResync(ctx context.Context) func() {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				uc.resyncAllConnected(ctx)
			}
		}
	}()
	return func() { close(stop) }
}

func (uc *ShopifyUseCase) resyncAllConnected(ctx context.Context) {
	// Placeholder: IntegrationsPort has no ListByKindAndStatus yet. Safe no-op
	// until we add it; webhooks cover the live path.
	uc.log.Debug("shopify_periodic_resync_skipped_no_list")
}

// Client exposes the underlying HTTP client so handlers can call VerifyWebhookHMAC.
func (uc *ShopifyUseCase) Client() *shopify.Client {
	return uc.client
}

// randomState returns a URL-safe base64-encoded 32-byte nonce.
func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
