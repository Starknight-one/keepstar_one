// seed_test_run — first real-data run of the v2 catalog flow: takes the
// Sephora seed CSV (docs/seeds/tenant_A/sephora_products.csv, ~8.5k rows)
// and pushes it through CSVIngester → inbox → discovery_v2 → apply_v2.
//
// Creates a fresh tenant with a timestamped slug so it can't collide with
// or overwrite any existing catalog data. master_products is IMMUTABLE on
// bind (per apply_v2 invariant), so even if our rows match existing
// masters by SKU/GTIN/normalized_match_key, nothing pre-existing gets
// modified.
//
// Usage:
//
//	cd project_admin/backend && go run ./cmd/seed_test_run
//
// Default CSV path is repo-absolute. Override with -csv <path>.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"

	anthropicAdapter "keepstar-admin/internal/adapters/anthropic"
	"keepstar-admin/internal/adapters/postgres"
	"keepstar-admin/internal/config"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

const (
	repoRoot   = "/Users/starknight/Keepstar_project/Keepstar_one_ultra"
	defaultCSV = repoRoot + "/docs/seeds/tenant_A/sephora_products.csv"
)

func main() {
	csvPath := flag.String("csv", defaultCSV, "path to seed CSV")
	naturalID := flag.String("natural-id", "product_id", "CSV column to use as inbox_items.external_id")
	flag.Parse()

	_ = godotenv.Load(repoRoot + "/project_admin/backend/.env")

	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL required")
	}
	if cfg.AnthropicAPIKey == "" {
		log.Fatal("ANTHROPIC_API_KEY required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	lg := logger.New("info")
	db, err := postgres.NewClient(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	if err := db.RunCatalogMigrations(ctx); err != nil {
		log.Fatalf("catalog migrations: %v", err)
	}
	if err := db.RunAdminMigrations(ctx); err != nil {
		log.Fatalf("admin migrations: %v", err)
	}

	// 1. Fresh tenant — slug timestamped so no collision with anything.
	ts := time.Now().Unix()
	slug := fmt.Sprintf("seed-sephora-%d", ts)
	var tenantID string
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO catalog.tenants (slug, name, type) VALUES ($1, $2, 'retailer')
		RETURNING id::text
	`, slug, slug).Scan(&tenantID)
	if err != nil {
		log.Fatalf("create tenant: %v", err)
	}
	log.Printf("== tenant created: slug=%s  id=%s", slug, tenantID)

	// 2. DI — same wiring as cmd/server main.go.
	inboxAdapter := postgres.NewInboxAdapter(db, lg)
	digestAdapter := postgres.NewMasterDigestAdapter(db, lg)
	actionLog := postgres.NewTenantActionLogAdapter(db, lg)
	agentRuns := postgres.NewAgentRunsAdapter(db, lg)
	artifact := postgres.NewMappingArtifactV2Adapter(db, lg)
	writer := postgres.NewCatalogV2WriterAdapter(db, lg)
	rateLimit := postgres.NewApplyRateLimitAdapter(db, lg)
	agentClient := anthropicAdapter.NewAgentClient(cfg.AnthropicAPIKey, "claude-sonnet-4-6")

	inboxUC := usecases.NewInboxUseCase(inboxAdapter, actionLog, lg)
	discovery := usecases.NewDiscoveryV2(agentClient, inboxAdapter, digestAdapter, artifact, actionLog, agentRuns, lg)
	apply := usecases.NewApplyV2(inboxAdapter, artifact, writer, actionLog, discovery, lg)
	orch := usecases.NewUpdateOrchestrator(inboxUC, apply, discovery, actionLog, rateLimit, lg)
	csvIngester := usecases.NewCSVIngester(inboxUC, orch, lg)

	// 3. Open CSV and ingest.
	f, err := os.Open(*csvPath)
	if err != nil {
		log.Fatalf("open csv: %v", err)
	}
	defer f.Close()
	stat, _ := f.Stat()
	log.Printf("== ingesting %s (%.2f MB), natural_id_field=%q", *csvPath, float64(stat.Size())/1048576, *naturalID)

	t0 := time.Now()
	res, ingestErr := csvIngester.IngestReader(ctx, tenantID, domain.InboxSourceCSV, f, *naturalID)
	elapsed := time.Since(t0)

	if ingestErr != nil {
		log.Printf("INGEST RETURNED ERROR: %v", ingestErr)
	}

	fmt.Println()
	fmt.Println("================ INGEST RESULT ================")
	fmt.Printf("Wallclock elapsed: %s\n", elapsed)
	if res != nil && res.Inbox != nil {
		fmt.Printf("Inbox:    inserted=%d  unchanged=%d  errors=%d\n",
			res.Inbox.Inserted, res.Inbox.Unchanged, res.Inbox.Errors)
	}
	if res != nil && res.Apply != nil {
		fmt.Printf("Apply:    total=%d  applied=%d  errors=%d  mapping_misses=%d\n",
			res.Apply.Total, res.Apply.Applied, res.Apply.Errors, res.Apply.MappingMisses)
		if res.Apply.FirstError != "" {
			fmt.Printf("          first_error: %q\n", res.Apply.FirstError)
		}
	}

	// 4. Final DB state for this tenant.
	fmt.Println()
	fmt.Println("================ FINAL DB STATE ================")

	var inboxTotal, inboxUnapplied int
	_ = db.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM catalog.inbox_items WHERE tenant_id=$1`, tenantID).Scan(&inboxTotal)
	_ = db.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM catalog.inbox_items WHERE tenant_id=$1 AND applied_at IS NULL`, tenantID).Scan(&inboxUnapplied)
	fmt.Printf("inbox_items:           %d total, %d still unapplied\n", inboxTotal, inboxUnapplied)

	var mastersOwned, mastersLinked int
	_ = db.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM catalog.master_products WHERE owner_tenant_id=$1`, tenantID).Scan(&mastersOwned)
	_ = db.Pool().QueryRow(ctx, `
		SELECT COUNT(DISTINCT mp.id) FROM catalog.master_products mp
		JOIN catalog.products p ON p.master_product_id = mp.id
		WHERE p.tenant_id=$1
	`, tenantID).Scan(&mastersLinked)
	fmt.Printf("master_products:       %d owned by tenant, %d linked-via-listings (includes bind-to-existing)\n",
		mastersOwned, mastersLinked)

	var listings, listingsUnlinked int
	_ = db.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM catalog.products WHERE tenant_id=$1 AND deleted_at IS NULL`, tenantID).Scan(&listings)
	_ = db.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM catalog.products WHERE tenant_id=$1 AND deleted_at IS NULL AND master_product_id IS NULL`, tenantID).Scan(&listingsUnlinked)
	fmt.Printf("catalog.products:      %d listings, %d unlinked (no master_product_id)\n", listings, listingsUnlinked)

	var withTier2 int
	_ = db.Pool().QueryRow(ctx, `
		SELECT COUNT(DISTINCT mp.id) FROM catalog.master_products mp
		JOIN catalog.products p ON p.master_product_id = mp.id
		WHERE p.tenant_id=$1 AND mp.tier2 <> '{}'::jsonb
	`, tenantID).Scan(&withTier2)
	fmt.Printf("masters with tier2:    %d (typed per-vertical attributes)\n", withTier2)

	var withTier3 int
	_ = db.Pool().QueryRow(ctx, `
		SELECT COUNT(DISTINCT mp.id) FROM catalog.master_products mp
		JOIN catalog.products p ON p.master_product_id = mp.id
		WHERE p.tenant_id=$1 AND mp.tier3 <> '{}'::jsonb
	`, tenantID).Scan(&withTier3)
	fmt.Printf("masters with tier3:    %d (unknown/non-cosmetics attributes)\n", withTier3)

	fmt.Println()
	fmt.Println("Vertical distribution:")
	verticalRows, err := db.Pool().Query(ctx, `
		SELECT mp.vertical, COUNT(DISTINCT mp.id)
		FROM catalog.master_products mp
		JOIN catalog.products p ON p.master_product_id = mp.id
		WHERE p.tenant_id=$1 GROUP BY mp.vertical ORDER BY 2 DESC
	`, tenantID)
	if err == nil {
		for verticalRows.Next() {
			var v string
			var n int
			_ = verticalRows.Scan(&v, &n)
			fmt.Printf("  %-20s %d\n", v, n)
		}
		verticalRows.Close()
	}

	// 5. Discovery cost / tokens / duration.
	fmt.Println()
	fmt.Println("================ DISCOVERY RUN(S) ================")
	runRows, err := db.Pool().Query(ctx, `
		SELECT trigger, status, COALESCE(cost_usd,0), COALESCE(tokens_in,0), COALESCE(tokens_out,0),
		       EXTRACT(EPOCH FROM (COALESCE(finished_at, NOW()) - started_at)) AS sec
		FROM admin.agent_runs WHERE tenant_id=$1 ORDER BY started_at
	`, tenantID)
	totalCost := 0.0
	totalIn, totalOut := 0, 0
	if err == nil {
		for runRows.Next() {
			var trigger, status string
			var cost float64
			var tIn, tOut int
			var sec float64
			_ = runRows.Scan(&trigger, &status, &cost, &tIn, &tOut, &sec)
			fmt.Printf("  trigger=%-15s status=%-18s cost=$%.4f  tok_in=%d  tok_out=%d  dur=%.1fs\n",
				trigger, status, cost, tIn, tOut, sec)
			totalCost += cost
			totalIn += tIn
			totalOut += tOut
		}
		runRows.Close()
	}
	fmt.Printf("Total discovery cost:  $%.4f  (tokens_in=%d, tokens_out=%d)\n", totalCost, totalIn, totalOut)

	fmt.Println()
	fmt.Println("================ TENANT KEPT ================")
	fmt.Printf("slug=%s\n", slug)
	fmt.Printf("id=%s\n", tenantID)
	fmt.Println("Inspect via curator UI (/curator/tenants/<id>/inbox|action-log|agent-runs) or SQL.")
}
