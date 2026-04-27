// dump-bulk-jsonl — diagnostic tool: fetch raw Shopify Bulk Operations JSONL
// for a tenant and write the first N lines to a local file with per-line type
// histogram. Used to investigate why _v2_variants/_v2_metafields/_v2_images/
// _v2_collections end up empty in catalog.products.raw_attributes (Bug #1).
//
// Decision tree after running this:
//
//   - histogram shows ProductVariant / Metafield / MediaImage / Collection
//     lines → children DO arrive; bug is in streamJSONLToStaging or
//     mergeChildrenIntoProduct (matching / accumulator).
//
//   - histogram shows only Product lines → Shopify isn't emitting children;
//     bug is in the GraphQL query body or access scopes.
//
//   - histogram is empty / very small → bulk op timed out or returned an
//     empty / failed result.
//
// Usage:
//   ADMIN_ENCRYPTION_KEY=... DATABASE_URL=... \
//   go run ./cmd/dump-bulk-jsonl -shop keepstar-neaqpan1.myshopify.com \
//                                [-out bulk-dump.jsonl] [-limit 500]
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"keepstar-admin/internal/adapters/postgres"
	"keepstar-admin/internal/adapters/shopify"
	"keepstar-admin/internal/config"
	"keepstar-admin/internal/crypto/secretbox"
)

const (
	bulkPollInterval = 5 * time.Second
	bulkOpMaxWait    = 15 * time.Minute
)

