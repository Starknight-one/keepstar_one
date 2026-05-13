// Package usecases — Shopify metadata-first import (M4 spec §4.1).
//
// This file is the entry point for the new pipeline that supersedes the
// legacy REST-based ShopifyUseCase. The flow lives across multiple files in
// this package, sequenced in commits 4a/b/c/d:
//
//   4a (THIS FILE)        — DumpToStaging: bulk pull → catalog.shopify_raw_imports
//   4b (metadata_harvest, auto_map_tier1, match_cascade, junk_detector)
//                          — pure-Go analysis + match logic, no LLM
//   4c (discovery_agent, validate_artifact)
//                          — Sonnet 4.6 multi-turn agent + validation pass
//   4d (shopify_harvester, embedding_job, webhook re-wire)
//                          — applies artifact, writes master/listing, hash-diff
//
// The legacy ShopifyUseCase (shopify.go + shopify_mapper.go) stays in place
// during 4a-c. Cut-over happens in 4d (DI swap in main.go + delete legacy).
package usecases

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"keepstar-admin/internal/adapters/shopify"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// webhookTopics — Shopify events we subscribe to at OAuth completion. After
// the curator-driven pivot (2026-04-27) products/* webhooks rewrite the
// listing in catalog.products via harvester-lite (Этап 2.2); inventory_levels
// is logged-only; app/uninstalled flips integration status to disconnected.
var webhookTopics = []string{
	"products/create",
	"products/update",
	"products/delete",
	"inventory_levels/update",
	"app/uninstalled",
}

// bulkPollInterval is how often we ask Shopify for current bulk-op status.
// Shopify recommends 1-5s; faster is wasted work, slower delays UI updates.
const bulkPollInterval = 3 * time.Second

// bulkOpMaxWait is the hard ceiling on a single bulk operation. 100k products
// usually finish in 10-20 minutes; anything past 90 means something's wrong on
// Shopify's side (rare) or we sent a query they can't bulk-process (caller bug).
const bulkOpMaxWait = 90 * time.Minute

// HarvesterLite is the per-product write path that replaces legacy
// runInitialSync. It writes to catalog.products only (no master_*) — implemented
// in Этап 2.2. This interface lets the V2 UC own webhook routing without
// importing the harvester package.
type HarvesterLite interface {
	// RunForTenant streams shopify_raw_imports for the tenant and upserts
	// listings into catalog.products. Called after CompleteOAuth + DumpToStaging.
	RunForTenant(ctx context.Context, tenantID string) (productCount int, err error)

	// UpsertOne applies the same Tier-1 mapping for a single Shopify-product
	// payload (used by webhook). The payload is the raw Shopify webhook body.
	UpsertOne(ctx context.Context, tenantID string, body []byte) error

	// SoftDeleteOne marks a listing deleted by source id (products/delete webhook).
	SoftDeleteOne(ctx context.Context, tenantID, sourceID string) error
}

// InstallCompletionHook is invoked after a successful install-flow OAuth (i.e.
// FlowKind == "install"). Implementations live in main.go and typically: fetch
// the shop owner email via the freshly-issued token, find-or-create an
// admin_user for that email + tenant, and issue a magic-link so the merchant
// can sign in from their inbox.
//
// Runs SYNCHRONOUSLY so the handler can branch on the return value:
//   - "" — happy path (email present, magic-link queued / sent async by the
//     implementation, no special UI handling needed).
//   - non-empty string — a pending-shop-link token: shop had no owner email,
//     the caller should redirect to /auth/install-complete?pending_link=TOKEN
//     so the merchant can sign in via standard methods and attach the tenant
//     after auth.
//
// Errors are logged by the implementation and never surface to the caller —
// the integration is already persisted at this point and we don't want to
// roll back a working install over a transient email/SMTP failure.
type InstallCompletionHook func(ctx context.Context, tenantID, shopDomain, token string) (pendingToken string)

