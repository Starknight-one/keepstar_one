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
	client        *shopify.Client
	integrations  ports.IntegrationsPort
	catalog       ports.AdminCatalogPort // tenant CRUD for install-flow auto-provision
	inboxIngester *ShopifyIngester       // 6-step catalog flow (inbox → discovery_v2 → apply_v2)
	onInstallDone InstallCompletionHook  // fired after install-flow OAuth; nil-safe
	publicURL     string
	log           *logger.Logger
}

func NewShopifyV2UseCase(
	client *shopify.Client,
	integrations ports.IntegrationsPort,
	catalog ports.AdminCatalogPort,
	publicURL string,
	log *logger.Logger,
) *ShopifyV2UseCase {
	return &ShopifyV2UseCase{
		client:       client,
		integrations: integrations,
		catalog:      catalog,
		publicURL:    publicURL,
		log:          log,
	}
}

// SetInboxIngester wires the 6-step catalog flow. Required for runInitialIngest
// and HandleWebhook to function; main.go calls this immediately after
// constructing the ingester. If left nil, the UC will panic on first ingest
// call rather than silently absorb the data.
func (uc *ShopifyV2UseCase) SetInboxIngester(ing *ShopifyIngester) { uc.inboxIngester = ing }

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

// runInitialIngest — background after Connect: bulk-pulls the Shopify catalog,
// writes raw payloads into catalog.inbox_items, and triggers discovery_v2 +
// apply_v2 through the orchestrator. Status flips: syncing → connected on
// success, syncing → error on failure.
func (uc *ShopifyV2UseCase) runInitialIngest(integrationID, tenantID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
	defer cancel()

	uc.log.Info("shopify_v2_initial_ingest_started", "integration_id", integrationID)

	if uc.inboxIngester == nil {
		uc.log.Error("shopify_v2_inbox_ingester_not_wired", "integration_id", integrationID)
		_ = uc.integrations.UpdateStatus(ctx, integrationID, domain.IntegrationStatusError, "inbox ingester not wired")
		return
	}

	uc.runInitialIngestNewFlow(ctx, integrationID, tenantID)
}

// runInitialIngestNewFlow: bulk-pull products → parse JSONL → inbox.WriteFromShopifyBulk
// → orchestrator.ManualSync (which cascades to discovery_v2 + apply_v2). discovery_v2
// reads samples straight from inbox via its own tools — no separate metafield-defs /
// navigation / shop-refs dumps required.
func (uc *ShopifyV2UseCase) runInitialIngestNewFlow(ctx context.Context, integrationID, tenantID string) {
	integ, err := uc.integrations.Get(ctx, tenantID, integrationID)
	if err != nil {
		uc.log.Error("shopify_v2_new_flow_lookup_failed", "integration_id", integrationID, "error", err)
		_ = uc.integrations.UpdateStatus(ctx, integrationID, domain.IntegrationStatusError, err.Error())
		return
	}
	shop := integ.ExternalID
	token := integ.Credentials
	if shop == "" || token == "" {
		err := fmt.Errorf("integration not connected (missing shop or token)")
		uc.log.Error("shopify_v2_new_flow_token_missing", "integration_id", integrationID)
		_ = uc.integrations.UpdateStatus(ctx, integrationID, domain.IntegrationStatusError, err.Error())
		return
	}

	items, err := uc.bulkPullProductsAsInboxItems(ctx, shop, token)
	if err != nil {
		uc.log.Error("shopify_v2_new_flow_bulk_pull_failed", "integration_id", integrationID, "error", err)
		_ = uc.integrations.UpdateStatus(ctx, integrationID, domain.IntegrationStatusError, err.Error())
		return
	}
	uc.log.Info("shopify_v2_new_flow_bulk_pull_done", "integration_id", integrationID, "items", len(items))

	res, err := uc.inboxIngester.IngestBulkItems(ctx, tenantID, items)
	if err != nil {
		uc.log.Error("shopify_v2_new_flow_ingest_failed", "integration_id", integrationID, "error", err)
		_ = uc.integrations.UpdateStatus(ctx, integrationID, domain.IntegrationStatusError, err.Error())
		return
	}
	applied := 0
	if res != nil && res.Apply != nil {
		applied = res.Apply.Applied
	}
	uc.log.Info("shopify_v2_initial_ingest_completed",
		"integration_id", integrationID, "items_in_inbox", len(items), "products_applied", applied)
	_ = uc.integrations.UpdateStatus(ctx, integrationID, domain.IntegrationStatusConnected, "")
}

