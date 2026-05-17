// smoke_catalog_v2 — end-to-end smoke for the post-hardening 6-step catalog
// flow (inbox → discovery_v2 → apply_v2). Four synthetic tenants prove the
// match cascade, per-row vertical classification, and tier3 fallback all
// work together.
//
// Scenario layout (deterministic; same data every run):
//
//	Tenant A — cosm-a   — 10 cosmetic products
//	Tenant B — cosm-b   — 10 cosmetic products, 7 overlapping with A across
//	                      the three match cascade stages:
//	                        SKU (3 products), GTIN (2), norm-match-key (2)
//	                      The remaining 3 are new masters.
//	Tenant C — electro  — 10 laptop products, all new, vertical='electronics',
//	                      attributes routed to tier3 (no typed table).
//	Tenant D — mixed    — 10 mixed: 3 cosmetics (SKU-overlap with A),
//	                      3 electronics (SKU-overlap with C), 2 furniture (new),
//	                      2 ski (new). Multi-branch artifact required.
//
// Expected final state:
//
//	master_products:    27  (A=10, B-new=3, C=10, D-new=4)
//	catalog.products:   40  (10 each)
//	by vertical: cosmetics=13, electronics=10, furniture=2, ski=2
//	mapping_miss:       0   (forgiving fallbacks absorb edge cases)
//
// Usage:
//
//	go run ./cmd/smoke_catalog_v2 [-keep] [-wipe]
//
// Cost: ~$0.20-0.50 (4 discovery_v2 runs, Sonnet 4.6). Runs in ~3-5 min.
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

// tenantScenario bundles the seed payload + label for one synthetic tenant.
type tenantScenario struct {
	slug  string
	label string
	items []usecases.ShopifyItem
}

