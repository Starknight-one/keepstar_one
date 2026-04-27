// run-merge-apply — generate a merge_report for a tenant from the active
// MappingArtifact. Read-only against master_products / master_variants.
//
// Walks every catalog.products row for the tenant, applies the artifact rules
// (BrandMapping / FieldMapping / JunkRules / MatchStrategyConfig) plus the
// match cascade, and emits one MergeProposal per listing into a fresh row in
// catalog.merge_reports. Curator reviews the proposals and approves them; a
// separate Apply pass (Phase D3) performs the master writes.
//
// Usage:
//   ADMIN_ENCRYPTION_KEY=… DATABASE_URL=… \
//   go run ./cmd/run-merge-apply -shop keepstar-neaqpan1.myshopify.com [-print]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
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
	shop := flag.String("shop", "", "shop domain")
	printReport := flag.Bool("print", false, "print the resulting report (proposals) to stdout")
	flag.Parse()
	if *shop == "" {
		log.Fatal("usage: run-merge-apply -shop <shop>.myshopify.com [-print]")
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	lg := logger.New("info")

	dbClient, err := postgres.NewClient(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer dbClient.Close()
	if err := dbClient.RunCatalogMigrations(ctx); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	integrationsAdapter := postgres.NewIntegrationsAdapter(dbClient, box)
	integration, err := integrationsAdapter.GetByShopDomain(ctx, *shop)
	if err != nil {
		log.Fatalf("integration lookup: %v", err)
	}
	tenantID := integration.TenantID
	log.Printf("tenant=%s shop=%s", tenantID, *shop)

	catalogAdapter := postgres.NewCatalogAdapter(dbClient, lg)
	mappingArtifactAdapter := postgres.NewMappingArtifactAdapter(dbClient, lg)
	mergeReportsAdapter := postgres.NewMergeReportsAdapter(dbClient, lg)
	masterVariantsAdapter := postgres.NewMasterVariantsAdapter(dbClient, lg)
	cascade := usecases.NewMatchCascade(masterVariantsAdapter, nil, lg) // nil embedder → step 4 disabled

	uc := usecases.NewMergeApplyUseCase(catalogAdapter, mappingArtifactAdapter, mergeReportsAdapter, cascade, lg)

	log.Println("=== generating merge_report ===")
	t0 := time.Now()
	res, err := uc.GenerateReport(ctx, tenantID)
	if err != nil {
		log.Fatalf("generate report: %v", err)
	}
	log.Printf("=== generated in %s ===", time.Since(t0))

	fmt.Println()
	fmt.Println("===== MERGE REPORT =====")
	fmt.Printf("report_id:        %d\n", res.ReportID)
	fmt.Printf("status:           %s\n", res.Status)
	fmt.Printf("total_listings:   %d\n", res.TotalListings)
	fmt.Printf("auto_link:        %d\n", res.AutoLinkCount)
	fmt.Printf("new_master:       %d\n", res.NewMasterCount)
	fmt.Printf("needs_review:     %d\n", res.NeedsReviewCount)
	fmt.Printf("skip:             %d\n", res.SkipCount)
	fmt.Printf("already_linked:   %d  (existing master links — no action)\n", res.AlreadyLinkedCount)
	fmt.Printf("duration_ms:      %d\n", res.DurationMs)

	if !*printReport {
		fmt.Println("\n(re-run with -print to dump proposals)")
		return
	}

	// Pull the saved report back and print proposals one per line.
	saved, err := mergeReportsAdapter.GetByID(ctx, res.ReportID)
	if err != nil || saved == nil {
		log.Fatalf("load saved report: %v", err)
	}
	fmt.Println()
	fmt.Println("===== PROPOSALS =====")
	for i, p := range saved.Proposals {
		fmt.Printf("[%d] %s — %q\n", i+1, p.Action, truncate(p.ListingName, 50))
		fmt.Printf("    listing_id=%s vendor=%q\n", short(p.ListingID), p.ListingVendor)
		switch p.Action {
		case "link_existing":
			fmt.Printf("    target_master_variant_id=%s\n", p.TargetMasterVariantID)
			if p.MatchEvidence != nil {
				fmt.Printf("    match: %s (score=%.2f) — %s\n",
					p.MatchEvidence.Strategy, p.MatchEvidence.Score, p.MatchEvidence.Reasoning)
			}
		case "new_master":
			if p.ProposedMaster != nil {
				js, _ := json.Marshal(p.ProposedMaster)
				if len(js) > 220 {
					js = append(js[:220], []byte("…")...)
				}
				fmt.Printf("    proposed: %s\n", js)
			}
		case "needs_review":
			if p.MatchEvidence != nil {
				fmt.Printf("    reason: %s — %s\n", p.MatchEvidence.Strategy, p.MatchEvidence.Reasoning)
			}
		case "skip":
			fmt.Printf("    skip_reason: %s\n", p.SkipReason)
		}
		fmt.Println()
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