// ShopifyV2UseCase owns the full Shopify lifecycle after the curator-driven
// pivot (2026-04-27): OAuth, webhook subscription, dump-to-staging, discovery
// agent, and webhook handling via harvester-lite. The legacy ShopifyUseCase
// (REST initial-sync writing directly to master_products) was removed.
type ShopifyV2UseCase struct {
	client          *shopify.Client
	integrations    ports.IntegrationsPort
	catalog         ports.AdminCatalogPort // tenant CRUD for install-flow auto-provision
	staging         ports.ShopifyStagingPort
	harvester       HarvesterLite        // wired from Этап 2.2; nil-safe (webhook upsert no-op'd)
	onInstallDone   InstallCompletionHook // fired after install-flow OAuth; nil-safe
	publicURL       string
	log             *logger.Logger
}

func NewShopifyV2UseCase(
	client *shopify.Client,
	integrations ports.IntegrationsPort,
	catalog ports.AdminCatalogPort,
	staging ports.ShopifyStagingPort,
	publicURL string,
	log *logger.Logger,
) *ShopifyV2UseCase {
	return &ShopifyV2UseCase{
		client:       client,
		integrations: integrations,
		catalog:      catalog,
		staging:      staging,
		publicURL:    publicURL,
		log:          log,
	}
}

// SetHarvester wires the harvester-lite implementation. Called from main.go
// after both the V2 UC and harvester are constructed (avoids import cycles
// while keeping main.go DI flat).
func (uc *ShopifyV2UseCase) SetHarvester(h HarvesterLite) { uc.harvester = h }

// SetInstallCompletionHook wires the post-install user-provisioning step.
// Called from main.go once the magic-link use case is constructed.
func (uc *ShopifyV2UseCase) SetInstallCompletionHook(h InstallCompletionHook) {
	uc.onInstallDone = h
}

// Client exposes the underlying HTTP client so handlers can call VerifyWebhookHMAC.
func (uc *ShopifyV2UseCase) Client() *shopify.Client { return uc.client }

// StartOAuth — mints state nonce, returns redirect URL for Shopify consent.
func (uc *ShopifyV2UseCase) StartOAuth(ctx context.Context, tenantID, shop string) (string, error) {
	return uc.startOAuth(ctx, tenantID, shop, domain.OAuthFlowConnect)
}

func (uc *ShopifyV2UseCase) startOAuth(ctx context.Context, tenantID, shop, flowKind string) (string, error) {
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
		FlowKind:   flowKind,
		CreatedAt:  now,
		ExpiresAt:  now.Add(10 * time.Minute),
	}); err != nil {
		return "", fmt.Errorf("persist state: %w", err)
	}
	redirectURI := uc.publicURL + "/admin/api/integrations/shopify/callback"
	return uc.client.InstallURL(normalized, redirectURI, state), nil
}

// InstallEntryResult describes what the install entry handler should do next.
// Exactly one of InstallURL or RedirectURL is set. RedirectURL points back into
// our admin when the shop is already installed (no OAuth needed); InstallURL
// kicks off Shopify consent for first-time and reinstall paths.
type InstallEntryResult struct {
	InstallURL  string
	RedirectURL string
}