func main() {
	keep := flag.Bool("keep", true, "keep test tenants after run (default true; pass -wipe to delete)")
	wipe := flag.Bool("wipe", false, "wipe test tenants after run instead of keeping")
	flag.Parse()

	_ = godotenv.Load("/Users/starknight/Keepstar_project/Keepstar_one_ultra/project_admin/backend/.env")

	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL required")
	}
	if cfg.AnthropicAPIKey == "" {
		log.Fatal("ANTHROPIC_API_KEY required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
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

	ts := time.Now().Unix()
	scenarios := []tenantScenario{
		{slug: fmt.Sprintf("smoke-cosm-a-%d", ts), label: "A (cosmetics, baseline)", items: buildCosmeticsA()},
		{slug: fmt.Sprintf("smoke-cosm-b-%d", ts), label: "B (cosmetics, 7/10 overlap with A)", items: buildCosmeticsB()},
		{slug: fmt.Sprintf("smoke-electro-%d", ts), label: "C (electronics, all new)", items: buildElectronicsC()},
		{slug: fmt.Sprintf("smoke-mixed-%d", ts), label: "D (mixed 4 verticals)", items: buildMixedD()},
	}

	tenantIDs := make(map[string]string, 4)
	for _, s := range scenarios {
		var id string
		err := db.Pool().QueryRow(ctx, `
			INSERT INTO catalog.tenants (slug, name, type) VALUES ($1, $2, 'retailer')
			RETURNING id::text
		`, s.slug, s.slug).Scan(&id)
		if err != nil {
			log.Fatalf("create tenant %s: %v", s.slug, err)
		}
		tenantIDs[s.slug] = id
		log.Printf("== tenant %s id=%s items=%d", s.label, id, len(s.items))
	}

	inboxAdapter := postgres.NewInboxAdapter(db, lg)
	actionLog := postgres.NewTenantActionLogAdapter(db, lg)
	agentRuns := postgres.NewAgentRunsAdapter(db, lg)
	artifactAdapter := postgres.NewMappingArtifactV2Adapter(db, lg)
	writer := postgres.NewCatalogV2WriterAdapter(db, lg)
	rateLimit := postgres.NewApplyRateLimitAdapter(db, lg)
	agentClient := anthropicAdapter.NewAgentClient(cfg.AnthropicAPIKey, "claude-sonnet-4-6")

	inboxUC := usecases.NewInboxUseCase(inboxAdapter, actionLog, lg)
	discovery := usecases.NewDiscoveryV2(agentClient, inboxAdapter, artifactAdapter, actionLog, agentRuns, lg)
	apply := usecases.NewApplyV2(inboxAdapter, artifactAdapter, writer, actionLog, discovery, lg)
	_ = usecases.NewUpdateOrchestrator(inboxUC, apply, discovery, actionLog, rateLimit, lg)

	totalCost := 0.0
	for _, s := range scenarios {
		tenantID := tenantIDs[s.slug]
		fmt.Println()
		log.Printf(">>> processing %s", s.label)

		ingestRes, err := inboxUC.WriteFromShopifyBulk(ctx, tenantID, s.items)
		if err != nil {
			log.Printf("    inbox seed: %v", err)
			continue
		}
		log.Printf("    inbox seeded: inserted=%d unchanged=%d errors=%d",
			ingestRes.Inserted, ingestRes.Unchanged, ingestRes.Errors)

		t0 := time.Now()
		artifact, err := discovery.Discover(ctx, tenantID, "first_install", nil)
		dur := time.Since(t0)
		if err != nil {
			log.Printf("    discovery error: %v", err)
		}
		if artifact != nil {
			ruleCount := 0
			for _, b := range artifact.Branches {
				ruleCount += len(b.FieldMap)
			}
			log.Printf("    discovery: branches=%v rules=%d classify_rules=%d (%.1fs)",
				artifact.VerticalNames(), ruleCount, len(artifact.ClassifyRules), dur.Seconds())
		}

		cost := readLastAgentRunCost(ctx, db, tenantID)
		totalCost += cost
		log.Printf("    discovery cost: $%.4f (cumulative $%.4f)", cost, totalCost)

		appRes, err := apply.ApplyForTenant(ctx, tenantID)
		if err != nil {
			log.Printf("    apply error: %v", err)
		}
		if appRes != nil {
			log.Printf("    apply: total=%d applied=%d errors=%d misses=%d first_err=%q",
				appRes.Total, appRes.Applied, appRes.Errors, appRes.MappingMisses, appRes.FirstError)
		}
	}

	fmt.Println()
	fmt.Println("================ FINAL REPORT ================")
	dumpGlobalCounts(ctx, db, tenantIDs)
	dumpVerticalDistribution(ctx, db, tenantIDs)
	dumpMatchOutcomes(ctx, db, tenantIDs)
	fmt.Printf("Total discovery_v2 cost: $%.4f\n", totalCost)

	if *wipe && !*keep {
		log.Printf("wiping test tenants…")
		for _, id := range tenantIDs {
			_, _ = db.Pool().Exec(ctx, `DELETE FROM catalog.tenants WHERE id = $1`, id)
		}
	} else {
		log.Printf("test tenants kept; pass -wipe to delete next run")
	}

	_ = domain.InboxItem{}
	_ = os.Stdout
}

// ---------------- synthetic data ----------------

// Stable master library shared across A, B, D — keyed by product_id. Same
// (sku, gtin, brand, name) tuple → same master after apply. Variations
// across tenants exercise individual cascade stages.
type libProduct struct {
	id       string // shopify GID (different per tenant; assigned in builders)
	sku      string
	gtin     string
	brand    string
	name     string
	pType    string
	price    string
	// cosmetics-only attributes (drive cosmetics.* rules)
	skinType []string
	concern  []string
	ingreds  []string
	// generic extras the agent routes to tier3 for non-cosmetic verticals
	extras map[string]string
}

var cosmeticsLibrary = []libProduct{
	{sku: "ORD-HA-30", gtin: "00800001000001", brand: "The Ordinary", name: "Hyaluronic Acid 2% + B5", pType: "Serum", price: "12.99",
		skinType: []string{"oily", "combination"}, concern: []string{"dehydration"}, ingreds: []string{"hyaluronic acid", "vitamin b5"}},
	{sku: "ORD-NIA-30", gtin: "00800001000002", brand: "The Ordinary", name: "Niacinamide 10% + Zinc 1%", pType: "Serum", price: "11.99",
		skinType: []string{"oily", "blemish prone"}, concern: []string{"blemishes"}, ingreds: []string{"niacinamide", "zinc"}},
	{sku: "ORD-SQ-50", gtin: "00800001000003", brand: "The Ordinary", name: "Squalane Cleanser", pType: "Cleanser", price: "14.99",
		skinType: []string{"dry", "sensitive"}, concern: []string{"dryness"}, ingreds: []string{"squalane"}},
	{sku: "ORD-RET-30", gtin: "00800001000004", brand: "The Ordinary", name: "Retinol 0.5% in Squalane", pType: "Serum", price: "10.99",
		skinType: []string{"all"}, concern: []string{"fine lines"}, ingreds: []string{"retinol", "squalane"}},
	{sku: "ORD-VC-30", gtin: "00800001000005", brand: "The Ordinary", name: "Vitamin C Suspension 23%", pType: "Serum", price: "5.80",
		skinType: []string{"all"}, concern: []string{"dullness"}, ingreds: []string{"vitamin c"}},
	{sku: "ORD-AHA-30", gtin: "00800001000006", brand: "The Ordinary", name: "AHA 30% + BHA 2% Peeling", pType: "Treatment", price: "7.20",
		skinType: []string{"non-sensitive"}, concern: []string{"texture"}, ingreds: []string{"glycolic acid", "lactic acid"}},
	{sku: "ORD-NMF-30", gtin: "00800001000007", brand: "The Ordinary", name: "Natural Moisturizing Factors", pType: "Moisturizer", price: "8.99",
		skinType: []string{"dry"}, concern: []string{"dehydration"}, ingreds: []string{"amino acids", "ceramides"}},
	{sku: "ORD-BUF-30", gtin: "00800001000008", brand: "The Ordinary", name: "Buffet Multi-Peptide Serum", pType: "Serum", price: "14.80",
		skinType: []string{"mature"}, concern: []string{"fine lines"}, ingreds: []string{"peptides"}},
	{sku: "ORD-CAF-30", gtin: "00800001000009", brand: "The Ordinary", name: "Caffeine Solution 5% + EGCG", pType: "Eye Care", price: "7.99",
		skinType: []string{"all"}, concern: []string{"puffiness"}, ingreds: []string{"caffeine"}},
	{sku: "ORD-GLY-240", gtin: "00800001000010", brand: "The Ordinary", name: "Glycolic Acid 7% Toner", pType: "Toner", price: "12.50",
		skinType: []string{"normal", "oily"}, concern: []string{"texture"}, ingreds: []string{"glycolic acid"}},
}

var electronicsLibrary = []libProduct{
	{sku: "DELL-XPS13-9320", gtin: "00400001000001", brand: "Dell", name: "XPS 13 9320", pType: "Laptop", price: "1599",
		extras: map[string]string{"cpu": "Intel i7-1360P", "ram_gb": "16", "storage_gb": "512", "screen_size": "13.4 inch"}},
	{sku: "DELL-XPS15-9520", gtin: "00400001000002", brand: "Dell", name: "XPS 15 9520", pType: "Laptop", price: "2499",
		extras: map[string]string{"cpu": "Intel i9-12900HK", "ram_gb": "32", "storage_gb": "1024", "screen_size": "15.6 inch"}},
	{sku: "APPLE-MBA-M3-13", gtin: "00400001000003", brand: "Apple", name: "MacBook Air M3 13 inch", pType: "Laptop", price: "1399",
		extras: map[string]string{"cpu": "Apple M3", "ram_gb": "16", "storage_gb": "512", "screen_size": "13.6 inch"}},
	{sku: "APPLE-MBP-M3-14", gtin: "00400001000004", brand: "Apple", name: "MacBook Pro M3 14 inch", pType: "Laptop", price: "1999",
		extras: map[string]string{"cpu": "Apple M3 Pro", "ram_gb": "18", "storage_gb": "512", "screen_size": "14.2 inch"}},
	{sku: "LENOVO-X1C-G11", gtin: "00400001000005", brand: "Lenovo", name: "ThinkPad X1 Carbon Gen 11", pType: "Laptop", price: "2199",
		extras: map[string]string{"cpu": "Intel i7-1365U", "ram_gb": "16", "storage_gb": "1024", "screen_size": "14 inch"}},
	{sku: "ASUS-ZB-UX5400", gtin: "00400001000006", brand: "Asus", name: "ZenBook 14 OLED", pType: "Laptop", price: "1299",
		extras: map[string]string{"cpu": "Intel i7-1260P", "ram_gb": "16", "storage_gb": "512", "screen_size": "14 inch"}},
	{sku: "HP-SPECTRE-X360", gtin: "00400001000007", brand: "HP", name: "Spectre x360 14 inch", pType: "Laptop", price: "1499",
		extras: map[string]string{"cpu": "Intel i7-1355U", "ram_gb": "16", "storage_gb": "1024", "screen_size": "13.5 inch"}},
	{sku: "RAZER-BLADE14", gtin: "00400001000008", brand: "Razer", name: "Blade 14", pType: "Laptop", price: "2799",
		extras: map[string]string{"cpu": "AMD Ryzen 9 7940HS", "ram_gb": "32", "storage_gb": "1024", "screen_size": "14 inch"}},
	{sku: "MSI-PRESTIGE13", gtin: "00400001000009", brand: "MSI", name: "Prestige 13 AI Evo", pType: "Laptop", price: "1399",
		extras: map[string]string{"cpu": "Intel Core Ultra 7", "ram_gb": "32", "storage_gb": "1024", "screen_size": "13.3 inch"}},
	{sku: "FRAMEWORK-13", gtin: "00400001000010", brand: "Framework", name: "Framework Laptop 13 AMD", pType: "Laptop", price: "1499",
		extras: map[string]string{"cpu": "AMD Ryzen 7 7840U", "ram_gb": "16", "storage_gb": "512", "screen_size": "13.5 inch"}},
}

var furnitureLibrary = []libProduct{
	{sku: "IKEA-KIVIK-3S", gtin: "00500001000001", brand: "IKEA", name: "KIVIK 3-Seat Sofa", pType: "Sofa", price: "899",
		extras: map[string]string{"material": "polyester", "color": "Tibbleby beige", "dimensions": "228x95x83", "seats": "3"}},
	{sku: "WEST-HARMONY-90", gtin: "00500001000002", brand: "West Elm", name: "Harmony 90 inch Sofa Velvet", pType: "Sofa", price: "1899",
		extras: map[string]string{"material": "velvet", "color": "deep sage", "dimensions": "229x95x86", "seats": "3"}},
}

var skiLibrary = []libProduct{
	{sku: "SALOMON-QST-92", gtin: "00600001000001", brand: "Salomon", name: "QST 92 All-Mountain Skis 176cm", pType: "Skis", price: "599",
		extras: map[string]string{"length_cm": "176", "waist_mm": "92", "turn_radius_m": "17", "skill_level": "intermediate"}},
	{sku: "BURTON-CUSTOM158", gtin: "00600001000002", brand: "Burton", name: "Custom Snowboard 158 Wide", pType: "Snowboard", price: "549",
		extras: map[string]string{"length_cm": "158", "shape": "directional", "flex": "medium", "camber_profile": "camber"}},
}

// Tenant A: first 10 cosmetics from library (the baseline owner of these masters).
func buildCosmeticsA() []usecases.ShopifyItem {
	return shopifyItemsFromLibrary("100000", cosmeticsLibrary, libVariant{})
}

// Tenant B: 10 cosmetics, 7 of which overlap A across all 3 cascade stages.
//   - Indices 0..2  → identical SKU as A → bind via stage 1 (SKU exact).
//   - Indices 3..4  → SKU mangled, GTIN preserved → bind via stage 2 (GTIN).
//   - Indices 5..6  → SKU + GTIN both mangled, brand+name unchanged → stage 3 (norm key).
//   - Indices 7..9  → entirely new SKU/GTIN/name → fresh masters (B-owned).
func buildCosmeticsB() []usecases.ShopifyItem {
	src := make([]libProduct, 0, 10)
	// 0..2: same SKU as A
	src = append(src, cosmeticsLibrary[0], cosmeticsLibrary[1], cosmeticsLibrary[2])
	// 3..4: mangle SKU but keep GTIN
	for _, i := range []int{3, 4} {
		p := cosmeticsLibrary[i]
		p.sku = p.sku + "-B" // different SKU
		src = append(src, p)
	}
	// 5..6: mangle SKU and GTIN, keep brand+name (normalized match key)
	for _, i := range []int{5, 6} {
		p := cosmeticsLibrary[i]
		p.sku = "B-" + p.sku
		p.gtin = "" // drop GTIN
		src = append(src, p)
	}
	// 7..9: new products entirely
	src = append(src,
		libProduct{sku: "BX-CER-30", gtin: "00800001000099", brand: "Beauty of Joseon", name: "Glow Serum Propolis + Niacinamide", pType: "Serum", price: "17.99",
			skinType: []string{"all"}, concern: []string{"dullness"}, ingreds: []string{"propolis", "niacinamide"}},
		libProduct{sku: "BX-TON-150", gtin: "00800001000098", brand: "Klairs", name: "Supple Preparation Unscented Toner", pType: "Toner", price: "22.00",
			skinType: []string{"sensitive"}, concern: []string{"redness"}, ingreds: []string{"centella asiatica"}},
		libProduct{sku: "BX-CLR-50", gtin: "00800001000097", brand: "Cosrx", name: "Low pH Good Morning Gel Cleanser", pType: "Cleanser", price: "13.50",
			skinType: []string{"oily"}, concern: []string{"blemishes"}, ingreds: []string{"tea tree"}},
	)
	return shopifyItemsFromLibrary("200000", src, libVariant{})
}

// Tenant C: all 10 electronics — all new masters, electronics vertical
// (no typed table yet → tier3 routing).
func buildElectronicsC() []usecases.ShopifyItem {
	return shopifyItemsFromLibrary("300000", electronicsLibrary, libVariant{})
}

// Tenant D: 10 products across 4 verticals with cross-tenant overlap.
//
//   - 3 cosmetics matching A's library entries 7,8,9 by SKU (bind to A)
//   - 3 electronics matching C's library entries 0,1,2 by SKU (bind to C)
//   - 2 furniture (new)
//   - 2 ski (new)
func buildMixedD() []usecases.ShopifyItem {
	src := []libProduct{
		cosmeticsLibrary[7], cosmeticsLibrary[8], cosmeticsLibrary[9],
		electronicsLibrary[0], electronicsLibrary[1], electronicsLibrary[2],
		furnitureLibrary[0], furnitureLibrary[1],
		skiLibrary[0], skiLibrary[1],
	}
	return shopifyItemsFromLibrary("400000", src, libVariant{})
}

type libVariant struct{}

// shopifyItemsFromLibrary serializes library products into Shopify-bulk-shaped
// JSON, one item per product. gidBase is the numeric prefix to namespace IDs
// per tenant so each tenant gets its own external_id space.
func shopifyItemsFromLibrary(gidBase string, src []libProduct, _ libVariant) []usecases.ShopifyItem {
	out := make([]usecases.ShopifyItem, 0, len(src))
	for i, p := range src {
		productID := fmt.Sprintf("gid://shopify/Product/%s%d", gidBase, i+1)
		variant := map[string]any{
			"sku":                p.sku,
			"barcode":            p.gtin, // shopify calls it barcode; agents should map → master.gtin
			"price":              p.price,
			"inventory_quantity": 100,
		}
		payload := map[string]any{
			"id":          productID,
			"title":       p.name,
			"vendor":      p.brand,
			"productType": p.pType,
			"product_type": p.pType, // both shapes — agent picks one
			"handle":      fmt.Sprintf("%s-%s", lowerSlug(p.brand), lowerSlug(p.name)),
			"tags":        []string{lowerSlug(p.pType)},
			"body_html":   p.name,
			"variants":    []map[string]any{variant},
		}
		if len(p.skinType) > 0 || len(p.concern) > 0 || len(p.ingreds) > 0 {
			payload["metafields"] = map[string]any{
				"skincare": map[string]any{
					"skin_type":       p.skinType,
					"concern":         p.concern,
					"key_ingredients": p.ingreds,
				},
			}
		}
		if len(p.extras) > 0 {
			// Surface generic per-vertical attributes under a separate
			// metafield bucket so the agent can route them to tier3.<key>
			// without colliding with cosmetics.* rules.
			specs := make(map[string]any, len(p.extras))
			for k, v := range p.extras {
				specs[k] = v
			}
			payload["specs"] = specs
		}
		raw, _ := json.Marshal(payload)
		out = append(out, usecases.ShopifyItem{GID: productID, Raw: raw})
	}
	return out
}

func lowerSlug(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			r += 32
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out = append(out, r)
		} else if r == ' ' {
			out = append(out, '-')
		}
	}
	return string(out)
}