func main() {
	shop := flag.String("shop", "", "shop domain (e.g. keepstar-neaqpan1.myshopify.com)")
	outPath := flag.String("out", "", "output JSONL file (default: bulk-dump-<shop>-<ts>.jsonl)")
	limit := flag.Int("limit", 500, "max number of JSONL lines to write to file (full histogram still computed)")
	flag.Parse()

	if *shop == "" {
		log.Fatal("usage: dump-bulk-jsonl -shop <shop>.myshopify.com [-out file.jsonl] [-limit 500]")
	}
	if _, ok := shopify.ValidateShopDomain(*shop); !ok {
		log.Fatalf("invalid shop domain: %s", *shop)
	}
	if *outPath == "" {
		*outPath = fmt.Sprintf("bulk-dump-%s-%d.jsonl",
			strings.TrimSuffix(*shop, ".myshopify.com"),
			time.Now().Unix())
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

	ctx, cancel := context.WithTimeout(context.Background(), bulkOpMaxWait+5*time.Minute)
	defer cancel()

	dbClient, err := postgres.NewClient(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer dbClient.Close()

	integrationsAdapter := postgres.NewIntegrationsAdapter(dbClient, box)
	integration, err := integrationsAdapter.GetByShopDomain(ctx, *shop)
	if err != nil {
		log.Fatalf("lookup integration by shop %s: %v", *shop, err)
	}
	log.Printf("integration tenant=%s shop=%s status=%s", integration.TenantID, integration.ExternalID, integration.Status)

	full, err := integrationsAdapter.Get(ctx, integration.TenantID, integration.ID)
	if err != nil {
		log.Fatalf("get full integration: %v", err)
	}
	if full.Credentials == "" {
		log.Fatal("integration has no decrypted token (Credentials empty)")
	}

	shopifyClient := shopify.NewClient(cfg.ShopifyAPIKey, cfg.ShopifyAPISecret, cfg.ShopifyAPIVersion, cfg.ShopifyScopes)

	op, err := runOrPollBulkOp(ctx, shopifyClient, *shop, full.Credentials)
	if err != nil {
		log.Fatalf("bulk op: %v", err)
	}
	log.Printf("bulk op COMPLETED id=%s objectCount=%s fileSize=%s url=%t",
		op.ID, op.ObjectCount, op.FileSize, op.URL != "")

	if op.URL == "" {
		log.Fatal("bulk op completed but URL is empty (catalog might be empty, or app missing read_products scope)")
	}

	body, err := shopifyClient.FetchBulkJSONL(ctx, op.URL)
	if err != nil {
		log.Fatalf("fetch jsonl: %v", err)
	}
	defer body.Close()

	out, err := os.Create(*outPath)
	if err != nil {
		log.Fatalf("create out: %v", err)
	}
	defer out.Close()
	writer := bufio.NewWriter(out)
	defer writer.Flush()

	scanner := shopify.ScanBulkJSONL(body)

	histogram := map[string]int{}
	parentLinkHistogram := map[string]int{} // child type → linked-to-parent count
	noParentSamples := []string{}           // first 5 lines we couldn't classify
	totalLines := 0
	writtenLines := 0

	for scanner.Scan() {
		totalLines++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var head struct {
			ID       string `json:"id"`
			ParentID string `json:"__parentId"`
		}
		if err := json.Unmarshal(line, &head); err != nil {
			histogram["__parse_error"]++
			continue
		}

		kind := classifyGID(head.ID)
		histogram[kind]++
		if head.ParentID != "" {
			parentLinkHistogram[kind]++
		} else if kind != "Product" {
			if len(noParentSamples) < 5 {
				snippet := string(line)
				if len(snippet) > 220 {
					snippet = snippet[:220] + "..."
				}
				noParentSamples = append(noParentSamples, fmt.Sprintf("[%s] %s", kind, snippet))
			}
		}

		if writtenLines < *limit {
			writer.Write(line)
			writer.WriteByte('\n')
			writtenLines++
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("scanner: %v", err)
	}

	fmt.Println()
	fmt.Println("===== HISTOGRAM =====")
	fmt.Printf("total lines:    %d\n", totalLines)
	fmt.Printf("written to %s: %d (--limit)\n", *outPath, writtenLines)
	fmt.Println()

	keys := make([]string, 0, len(histogram))
	for k := range histogram {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		linked := parentLinkHistogram[k]
		if k == "Product" {
			fmt.Printf("  %-25s  %5d (top-level)\n", k, histogram[k])
		} else {
			fmt.Printf("  %-25s  %5d (with __parentId: %d)\n", k, histogram[k], linked)
		}
	}

	if len(noParentSamples) > 0 {
		fmt.Println()
		fmt.Println("===== UNPARENTED CHILD SAMPLES (first 5) =====")
		for _, s := range noParentSamples {
			fmt.Println(s)
		}
	}

	fmt.Println()
	fmt.Println("===== INTERPRETATION =====")
	productLines := histogram["Product"]
	variantLines := histogram["ProductVariant"]
	imageLines := histogram["MediaImage"] + histogram["ProductImage"]
	metafieldLines := histogram["Metafield"]
	collectionLines := histogram["Collection"]

	switch {
	case productLines == 0:
		fmt.Println("  No Product lines. Bulk op succeeded but yielded no products. Empty catalog?")
	case variantLines == 0 && metafieldLines == 0 && imageLines == 0 && collectionLines == 0:
		fmt.Println("  Children are MISSING from JSONL. Shopify did not emit them.")
		fmt.Println("  Likely root cause:")
		fmt.Println("    * Bulk op was created with an older/cached query body, OR")
		fmt.Println("    * Access token lacks scopes (read_inventory / read_publications), OR")
		fmt.Println("    * Products created via productSet without inventory at any location skip child emission until indexed.")
		fmt.Println("  Action: re-run bulk op with FRESH RunBulkProductsQuery (this tool does that),")
		fmt.Println("          and inspect 'productSet' job inventory in Shopify Admin.")
	case variantLines > 0 && metafieldLines == 0 && imageLines == 0 && collectionLines == 0:
		fmt.Println("  Variants present, but metafields/images/collections missing.")
		fmt.Println("  Likely a per-section issue (scopes for inventory/publications, or permission for metafields).")
	default:
		fmt.Printf("  Children PRESENT: variants=%d, images=%d, metafields=%d, collections=%d\n",
			variantLines, imageLines, metafieldLines, collectionLines)
		fmt.Println("  If staging.raw_attributes still empty, bug is in streamJSONLToStaging matching.")
		fmt.Println("  Check classifyGID outputs above against case-statements in streamJSONLToStaging.")
	}
	fmt.Println()
}

// classifyGID extracts the resource type from a Shopify GID like
// "gid://shopify/ProductVariant/12345". For non-GID strings or empty IDs,
// returns "Unknown".
func classifyGID(gid string) string {
	if gid == "" {
		return "Unknown"
	}
	const prefix = "gid://shopify/"
	if !strings.HasPrefix(gid, prefix) {
		return "Unknown(non-gid)"
	}
	rest := gid[len(prefix):]
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return "Unknown(no-slash)"
	}
	return rest[:slash]
}

// runOrPollBulkOp tries to start a fresh products bulk-op. If one is already
// running (Shopify limits to one per app+shop), it polls the existing one.
// Either way, returns the COMPLETED operation.
func runOrPollBulkOp(ctx context.Context, c *shopify.Client, shop, token string) (*shopify.BulkOperation, error) {
	if _, err := c.RunBulkProductsQuery(ctx, shop, token); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "already in progress") {
			return nil, fmt.Errorf("run query: %w", err)
		}
		log.Println("a bulk op is already running for this shop+app — polling existing")
	} else {
		log.Println("bulk op started, polling for completion")
	}

	deadline := time.Now().Add(bulkOpMaxWait)
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("bulk op exceeded %s", bulkOpMaxWait)
		}
		op, err := c.GetCurrentBulkOperation(ctx, shop, token)
		if err != nil {
			return nil, fmt.Errorf("poll: %w", err)
		}
		if op == nil {
			return nil, fmt.Errorf("no current bulk op (race?)")
		}
		switch op.Status {
		case shopify.BulkOpCompleted:
			return op, nil
		case shopify.BulkOpFailed, shopify.BulkOpCanceled, shopify.BulkOpExpired:
			return nil, fmt.Errorf("bulk op terminal status %s (errorCode=%s)", op.Status, op.ErrorCode)
		}
		log.Printf("  bulk op status=%s objectCount=%s; waiting %s...", op.Status, op.ObjectCount, bulkPollInterval)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(bulkPollInterval):
		}
	}
}