// StartInstallEntry is the unauthenticated entry point used when Shopify itself
// drives the install — App Store, Partner Dashboard, or merchant clicking "Open
// app" inside their Shopify admin. The handler hands us the shop domain after
// HMAC verification and we dispatch:
//
//   - Existing integration, status=connected → already installed, just bounce
//     them into our admin (caller redirects to RedirectURL).
//   - Existing integration, any other status → reinstall on the same tenant,
//     start OAuth (FlowKind=install).
//   - No existing integration → auto-provision tenant from shop_domain, start
//     OAuth (FlowKind=install). The tenant row is created up-front so a user
//     who abandons mid-consent and retries reuses the same tenant instead of
//     spawning duplicates.
//
// Idempotency: the dup-check is GetByShopDomain on the integrations table, so
// repeated install entries for the same shop converge on one tenant + one
// integration row regardless of how many times the merchant retries.
func (uc *ShopifyV2UseCase) StartInstallEntry(ctx context.Context, shop string) (*InstallEntryResult, error) {
	normalized, ok := shopify.ValidateShopDomain(shop)
	if !ok {
		return nil, fmt.Errorf("invalid shop domain: must be *.myshopify.com")
	}

	existing, err := uc.integrations.GetByShopDomain(ctx, normalized)
	if err == nil && existing != nil {
		if existing.Status == domain.IntegrationStatusConnected {
			uc.log.Info("shopify_install_entry_already_connected",
				"shop", normalized, "tenant_id", existing.TenantID, "integration_id", existing.ID)
			return &InstallEntryResult{
				RedirectURL: "/integrations?already_installed=1&id=" + existing.ID,
			}, nil
		}
		// Reinstall on the same tenant — keep tenant_id, force fresh OAuth.
		installURL, err := uc.startOAuth(ctx, existing.TenantID, normalized, domain.OAuthFlowInstall)
		if err != nil {
			return nil, err
		}
		uc.log.Info("shopify_install_entry_reinstall",
			"shop", normalized, "tenant_id", existing.TenantID, "prior_status", existing.Status)
		return &InstallEntryResult{InstallURL: installURL}, nil
	}

	// First-time install: auto-provision tenant. Slug derived from shop
	// subdomain ("hey-babes-cosmetics.myshopify.com" → "hey-babes-cosmetics").
	subdomain := strings.TrimSuffix(normalized, ".myshopify.com")
	tenant, err := uc.catalog.CreateTenant(ctx, &domain.Tenant{
		Slug: slugify(subdomain),
		Name: subdomain,
		Type: "retailer",
		Settings: map[string]any{
			"currency":         "USD",
			"shop_domain":      normalized,
			"provisioned_via":  "shopify_install",
			"provisioned_at":   time.Now().UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("auto-provision tenant: %w", err)
	}
	uc.log.Info("shopify_install_entry_tenant_provisioned",
		"shop", normalized, "tenant_id", tenant.ID)

	installURL, err := uc.startOAuth(ctx, tenant.ID, normalized, domain.OAuthFlowInstall)
	if err != nil {
		return nil, err
	}
	return &InstallEntryResult{InstallURL: installURL}, nil
}

// CompleteOAuth — verify HMAC, consume state, exchange code → token, persist
// integration, register webhooks, kick off background dump-to-staging +
// harvester-lite (writes only catalog.products, no master_*). Returns the
// flow kind so the HTTP handler can pick the right post-OAuth redirect (the
// install path lands on a "check your email" page; the connect path goes
// straight back into the in-app integrations view).
//
// pendingToken is non-empty only on install flows where the shop has no
// owner email — the caller redirects to /auth/install-complete?pending_link=…
// to walk the merchant through a standard sign-in that attaches the tenant.
func (uc *ShopifyV2UseCase) CompleteOAuth(ctx context.Context, shop, code, state string, query map[string][]string) (*domain.Integration, string, string, error) {
	normalized, ok := shopify.ValidateShopDomain(shop)
	if !ok {
		return nil, "", "", fmt.Errorf("invalid shop domain")
	}
	if !uc.client.VerifyInstallHMAC(query) {
		return nil, "", "", fmt.Errorf("hmac verification failed")
	}
	record, err := uc.integrations.ConsumeOAuthState(ctx, state)
	if err != nil {
		return nil, "", "", fmt.Errorf("consume state: %w", err)
	}
	if record.ShopDomain != normalized {
		return nil, "", "", fmt.Errorf("state/shop mismatch")
	}
	if time.Now().UTC().After(record.ExpiresAt) {
		return nil, "", "", fmt.Errorf("state expired")
	}

	token, err := uc.client.ExchangeCodeForToken(ctx, normalized, code)
	if err != nil {
		return nil, "", "", fmt.Errorf("exchange code: %w", err)
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
		return nil, "", "", fmt.Errorf("persist integration: %w", err)
	}

	webhookAddress := uc.publicURL + "/admin/api/webhooks/shopify"
	for _, topic := range webhookTopics {
		if err := uc.client.RegisterWebhook(ctx, normalized, token, topic, webhookAddress); err != nil {
			uc.log.Error("shopify_webhook_register_failed",
				"shop", normalized, "topic", topic, "error", err)
			// Not fatal — periodic resync (curator-triggered) covers gaps.
		}
	}

	// Install flow: run the install-completion hook SYNCHRONOUSLY so the
	// handler can branch on its return value. The hook itself fans the slow
	// parts (Shopify shop-info fetch, magic-link email send) into goroutines
	// when appropriate, so the user-facing OAuth-callback latency stays low.
	// Non-empty return = pending-shop-link token (shop had no owner email),
	// caller redirects to /auth/install-complete?pending_link=…
	var pendingToken string
	if record.FlowKind == domain.OAuthFlowInstall && uc.onInstallDone != nil {
		pendingToken = uc.onInstallDone(ctx, record.TenantID, normalized, token)
	}

	go uc.runInitialIngest(integration.ID, record.TenantID)
	return integration, record.FlowKind, pendingToken, nil
}

// runInitialIngest — background after Connect: dump-to-staging then run
// harvester-lite. No master_* writes — harvester-lite only fills catalog.products.
// Status flips to 'connected' on success, 'error' on failure.
func (uc *ShopifyV2UseCase) runInitialIngest(integrationID, tenantID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
	defer cancel()

	uc.log.Info("shopify_v2_initial_ingest_started", "integration_id", integrationID)
	dump, err := uc.DumpToStaging(ctx, tenantID, integrationID)
	if err != nil {
		uc.log.Error("shopify_v2_initial_dump_failed", "integration_id", integrationID, "error", err)
		_ = uc.integrations.UpdateStatus(ctx, integrationID, domain.IntegrationStatusError, err.Error())
		return
	}

	if uc.harvester == nil {
		// Harvester not wired yet (Этап 2.2 in progress) — staging is populated
		// but listings stay empty. Mark connected anyway so the UI doesn't sit
		// on 'syncing' forever; curator-driven re-run will fill listings later.
		uc.log.Info("shopify_v2_harvester_not_wired_skipping_apply",
			"integration_id", integrationID, "products_in_staging", dump.ProductCount)
		_ = uc.integrations.UpdateStatus(ctx, integrationID, domain.IntegrationStatusConnected, "")
		return
	}

	count, err := uc.harvester.RunForTenant(ctx, tenantID)
	if err != nil {
		uc.log.Error("shopify_v2_harvester_failed", "integration_id", integrationID, "error", err)
		_ = uc.integrations.UpdateStatus(ctx, integrationID, domain.IntegrationStatusError, err.Error())
		return
	}
	uc.log.Info("shopify_v2_initial_ingest_completed",
		"integration_id", integrationID, "products_written", count)
	_ = uc.integrations.UpdateStatus(ctx, integrationID, domain.IntegrationStatusConnected, "")
}

// HandleWebhook dispatches verified Shopify webhook deliveries. Caller has
// validated HMAC; body is the raw bytes.
func (uc *ShopifyV2UseCase) HandleWebhook(ctx context.Context, topic, shopDomain string, body []byte) error {
	integration, err := uc.integrations.GetByShopDomain(ctx, shopDomain)
	if err != nil {
		return fmt.Errorf("lookup integration: %w", err)
	}
	switch topic {
	case "products/create", "products/update":
		if uc.harvester == nil {
			uc.log.Info("shopify_v2_webhook_upsert_skipped_no_harvester",
				"shop", shopDomain, "topic", topic)
			return nil
		}
		return uc.harvester.UpsertOne(ctx, integration.TenantID, body)
	case "products/delete":
		if uc.harvester == nil {
			return nil
		}
		var payload struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return fmt.Errorf("parse delete payload: %w", err)
		}
		if payload.ID == 0 {
			return fmt.Errorf("delete payload missing id")
		}
		return uc.harvester.SoftDeleteOne(ctx, integration.TenantID,
			fmt.Sprintf("%d", payload.ID))
	case "inventory_levels/update":
		// Shopify's payload doesn't carry product_id; mapping it back requires
		// inventory_item lookup. Curator-triggered re-dump covers stock drift.
		uc.log.Info("shopify_v2_inventory_event_ignored", "shop", shopDomain)
		return nil
	case "app/uninstalled":
		return uc.integrations.UpdateStatus(ctx, integration.ID,
			domain.IntegrationStatusDisconnected, "app uninstalled")
	default:
		uc.log.Info("shopify_v2_webhook_unhandled_topic", "topic", topic)
		return nil
	}
}

// randomState returns a URL-safe base64-encoded 32-byte nonce.
func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// DumpToStagingResult carries the outcome of a single bulk-pull. ProductCount
// is the most useful single number for the progress UI; CollectionCount /
// MetafieldDefCount / Vendors are smaller numbers shown alongside.
type DumpToStagingResult struct {
	OperationID      string
	ProductCount     int
	CollectionCount  int
	MetafieldDefs    int
	NavigationItems  int
	VendorCount      int
	ProductTypeCount int
	TagCount         int
	Duration         time.Duration
}

// DumpToStaging runs one full metadata-first pull for a tenant. It is
// idempotent: ON CONFLICT in the staging table replaces existing rows so
// re-running just refreshes the buffer. The caller is responsible for
// authorization (tenant must own the integration).
//
// Sequence:
//   1. Pull metafield definitions (3 owner types × small list each)
//   2. Pull main navigation menu (one snapshot)
//   3. Pull shop reference data (vendors, product types, tags)
//   4. Fire bulkOperationRunQuery for the full products graph
//   5. Poll until COMPLETED / FAILED / CANCELED
//   6. Stream JSONL → split into per-product rows → upsert into staging
//
// Steps 1-3 finish in seconds. Step 4-5 dominate wallclock for big catalogs.
// Step 6 is bound by Postgres write throughput (~1k products/sec on Neon).
func (uc *ShopifyV2UseCase) DumpToStaging(ctx context.Context, tenantID, integrationID string) (*DumpToStagingResult, error) {
	start := time.Now()
	integration, err := uc.integrations.Get(ctx, tenantID, integrationID)
	if err != nil {
		return nil, fmt.Errorf("lookup integration: %w", err)
	}
	shop := integration.ExternalID
	token := integration.Credentials
	if shop == "" || token == "" {
		return nil, fmt.Errorf("integration not connected (missing shop or token)")
	}

	res := &DumpToStagingResult{}

	// --- Phase 1: metadata snapshots (~5 sec) -----------------------------------
	if err := uc.dumpMetafieldDefs(ctx, tenantID, shop, token, res); err != nil {
		return nil, fmt.Errorf("metafield defs: %w", err)
	}
	if err := uc.dumpNavigation(ctx, tenantID, shop, token, res); err != nil {
		return nil, fmt.Errorf("navigation: %w", err)
	}
	if err := uc.dumpShopReferences(ctx, tenantID, shop, token, res); err != nil {
		return nil, fmt.Errorf("shop references: %w", err)
	}

	// --- Phase 2: bulk products pull (5-40 min) ---------------------------------
	op, err := uc.runBulkProductsAndWait(ctx, shop, token)
	if err != nil {
		return nil, fmt.Errorf("bulk products: %w", err)
	}
	res.OperationID = op.ID

	// --- Phase 3: stream JSONL → staging ----------------------------------------
	if op.URL == "" {
		// Empty catalogs return no URL. Not an error — just nothing to stream.
		uc.log.Info("shopify_v2_bulk_op_no_url", "shop", shop, "status", op.Status)
		res.Duration = time.Since(start)
		return res, nil
	}
	productCount, err := uc.streamJSONLToStaging(ctx, tenantID, op.URL)
	if err != nil {
		return nil, fmt.Errorf("stream jsonl: %w", err)
	}
	res.ProductCount = productCount

	// Recount collections from the staging table since the bulk JSONL inlines
	// collections under product nodes (no separate top-level "collection"
	// rows are written by streamJSONLToStaging in 4a).
	if counts, err := uc.staging.CountByKind(ctx, tenantID); err == nil {
		res.CollectionCount = counts["collection"]
	}

	res.Duration = time.Since(start)
	uc.log.Info("shopify_v2_dump_to_staging_completed",
		"shop", shop,
		"products", res.ProductCount,
		"metafield_defs", res.MetafieldDefs,
		"vendors", res.VendorCount,
		"duration_ms", res.Duration.Milliseconds(),
	)
	return res, nil
}

// dumpMetafieldDefs writes one staging row per owner type. The discovery agent
// reads these to know which metafields are formally declared (high-signal,
// merchant intentional) vs ad-hoc (lower-signal).
func (uc *ShopifyV2UseCase) dumpMetafieldDefs(ctx context.Context, tenantID, shop, token string, res *DumpToStagingResult) error {
	for _, ownerType := range []string{"PRODUCT", "PRODUCTVARIANT", "COLLECTION"} {
		defs, err := uc.client.MetafieldDefinitions(ctx, shop, token, ownerType)
		if err != nil {
			return fmt.Errorf("%s defs: %w", ownerType, err)
		}
		payload, _ := json.Marshal(defs)
		sourceID := "metafield_defs:" + strings.ToLower(ownerType)
		if err := uc.staging.UpsertRaw(ctx, tenantID, "metadata", sourceID, payload); err != nil {
			return fmt.Errorf("staging write %s: %w", sourceID, err)
		}
		res.MetafieldDefs += len(defs)
	}
	return nil
}

// dumpNavigation snapshots the published storefront menu. Many shops use only
// "main-menu" — a missing menu is logged but not fatal (we'll fall back to
// collection-handle prefixes when building the category tree).
func (uc *ShopifyV2UseCase) dumpNavigation(ctx context.Context, tenantID, shop, token string, res *DumpToStagingResult) error {
	menu, err := uc.client.NavigationMenu(ctx, shop, token, "main-menu")
	if err != nil {
		// Log and continue — navigation is helpful but not required.
		uc.log.Info("shopify_v2_menu_unavailable", "shop", shop, "error", err.Error())
		return nil
	}
	if menu == nil {
		uc.log.Info("shopify_v2_menu_not_present", "shop", shop, "handle", "main-menu")
		return nil
	}
	payload, _ := json.Marshal(menu)
	if err := uc.staging.UpsertRaw(ctx, tenantID, "menu", "main-menu", payload); err != nil {
		return fmt.Errorf("staging menu: %w", err)
	}
	res.NavigationItems = countMenuItems(menu.Items)
	return nil
}

// dumpShopReferences captures the vendor/type/tag universe in one row. The
// discovery agent uses these as quick "what's the shape of this catalog"
// context before requesting samples.
func (uc *ShopifyV2UseCase) dumpShopReferences(ctx context.Context, tenantID, shop, token string, res *DumpToStagingResult) error {
	refs, err := uc.client.ShopReferences(ctx, shop, token)
	if err != nil {
		return fmt.Errorf("references: %w", err)
	}
	payload, _ := json.Marshal(refs)
	if err := uc.staging.UpsertRaw(ctx, tenantID, "metadata", "shop_references", payload); err != nil {
		return fmt.Errorf("staging references: %w", err)
	}
	res.VendorCount = len(refs.ProductVendors)
	res.ProductTypeCount = len(refs.ProductTypes)
	res.TagCount = len(refs.ProductTags)
	return nil
}

// runBulkProductsAndWait fires a new bulk-op and polls until it's terminal.
// If a previous op is still RUNNING for this app+shop, Shopify rejects the
// new RunQuery — we transparently switch to polling that one instead.
func (uc *ShopifyV2UseCase) runBulkProductsAndWait(ctx context.Context, shop, token string) (*shopify.BulkOperation, error) {
	if _, err := uc.client.RunBulkProductsQuery(ctx, shop, token); err != nil {
		// "already in progress" — fall through to polling current op.
		if !strings.Contains(strings.ToLower(err.Error()), "already in progress") {
			return nil, err
		}
		uc.log.Info("shopify_v2_bulk_op_already_running", "shop", shop)
	}

	deadline := time.Now().Add(bulkOpMaxWait)
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("bulk op exceeded %s", bulkOpMaxWait)
		}
		op, err := uc.client.GetCurrentBulkOperation(ctx, shop, token)
		if err != nil {
			return nil, fmt.Errorf("poll: %w", err)
		}
		if op == nil {
			return nil, fmt.Errorf("no current bulk op (race condition?)")
		}
		switch op.Status {
		case shopify.BulkOpCompleted:
			return op, nil
		case shopify.BulkOpFailed, shopify.BulkOpCanceled, shopify.BulkOpExpired:
			return nil, fmt.Errorf("bulk op terminal status %s (errorCode=%s)", op.Status, op.ErrorCode)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(bulkPollInterval):
		}
	}
}