// bulkPullProductsAsInboxItems runs the Shopify bulk-op and parses the
// resulting JSONL into a flat list of products with their children
// (variants, images, metafields, collections) merged under the parent.
func (uc *ShopifyV2UseCase) bulkPullProductsAsInboxItems(ctx context.Context, shop, token string) ([]ShopifyItem, error) {
	op, err := uc.runBulkProductsAndWait(ctx, shop, token)
	if err != nil {
		return nil, fmt.Errorf("bulk op: %w", err)
	}
	if op.URL == "" {
		return nil, fmt.Errorf("bulk op completed with empty URL")
	}

	body, err := uc.client.FetchBulkJSONL(ctx, op.URL)
	if err != nil {
		return nil, fmt.Errorf("fetch jsonl: %w", err)
	}
	defer body.Close()

	scanner := shopify.ScanBulkJSONL(body)
	products := make(map[string]json.RawMessage)
	productOrder := make([]string, 0, 256)
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
		row := make(json.RawMessage, len(line))
		copy(row, line)

		if head.ParentID == "" {
			if _, exists := products[head.ID]; !exists {
				productOrder = append(productOrder, head.ID)
			}
			products[head.ID] = row
			continue
		}
		kind := classifyChild(head.ID)
		children[head.ParentID] = append(children[head.ParentID], childRow{kind: kind, row: row})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner: %w", err)
	}

	items := make([]ShopifyItem, 0, len(productOrder))
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
				a.metafields = append(a.metafields, c.row)
			}
		}
		merged, err := mergeChildrenIntoProduct(a)
		if err != nil {
			return items, fmt.Errorf("merge children for %s: %w", pid, err)
		}
		items = append(items, ShopifyItem{GID: pid, Raw: merged})
	}
	return items, nil
}

// HandleWebhook dispatches verified Shopify webhook deliveries. Caller has
// validated HMAC; body is the raw bytes.
func (uc *ShopifyV2UseCase) HandleWebhook(ctx context.Context, topic, shopDomain string, body []byte) error {
	integration, err := uc.integrations.GetByShopDomain(ctx, shopDomain)
	if err != nil {
		return fmt.Errorf("lookup integration: %w", err)
	}
	if uc.inboxIngester == nil {
		uc.log.Error("shopify_v2_webhook_ingester_not_wired", "shop", shopDomain, "topic", topic)
		return fmt.Errorf("inbox ingester not wired")
	}
	switch topic {
	case "products/create", "products/update":
		// Orchestrator handles rate-limiting (≤1 apply/day) and routes the
		// payload through inbox → apply_v2. Shopify webhook bodies carry the
		// product id as a numeric Admin REST id; we lift it back to the GID
		// form the rest of the pipeline expects.
		var probe struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(body, &probe); err != nil || probe.ID == 0 {
			uc.log.Warn("shopify_v2_webhook_parse_id_failed", "shop", shopDomain, "error", err)
			return nil
		}
		gid := fmt.Sprintf("gid://shopify/Product/%d", probe.ID)
		verb := "updated"
		if topic == "products/create" {
			verb = "created"
		}
		_, err := uc.inboxIngester.IngestSingleWebhook(ctx, integration.TenantID, gid, verb, body)
		return err
	case "products/delete":
		// TODO: surface deletion through the new flow. For now log and ignore —
		// catalog won't auto-clean removed listings until a manual sync runs.
		uc.log.Info("shopify_v2_webhook_delete_not_routed", "shop", shopDomain)
		return nil
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

