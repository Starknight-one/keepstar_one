// smoke_catalog_v2 — end-to-end smoke test for the new 6-step catalog flow.
//
// What it does:
//   1. Creates a fresh tenant `smoke-cosmetics-<ts>`.
//   2. Seeds 10 synthetic Shopify-shaped cosmetics products into
//      catalog.inbox_items.
//   3. Runs DiscoveryV2.Discover (real Anthropic Sonnet 4.6 call, ~$0.40-1.00).
//   4. Inspects the produced mapping artifact, agent_runs row, and
//      tenant_action_log entries.
//   5. Runs ApplyV2.ApplyForTenant — writes master_products + master_cosmetics
//      + slim catalog.products rows.
//   6. Prints final counts + sample data.
//
// Survives re-runs: passes -keep to retain the test tenant for inspection,
// or -wipe to drop it after.
//
// Usage:
//   ANTHROPIC_API_KEY=... DATABASE_URL=... \
//   go run ./cmd/smoke_catalog_v2 [-keep] [-wipe]
package main

import (
	"context"
	"encoding/json"
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

func main() {
	keep := flag.Bool("keep", true, "keep the test tenant after run (default true; pass -wipe to delete)")
	wipe := flag.Bool("wipe", false, "wipe the test tenant after run instead of keeping")
	flag.Parse()

	// godotenv-load .env from repo root (mirrors how other cmd tools work).
	_ = godotenv.Load("/Users/starknight/Keepstar_project/Keepstar_one_ultra/project_admin/backend/.env")

	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL required (load .env or export)")
	}
	if cfg.AnthropicAPIKey == "" {
		log.Fatal("ANTHROPIC_API_KEY required (Sonnet 4.6 access)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	lg := logger.New("info")
	dbClient, err := postgres.NewClient(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer dbClient.Close()

	// Make sure new tables exist (idempotent).
	if err := dbClient.RunCatalogMigrations(ctx); err != nil {
		log.Fatalf("catalog migrations: %v", err)
	}
	if err := dbClient.RunAdminMigrations(ctx); err != nil {
		log.Fatalf("admin migrations: %v", err)
	}

	// --- Step 1: create test tenant ---
	tenantSlug := fmt.Sprintf("smoke-cosmetics-%d", time.Now().Unix())
	var tenantID string
	err = dbClient.Pool().QueryRow(ctx, `
		INSERT INTO catalog.tenants (slug, name, type)
		VALUES ($1, $2, 'retailer')
		RETURNING id::text
	`, tenantSlug, "Smoke Cosmetics").Scan(&tenantID)
	if err != nil {
		log.Fatalf("create test tenant: %v", err)
	}
	log.Printf("[1/6] created tenant slug=%s id=%s", tenantSlug, tenantID)

	// --- Step 2: seed inbox with 10 synthetic cosmetics products ---
	inboxAdapter := postgres.NewInboxAdapter(dbClient, lg)
	actionLog := postgres.NewTenantActionLogAdapter(dbClient, lg)
	inboxUC := usecases.NewInboxUseCase(inboxAdapter, actionLog, lg)

	items := buildSyntheticCosmeticsBatch(10)
	res, err := inboxUC.WriteFromShopifyBulk(ctx, tenantID, items)
	if err != nil {
		log.Fatalf("seed inbox: %v", err)
	}
	log.Printf("[2/6] seeded inbox: total=%d inserted=%d unchanged=%d errors=%d",
		res.Total, res.Inserted, res.Unchanged, res.Errors)

	// --- Step 3: wire discovery_v2 + apply_v2 + writer ---
	agentRunsAdapter := postgres.NewAgentRunsAdapter(dbClient, lg)
	artifactAdapter := postgres.NewMappingArtifactV2Adapter(dbClient, lg)
	writerAdapter := postgres.NewCatalogV2WriterAdapter(dbClient, lg)
	agentClient := anthropicAdapter.NewAgentClient(cfg.AnthropicAPIKey, "claude-sonnet-4-6")

	discovery := usecases.NewDiscoveryV2(agentClient, inboxAdapter, artifactAdapter, actionLog, agentRunsAdapter, lg)

	// --- Step 4: run discovery (REAL Anthropic call) ---
	log.Printf("[3/6] running discovery_v2 (Anthropic Sonnet 4.6, $5 budget)…")
	t0 := time.Now()
	artifact, err := discovery.Discover(ctx, tenantID, "first_install", nil)
	dur := time.Since(t0)
	if err != nil {
		log.Printf("    discovery returned error: %v", err)
	}
	if artifact == nil {
		log.Printf("    no artifact produced")
	} else {
		body, _ := json.MarshalIndent(artifact, "", "  ")
		log.Printf("    artifact: vertical=%s rules=%d notes=%q",
			artifact.Vertical, len(artifact.FieldMap), artifact.Notes)
		fmt.Println(string(body))
	}
	log.Printf("[4/6] discovery_v2 finished in %s", dur)

	// --- Step 5: inspect agent_runs row ---
	logAgentRunsInline(ctx, dbClient, tenantID)

	// --- Step 6: apply (if discovery committed) ---
	if artifact != nil {
		applyUC := usecases.NewApplyV2(inboxAdapter, artifactAdapter, writerAdapter, actionLog, discovery, lg)
		log.Printf("[5/6] running apply_v2…")
		appRes, err := applyUC.ApplyForTenant(ctx, tenantID)
		if err != nil {
			log.Printf("    apply returned error: %v", err)
		}
		if appRes != nil {
			log.Printf("    apply: total=%d applied=%d errors=%d misses=%d first_err=%q",
				appRes.Total, appRes.Applied, appRes.Errors, appRes.MappingMisses, appRes.FirstError)
		}
	} else {
		log.Printf("[5/6] skipping apply — no artifact")
	}

	// --- Step 6: final DB inspection ---
	log.Printf("[6/6] final state:")
	dumpCounts(ctx, dbClient, tenantID)
	dumpSampleMaster(ctx, dbClient, tenantID)
	dumpActionLog(ctx, dbClient, tenantID)

	if *wipe {
		log.Printf("wiping test tenant…")
		_, _ = dbClient.Pool().Exec(ctx, `DELETE FROM catalog.tenants WHERE id = $1`, tenantID)
	} else if *keep {
		log.Printf("test tenant kept (slug=%s id=%s); pass -wipe to delete next time", tenantSlug, tenantID)
	}
}

// --- helpers ---

func buildSyntheticCosmeticsBatch(n int) []usecases.ShopifyItem {
	// Realistic Shopify-bulk-ish JSONL payloads. Keys agent will see via
	// list_fields: title, vendor, productType, tags, options, variants,
	// body_html, handle, id.
	samples := []map[string]any{
		{
			"id":         "gid://shopify/Product/100001",
			"title":      "Hyaluronic Acid 2% + B5 Serum",
			"vendor":     "The Ordinary",
			"productType": "Serum",
			"handle":     "hyaluronic-acid-serum",
			"tags":       []string{"hydrating", "all skin types", "hyaluronic acid", "morning routine"},
			"body_html":  "Multi-molecular hyaluronic acid serum. Hydrates and plumps skin.",
			"options":    []map[string]any{{"name": "Size", "values": []string{"30 ml"}}},
			"variants": []map[string]any{
				{"sku": "ORD-HA-30", "price": "12.99", "inventory_quantity": 200, "weight": 30, "weight_unit": "g"},
			},
			"metafields": map[string]any{
				"skincare": map[string]any{
					"skin_type":       []string{"oily", "combination", "all"},
					"concern":         []string{"dehydration"},
					"key_ingredients": []string{"hyaluronic acid", "vitamin b5"},
					"routine_step":    "serum",
				},
			},
		},
		{
			"id":         "gid://shopify/Product/100002",
			"title":      "Niacinamide 10% + Zinc 1% Serum",
			"vendor":     "The Ordinary",
			"productType": "Serum",
			"handle":     "niacinamide-zinc-serum",
			"tags":       []string{"oily skin", "blemish prone", "niacinamide", "pore minimizing"},
			"body_html":  "High-strength niacinamide for blemish-prone skin.",
			"variants": []map[string]any{
				{"sku": "ORD-NIA-30", "price": "11.99", "inventory_quantity": 150},
			},
			"metafields": map[string]any{
				"skincare": map[string]any{
					"skin_type":       []string{"oily", "blemish prone"},
					"concern":         []string{"blemishes", "enlarged pores"},
					"key_ingredients": []string{"niacinamide", "zinc"},
					"routine_step":    "serum",
				},
			},
		},
		{
			"id":         "gid://shopify/Product/100003",
			"title":      "Squalane Cleanser",
			"vendor":     "The Ordinary",
			"productType": "Cleanser",
			"handle":     "squalane-cleanser",
			"tags":       []string{"cleanser", "gentle", "all skin types"},
			"body_html":  "Hydrating squalane-based cleanser.",
			"variants": []map[string]any{
				{"sku": "ORD-SQ-50", "price": "14.99", "inventory_quantity": 80},
			},
			"metafields": map[string]any{
				"skincare": map[string]any{
					"skin_type":       []string{"dry", "sensitive", "all"},
					"concern":         []string{"dryness"},
					"key_ingredients": []string{"squalane"},
					"routine_step":    "cleanser",
					"routine_time":    "AM/PM",
				},
			},
		},
		{
			"id":         "gid://shopify/Product/100004",
			"title":      "Retinol 0.5% in Squalane",
			"vendor":     "The Ordinary",
			"productType": "Serum",
			"handle":     "retinol-05",
			"tags":       []string{"retinol", "anti-aging", "evening"},
			"body_html":  "Mid-strength retinol for fine lines.",
			"variants": []map[string]any{
				{"sku": "ORD-RET-30", "price": "10.99", "inventory_quantity": 120},
			},
			"metafields": map[string]any{
				"skincare": map[string]any{
					"skin_type":       []string{"all"},
					"concern":         []string{"fine lines", "wrinkles"},
					"key_ingredients": []string{"retinol", "squalane"},
					"routine_step":    "serum",
					"routine_time":    "PM",
				},
			},
		},
		{
			"id":         "gid://shopify/Product/100005",
			"title":      "Vitamin C Suspension 23% + HA Spheres 2%",
			"vendor":     "The Ordinary",
			"productType": "Serum",
			"handle":     "vitamin-c-23",
			"tags":       []string{"vitamin c", "brightening", "antioxidant"},
			"body_html":  "Anhydrous vitamin C suspension.",
			"variants": []map[string]any{
				{"sku": "ORD-VC-30", "price": "5.80", "inventory_quantity": 200},
			},
			"metafields": map[string]any{
				"skincare": map[string]any{
					"skin_type":       []string{"all"},
					"concern":         []string{"dullness", "uneven tone"},
					"key_ingredients": []string{"vitamin c"},
					"routine_step":    "serum",
				},
			},
		},
		{
			"id":         "gid://shopify/Product/100006",
			"title":      "AHA 30% + BHA 2% Peeling Solution",
			"vendor":     "The Ordinary",
			"productType": "Treatment",
			"handle":     "aha-bha-peeling",
			"tags":       []string{"exfoliation", "weekly", "advanced"},
			"body_html":  "Strong weekly exfoliant.",
			"variants": []map[string]any{
				{"sku": "ORD-AHABHA-30", "price": "7.20", "inventory_quantity": 90},
			},
			"metafields": map[string]any{
				"skincare": map[string]any{
					"skin_type":       []string{"non-sensitive"},
					"concern":         []string{"texture", "dullness", "blemishes"},
					"key_ingredients": []string{"glycolic acid", "lactic acid", "salicylic acid"},
					"routine_step":    "treatment",
					"routine_time":    "PM",
				},
			},
		},
		{
			"id":         "gid://shopify/Product/100007",
			"title":      "Natural Moisturizing Factors + HA",
			"vendor":     "The Ordinary",
			"productType": "Moisturizer",
			"handle":     "nmf-ha",
			"tags":       []string{"moisturizer", "daily", "hydrating"},
			"body_html":  "Daily hydrating moisturizer with NMFs.",
			"variants": []map[string]any{
				{"sku": "ORD-NMF-30", "price": "8.99", "inventory_quantity": 160, "weight": 30, "weight_unit": "g"},
			},
			"metafields": map[string]any{
				"skincare": map[string]any{
					"skin_type":       []string{"all", "dry"},
					"concern":         []string{"dehydration"},
					"key_ingredients": []string{"hyaluronic acid", "amino acids", "ceramides"},
					"routine_step":    "moisturizer",
				},
			},
		},
		{
			"id":         "gid://shopify/Product/100008",
			"title":      "Buffet™ Multi-Technology Peptide Serum",
			"vendor":     "The Ordinary",
			"productType": "Serum",
			"handle":     "buffet",
			"tags":       []string{"peptides", "anti-aging", "advanced"},
			"body_html":  "Multi-peptide age-supporting serum.",
			"variants": []map[string]any{
				{"sku": "ORD-BUF-30", "price": "14.80", "inventory_quantity": 100},
			},
			"metafields": map[string]any{
				"skincare": map[string]any{
					"skin_type":       []string{"all", "mature"},
					"concern":         []string{"fine lines", "loss of elasticity"},
					"key_ingredients": []string{"peptides", "hyaluronic acid"},
					"routine_step":    "serum",
				},
			},
		},
		{
			"id":         "gid://shopify/Product/100009",
			"title":      "Caffeine Solution 5% + EGCG",
			"vendor":     "The Ordinary",
			"productType": "Eye Care",
			"handle":     "caffeine-solution",
			"tags":       []string{"eye care", "depuffing", "morning"},
			"body_html":  "Reduces puffiness and dark circles around eyes.",
			"variants": []map[string]any{
				{"sku": "ORD-CAF-30", "price": "7.99", "inventory_quantity": 140},
			},
			"metafields": map[string]any{
				"skincare": map[string]any{
					"skin_type":       []string{"all"},
					"concern":         []string{"puffiness", "dark circles"},
					"key_ingredients": []string{"caffeine", "egcg"},
					"target_area":     []string{"eye area"},
					"routine_step":    "treatment",
				},
			},
		},
		{
			"id":         "gid://shopify/Product/100010",
			"title":      "Glycolic Acid 7% Toning Solution",
			"vendor":     "The Ordinary",
			"productType": "Toner",
			"handle":     "glycolic-toner",
			"tags":       []string{"toner", "exfoliating", "daily"},
			"body_html":  "Gentle daily exfoliating toner.",
			"variants": []map[string]any{
				{"sku": "ORD-GLY-240", "price": "12.50", "inventory_quantity": 110},
			},
			"metafields": map[string]any{
				"skincare": map[string]any{
					"skin_type":       []string{"normal", "oily"},
					"concern":         []string{"texture", "dullness"},
					"key_ingredients": []string{"glycolic acid"},
					"routine_step":    "toner",
					"routine_time":    "PM",
				},
			},
		},
	}
	if n > len(samples) {
		n = len(samples)
	}
	out := make([]usecases.ShopifyItem, 0, n)
	for i := 0; i < n; i++ {
		raw, _ := json.Marshal(samples[i])
		out = append(out, usecases.ShopifyItem{
			GID: samples[i]["id"].(string),
			Raw: raw,
		})
	}
	return out
}

// --- inspection helpers ---

func logAgentRunsInline(ctx context.Context, db *postgres.Client, tenantID string) {
	rows, err := db.Pool().Query(ctx, `
		SELECT id::text, trigger, started_at, finished_at, status, tokens_in, tokens_out, cost_usd,
		       COALESCE(jsonb_array_length(tools_called), 0) AS calls
		FROM admin.agent_runs
		WHERE tenant_id = $1
		ORDER BY started_at DESC
	`, tenantID)
	if err != nil {
		log.Printf("    agent_runs query: %v", err)
		return
	}
	defer rows.Close()
	fmt.Println()
	fmt.Println("--- agent_runs ---")
	for rows.Next() {
		var id, trigger, status string
		var startedAt time.Time
		var finishedAt *time.Time
		var inTok, outTok, calls int
		var cost float64
		if err := rows.Scan(&id, &trigger, &startedAt, &finishedAt, &status, &inTok, &outTok, &cost, &calls); err != nil {
			log.Printf("    scan: %v", err)
			continue
		}
		dur := "..."
		if finishedAt != nil {
			dur = finishedAt.Sub(startedAt).Truncate(time.Millisecond).String()
		}
		fmt.Printf("    %s  trigger=%s  status=%s  tools=%d  in=%d  out=%d  cost=$%.4f  dur=%s\n",
			id[:8], trigger, status, calls, inTok, outTok, cost, dur)
	}
}

func dumpCounts(ctx context.Context, db *postgres.Client, tenantID string) {
	var inboxTotal, inboxApplied, mp, mc, listings int
	_ = db.Pool().QueryRow(ctx, `SELECT COUNT(*), COUNT(applied_at) FROM catalog.inbox_items WHERE tenant_id=$1`, tenantID).Scan(&inboxTotal, &inboxApplied)
	_ = db.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM catalog.master_products WHERE owner_tenant_id=$1`, tenantID).Scan(&mp)
	_ = db.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM catalog.master_cosmetics mc
		JOIN catalog.master_products mp ON mp.id = mc.master_product_id
		WHERE mp.owner_tenant_id = $1
	`, tenantID).Scan(&mc)
	_ = db.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM catalog.products WHERE tenant_id=$1`, tenantID).Scan(&listings)
	fmt.Printf("    inbox_items:        %d (applied=%d)\n", inboxTotal, inboxApplied)
	fmt.Printf("    master_products:    %d\n", mp)
	fmt.Printf("    master_cosmetics:   %d\n", mc)
	fmt.Printf("    catalog.products:   %d\n", listings)
}

func dumpSampleMaster(ctx context.Context, db *postgres.Client, tenantID string) {
	rows, err := db.Pool().Query(ctx, `
		SELECT mp.name, mp.brand, mp.vertical,
		       COALESCE(mc.skin_type, '{}'),
		       COALESCE(mc.concern, '{}'),
		       COALESCE(mc.key_ingredients, '{}')
		FROM catalog.master_products mp
		LEFT JOIN catalog.master_cosmetics mc ON mc.master_product_id = mp.id
		WHERE mp.owner_tenant_id = $1
		ORDER BY mp.created_at
		LIMIT 3
	`, tenantID)
	if err != nil {
		return
	}
	defer rows.Close()
	fmt.Println()
	fmt.Println("--- sample masters ---")
	for rows.Next() {
		var name, brand, vertical string
		var skin, concern, keyIng []string
		_ = rows.Scan(&name, &brand, &vertical, &skin, &concern, &keyIng)
		fmt.Printf("    %s  brand=%s  vertical=%s\n", name, brand, vertical)
		fmt.Printf("      skin_type=%v\n", skin)
		fmt.Printf("      concern=%v\n", concern)
		fmt.Printf("      ingredients=%v\n", keyIng)
	}
}

func dumpActionLog(ctx context.Context, db *postgres.Client, tenantID string) {
	rows, err := db.Pool().Query(ctx, `
		SELECT action, status, occurred_at, payload
		FROM admin.tenant_action_log
		WHERE tenant_id = $1
		ORDER BY occurred_at ASC
	`, tenantID)
	if err != nil {
		return
	}
	defer rows.Close()
	fmt.Println()
	fmt.Println("--- action log ---")
	for rows.Next() {
		var action, status string
		var occurredAt time.Time
		var payload []byte
		_ = rows.Scan(&action, &status, &occurredAt, &payload)
		fmt.Printf("    %s  %s/%s  %s\n", occurredAt.Format("15:04:05"), action, status, string(payload))
	}
}

// Compile-time shut-up for unused imports in case build cuts a path:
var _ = domain.InboxItem{}
var _ = os.Stdout