// ---------------- inspection / reporting ----------------

func readLastAgentRunCost(ctx context.Context, db *postgres.Client, tenantID string) float64 {
	var cost float64
	_ = db.Pool().QueryRow(ctx, `
		SELECT COALESCE(cost_usd, 0) FROM admin.agent_runs
		WHERE tenant_id = $1 ORDER BY started_at DESC LIMIT 1
	`, tenantID).Scan(&cost)
	return cost
}

func dumpGlobalCounts(ctx context.Context, db *postgres.Client, tenantIDs map[string]string) {
	ids := make([]string, 0, len(tenantIDs))
	for _, id := range tenantIDs {
		ids = append(ids, id)
	}
	var (
		masters, listings, cosmetics, withTier3 int
	)
	_ = db.Pool().QueryRow(ctx, `
		SELECT COUNT(DISTINCT mp.id)
		FROM catalog.master_products mp
		JOIN catalog.products p ON p.master_product_id = mp.id
		WHERE p.tenant_id = ANY($1::uuid[])
	`, ids).Scan(&masters)
	_ = db.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM catalog.products WHERE tenant_id = ANY($1::uuid[]) AND deleted_at IS NULL
	`, ids).Scan(&listings)
	_ = db.Pool().QueryRow(ctx, `
		SELECT COUNT(DISTINCT mc.master_product_id) FROM catalog.master_cosmetics mc
		JOIN catalog.products p ON p.master_product_id = mc.master_product_id
		WHERE p.tenant_id = ANY($1::uuid[])
	`, ids).Scan(&cosmetics)
	_ = db.Pool().QueryRow(ctx, `
		SELECT COUNT(DISTINCT mp.id) FROM catalog.master_products mp
		JOIN catalog.products p ON p.master_product_id = mp.id
		WHERE p.tenant_id = ANY($1::uuid[]) AND mp.tier3 <> '{}'::jsonb
	`, ids).Scan(&withTier3)

	fmt.Printf("Distinct masters across 4 tenants: %d  (expected: 27)\n", masters)
	fmt.Printf("Total active listings:             %d  (expected: 40)\n", listings)
	fmt.Printf("Distinct master_cosmetics:         %d  (expected: 13 — A=10 own + B-new=3)\n", cosmetics)
	fmt.Printf("Distinct masters with tier3:       %d  (expected: 17 — C=10 electronics + D-new=4 furniture/ski + B-new=3 cosmetics with tags/handle)\n", withTier3)
}

func dumpVerticalDistribution(ctx context.Context, db *postgres.Client, tenantIDs map[string]string) {
	ids := make([]string, 0, len(tenantIDs))
	for _, id := range tenantIDs {
		ids = append(ids, id)
	}
	rows, err := db.Pool().Query(ctx, `
		SELECT mp.vertical, COUNT(DISTINCT mp.id)
		FROM catalog.master_products mp
		JOIN catalog.products p ON p.master_product_id = mp.id
		WHERE p.tenant_id = ANY($1::uuid[])
		GROUP BY mp.vertical
		ORDER BY mp.vertical
	`, ids)
	if err != nil {
		fmt.Printf("vertical distribution query: %v\n", err)
		return
	}
	defer rows.Close()
	fmt.Println()
	fmt.Println("Vertical distribution of distinct masters:")
	for rows.Next() {
		var v string
		var n int
		_ = rows.Scan(&v, &n)
		fmt.Printf("  %-20s %d\n", v, n)
	}
}

func dumpMatchOutcomes(ctx context.Context, db *postgres.Client, tenantIDs map[string]string) {
	fmt.Println()
	fmt.Println("Per-tenant master ownership / binding:")
	for slug, id := range tenantIDs {
		var listed, ownedHere, boundElsewhere int
		_ = db.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM catalog.products WHERE tenant_id=$1 AND deleted_at IS NULL`, id).Scan(&listed)
		_ = db.Pool().QueryRow(ctx, `
			SELECT COUNT(DISTINCT mp.id) FROM catalog.master_products mp
			JOIN catalog.products p ON p.master_product_id = mp.id
			WHERE p.tenant_id = $1 AND mp.owner_tenant_id = $1
		`, id).Scan(&ownedHere)
		_ = db.Pool().QueryRow(ctx, `
			SELECT COUNT(DISTINCT mp.id) FROM catalog.master_products mp
			JOIN catalog.products p ON p.master_product_id = mp.id
			WHERE p.tenant_id = $1 AND (mp.owner_tenant_id <> $1 OR mp.owner_tenant_id IS NULL)
		`, id).Scan(&boundElsewhere)
		fmt.Printf("  %-45s listings=%d  owned=%d  bound-to-external=%d\n", slug, listed, ownedHere, boundElsewhere)
	}
}

// keep imports honest
var _ = domain.InboxItem{}
var _ = os.Stdout