// productAccum holds a partial product reconstruction during JSONL streaming.
// Children (variants/images/metafields/collections) arrive as separate lines
// with __parentId pointing at the product, so we bucket them here until the
// parent product line itself appears.
type productAccum struct {
	productPayload json.RawMessage
	variants       []json.RawMessage
	images         []json.RawMessage
	metafields     []json.RawMessage
	collections    []json.RawMessage
}

// streamJSONLToStaging parses the bulk-op JSONL and writes one staging row per
// top-level product, with all children (variants/images/metafields/collections)
// merged in under canonical "_v2_*" keys.
//
// Algorithm: two-pass over the JSONL.
//   Pass 1 — bucket every line: top-level Product rows go into a map keyed by
//            product GID (with insertion order preserved); child rows go into
//            another map keyed by their __parentId (the product GID).
//   Pass 2 — for each collected product, merge its children and upsert into
//            staging.
//
// Why two-pass: Shopify's Bulk Operations spec promises that a product and its
// children appear contiguously, but does NOT guarantee whether children precede
// or follow their parent. Empirically (Admin API 2026-04, dev-store seed) the
// order is parent-first; the original implementation assumed children-first
// and silently dropped all children. Two-pass is robust to either order.
//
// Memory note: the entire JSONL is held in RAM during pass 1. Bulk operations
// are capped server-side around 1M-5M lines; for a 100k-product catalog with
// ~3 children each, this is roughly 300MB of raw bytes. Acceptable for our
// deployment (Railway gives at least 1GB on the smallest tier). If we ever
// need to support truly massive catalogs we'll switch to a temp-file two-pass.
func (uc *ShopifyV2UseCase) streamJSONLToStaging(ctx context.Context, tenantID, jsonlURL string) (int, error) {
	body, err := uc.client.FetchBulkJSONL(ctx, jsonlURL)
	if err != nil {
		return 0, err
	}
	defer body.Close()

	scanner := shopify.ScanBulkJSONL(body)

	products := make(map[string]json.RawMessage)
	productOrder := make([]string, 0, 256) // preserve order for deterministic upserts
	children := make(map[string][]childRow)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var head struct {
			ID       string `json:"id"`
			ParentID string `json:"__parentId"`
		}
		if err := json.Unmarshal(line, &head); err != nil {
			uc.log.Info("shopify_v2_jsonl_parse_warning", "error", err.Error())
			continue
		}

		// Copy — scanner.Bytes() is reused on next Scan.
		row := make(json.RawMessage, len(line))
		copy(row, line)

		if head.ParentID == "" {
			if _, exists := products[head.ID]; !exists {
				productOrder = append(productOrder, head.ID)
			}
			products[head.ID] = row
			continue
		}

		// Child. Classify by GID prefix. Metafields requested without `id`
		// land here with empty head.ID — we still know it's a metafield by
		// elimination (it's a child of a Product GID and has no GID of its
		// own). The `Metafield` case below still wins when Shopify decides
		// to populate id in future API versions.
		kind := classifyChild(head.ID)
		children[head.ParentID] = append(children[head.ParentID], childRow{kind: kind, row: row})
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scanner: %w", err)
	}

	count := 0
	orphans := 0
	for _, pid := range productOrder {
		a := &productAccum{productPayload: products[pid]}
		for _, c := range children[pid] {
			switch c.kind {
			case childKindVariant:
				a.variants = append(a.variants, c.row)
			case childKindImage:
				a.images = append(a.images, c.row)
			case childKindMetafield:
				a.metafields = append(a.metafields, c.row)
			case childKindCollection:
				a.collections = append(a.collections, c.row)
			default:
				// Unknown child types we still keep — bucket into metafields
				// as a permissive default so we never drop data.
				a.metafields = append(a.metafields, c.row)
			}
		}
		merged, err := mergeChildrenIntoProduct(a)
		if err != nil {
			return count, fmt.Errorf("merge children for %s: %w", pid, err)
		}
		if err := uc.staging.UpsertRaw(ctx, tenantID, "product", pid, merged); err != nil {
			return count, fmt.Errorf("staging upsert %s: %w", pid, err)
		}
		delete(children, pid)
		count++
	}
	// Anything left in `children` is referencing a parent we never saw —
	// orphans. Log so we notice if the bulk op is truncated or misordered.
	for parentID, rows := range children {
		orphans += len(rows)
		uc.log.Info("shopify_v2_jsonl_orphan_children",
			"parent_id", parentID,
			"count", len(rows),
		)
	}
	uc.log.Info("shopify_v2_jsonl_summary",
		"products", count,
		"orphans", orphans,
	)
	return count, nil
}

