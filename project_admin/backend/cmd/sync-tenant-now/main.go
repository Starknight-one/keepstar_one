// sync-tenant-now — one-shot trigger of the curator-driven pipeline for a
// single tenant. Replays what production does after an OAuth callback or a
// products/* webhook: DumpToStaging → harvester-lite → catalog.products.
//
// Used to manually pull dev-store catalog into our DB when webhooks miss
// (HMAC mismatch, deploy lag, etc.) without waiting for the next merchant
// catalog change. Decrypts the stored offline access token via secretbox,
// instantiates the same usecases prod admin uses, and runs them against the
// shared Neon DB.
//
// Usage:
//   ADMIN_ENCRYPTION_KEY=... DATABASE_URL=... \
//   go run ./cmd/sync-tenant-now -shop keepstar-neaqpan1.myshopify.com
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"keepstar-admin/internal/adapters/postgres"
	"keepstar-admin/internal/adapters/shopify"
	"keepstar-admin/internal/config"
	"keepstar-admin/internal/crypto/secretbox"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

func main() {
	shop := flag.String("shop", "", "shop domain (e.g. keepstar-neaqpan1.myshopify.com)")
	flag.Parse()
	if *shop == "" {
		log.Fatal("usage: sync-tenant-now -shop <shop>.myshopify.com")
	}
	if _, ok := shopify.ValidateShopDomain(*shop); !ok {
		log.Fatalf("invalid shop domain: %s", *shop)
	}

	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL required")
	}
	if cfg.EncryptionKey == "" {
		log.Fatal("ADMIN_ENCRYPTION_KEY required")
	}
	box, err := secretbox.NewFromBase64(cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("encryption key: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	lg := logger.New("info")

	dbClient, err := postgres.NewClient(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer dbClient.Close()

	// Apply migrations on entry — same set the prod admin server runs at boot.
	// Idempotent (CREATE ... IF NOT EXISTS, ALTER ... IF NOT EXISTS), so running
	// it from a CLI is safe and ensures the harvester always sees an up-to-date
	// schema when developing against a freshly-migrated branch.
	if err := dbClient.RunCatalogMigrations(ctx); err != nil {
		log.Fatalf("catalog migrations: %v", err)
	}

	integrationsAdapter := postgres.NewIntegrationsAdapter(dbClient, box)
	stagingAdapter := postgres.NewShopifyStagingAdapter(dbClient, lg)
	catalogAdapter := postgres.NewCatalogAdapter(dbClient, lg)
	candidatesAdapter := postgres.NewCandidatesAdapter(dbClient, lg)
	categoriesAdapter := postgres.NewCategoriesV2Adapter(dbClient, lg)

	integration, err := integrationsAdapter.GetByShopDomain(ctx, *shop)
	if err != nil {
		log.Fatalf("lookup integration by shop %s: %v", *shop, err)
	}
	log.Printf("integration tenant=%s shop=%s status=%s", integration.TenantID, integration.ExternalID, integration.Status)

	shopifyClient := shopify.NewClient(cfg.ShopifyAPIKey, cfg.ShopifyAPISecret, cfg.ShopifyAPIVersion, cfg.ShopifyScopes)
	v2 := usecases.NewShopifyV2UseCase(shopifyClient, integrationsAdapter, stagingAdapter, cfg.PublicBaseURL, lg)
	harvester := usecases.NewHarvesterLite(stagingAdapter, catalogAdapter, lg)
	harvester.SetSignals(candidatesAdapter, categoriesAdapter)
	v2.SetHarvester(harvester)

	log.Printf("=== DumpToStaging start ===")
	t0 := time.Now()
	dump, err := v2.DumpToStaging(ctx, integration.TenantID, integration.ID)
	if err != nil {
		log.Fatalf("dump: %v", err)
	}
	log.Printf("=== DumpToStaging done: %d products / %d collections / %d metafield_defs in %s ===",
		dump.ProductCount, dump.CollectionCount, dump.MetafieldDefs, time.Since(t0))

	log.Printf("=== HarvesterLite.RunForTenant start ===")
	t0 = time.Now()
	count, err := harvester.RunForTenant(ctx, integration.TenantID)
	if err != nil {
		log.Fatalf("harvester: %v", err)
	}
	log.Printf("=== HarvesterLite done: %d listings written in %s ===", count, time.Since(t0))
}