// childRow tags a buffered child JSONL line with its inferred kind so pass 2
// doesn't have to re-classify by GID prefix.
type childRow struct {
	kind childKind
	row  json.RawMessage
}

type childKind int

const (
	childKindUnknown childKind = iota
	childKindVariant
	childKindImage
	childKindMetafield
	childKindCollection
)

// classifyChild infers the type of a JSONL child row from its GID. Metafields
// can arrive with empty ID (we don't request `id` in the metafield selection);
// callers handle that case via the parent-context fallback.
func classifyChild(gid string) childKind {
	switch {
	case strings.Contains(gid, "/ProductVariant/"):
		return childKindVariant
	case strings.Contains(gid, "/MediaImage/"), strings.Contains(gid, "/ProductImage/"):
		return childKindImage
	case strings.Contains(gid, "/Metafield/"):
		return childKindMetafield
	case strings.Contains(gid, "/Collection/"):
		return childKindCollection
	default:
		return childKindUnknown
	}
}

// mergeChildrenIntoProduct splices accumulated child rows into the product
// payload under canonical keys. Even though the bulk query already returns
// nested edges for variants/images/metafields/collections inline, those
// nested arrays are paginated under the hood; the JSONL emission flattens
// them into separate lines. We re-attach under "_v2_*" keys so downstream
// code can prefer them when present without colliding with the top-level
// fields we kept in the GraphQL query.
func mergeChildrenIntoProduct(a *productAccum) (json.RawMessage, error) {
	var product map[string]json.RawMessage
	if err := json.Unmarshal(a.productPayload, &product); err != nil {
		return nil, err
	}
	if len(a.variants) > 0 {
		raw, _ := json.Marshal(a.variants)
		product["_v2_variants"] = raw
	}
	if len(a.images) > 0 {
		raw, _ := json.Marshal(a.images)
		product["_v2_images"] = raw
	}
	if len(a.metafields) > 0 {
		raw, _ := json.Marshal(a.metafields)
		product["_v2_metafields"] = raw
	}
	if len(a.collections) > 0 {
		raw, _ := json.Marshal(a.collections)
		product["_v2_collections"] = raw
	}
	return json.Marshal(product)
}

// countMenuItems totals all nodes in a (possibly-nested) menu tree.
func countMenuItems(items []shopify.NavigationMenuItem) int {
	total := len(items)
	for _, it := range items {
		total += countMenuItems(it.Items)
	}
	return total
}
