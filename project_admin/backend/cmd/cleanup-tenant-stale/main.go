// cleanup-tenant-stale — surgical cleanup of stale Shopify-imported listings
// for one tenant. Removes catalog.products and catalog.shopify_raw_imports
// rows whose source_id no longer exists in the merchant's current Shopify
// catalog. Safe to re-run: idempotent and only touches Shopify-sourced rows
// (manually-created products / CSV imports keyed differently are untouched).
//
// Algorithm:
//
//   1. Fetch the current Shopify product list via Bulk Operations (full pull,
//      same query as harvester-lite — one product node per top-level row).
//   2. Build a set of current numeric source_ids.
//   3. By default, print what would be deleted and exit (dry-run).
//   4. With -apply, run two DELETEs in one transaction:
//        DELETE FROM catalog.products
//          WHERE tenant_id=$1 AND source_system='shopify'
//            AND source_id NOT IN (current_set);
//        DELETE FROM catalog.shopify_raw_imports
//          WHERE tenant_id=$1 AND source_kind='product'
//            AND source_id NOT IN (current_set);
//
// Usage:
//   ADMIN_ENCRYPTION_KEY=... DATABASE_URL=... \
//   go run ./cmd/cleanup-tenant-stale \
//       -shop keepstar-neaqpan1.myshopify.com [-apply]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

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
	apply := flag.Bool("apply", false, "execute DELETEs (default: dry-run, just print)")
	flag.Parse()

	if *shop == "" {
		log.Fatal("usage: cleanup-tenant-stale -shop <shop>.myshopify.com [-apply]")
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
		log.Fatalf("lookup integration: %v", err)
	}
	full, err := integrationsAdapter.Get(ctx, integration.TenantID, integration.ID)
	if err != nil {
		log.Fatalf("get full integration: %v", err)
	}
	if full.Credentials == "" {
		log.Fatal("integration has no decrypted token")
	}
	tenantID := integration.TenantID
	log.Printf("tenant=%s shop=%s", tenantID, *shop)

	shopifyClient := shopify.NewClient(cfg.ShopifyAPIKey, cfg.ShopifyAPISecret, cfg.ShopifyAPIVersion, cfg.ShopifyScopes)

	currentIDs, err := fetchCurrentProductIDs(ctx, shopifyClient, *shop, full.Credentials)
	if err != nil {
		log.Fatalf("fetch current product ids: %v", err)
	}
	log.Printf("current Shopify catalog: %d products", len(currentIDs))
	if len(currentIDs) == 0 {
		log.Fatal("refusing to proceed with empty current set — would delete everything for this tenant")
	}

	// Build VARIADIC arg list for NOT IN.
	idArgs := make([]any, 0, len(currentIDs)+1)
	idArgs = append(idArgs, tenantID)
	for id := range currentIDs {
		idArgs = append(idArgs, id)
	}
	placeholders := buildPlaceholders(2, len(currentIDs))

	var prodCount, stagingCount int
	if err := dbClient.Pool().QueryRow(ctx,
		"SELECT COUNT(*) FROM catalog.products WHERE tenant_id=$1 AND source_system='shopify' AND source_id NOT IN ("+placeholders+")",
		idArgs...).Scan(&prodCount); err != nil {
		log.Fatalf("count stale products: %v", err)
	}
	if err := dbClient.Pool().QueryRow(ctx,
		"SELECT COUNT(*) FROM catalog.shopify_raw_imports WHERE tenant_id=$1 AND source_kind='product' AND source_id NOT IN ("+placeholders+")",
		idArgs...).Scan(&stagingCount); err != nil {
		log.Fatalf("count stale staging: %v", err)
	}

	log.Printf("stale rows to delete: catalog.products=%d, shopify_raw_imports=%d", prodCount, stagingCount)

	// Show a few sample stale rows so the operator can sanity-check before -apply.
	rows, err := dbClient.Pool().Query(ctx,
		"SELECT source_id, COALESCE(original_name, name, '') FROM catalog.products WHERE tenant_id=$1 AND source_system='shopify' AND source_id NOT IN ("+placeholders+") ORDER BY created_at DESC LIMIT 10",
		idArgs...)
	if err == nil {
		log.Println("sample stale catalog.products (up to 10):")
		for rows.Next() {
			var sid, name string
			_ = rows.Scan(&sid, &name)
			log.Printf("  source_id=%s name=%q", sid, truncate(name, 60))
		}
		rows.Close()
	}

	if !*apply {
		log.Println("DRY RUN — no rows deleted. Re-run with -apply to actually delete.")
		return
	}

	tx, err := dbClient.Pool().Begin(ctx)
	if err != nil {
		log.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	tag1, err := tx.Exec(ctx,
		"DELETE FROM catalog.products WHERE tenant_id=$1 AND source_system='shopify' AND source_id NOT IN ("+placeholders+")",
		idArgs...)
	if err != nil {
		log.Fatalf("delete from catalog.products: %v", err)
	}

	tag2, err := tx.Exec(ctx,
		"DELETE FROM catalog.shopify_raw_imports WHERE tenant_id=$1 AND source_kind='product' AND source_id NOT IN ("+placeholders+")",
		idArgs...)
	if err != nil {
		log.Fatalf("delete from staging: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("commit: %v", err)
	}
	log.Printf("APPLIED: catalog.products deleted=%d, shopify_raw_imports deleted=%d",
		tag1.RowsAffected(), tag2.RowsAffected())
}

// fetchCurrentProductIDs runs a fresh Bulk Operations query and returns the set
// of numeric product IDs currently in the merchant catalog.
func fetchCurrentProductIDs(ctx context.Context, c *shopify.Client, shop, token string) (map[string]struct{}, error) {
	if _, err := c.RunBulkProductsQuery(ctx, shop, token); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "already in progress") {
			return nil, err
		}
		log.Println("a bulk op is already running — polling existing")
	}
	deadline := time.Now().Add(bulkOpMaxWait)
	var op *shopify.BulkOperation
	for {
		if time.Now().After(deadline) {
			return nil, errPollTimeout
		}
		current, err := c.GetCurrentBulkOperation(ctx, shop, token)
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, errNoCurrentOp
		}
		switch current.Status {
		case shopify.BulkOpCompleted:
			op = current
		case shopify.BulkOpFailed, shopify.BulkOpCanceled, shopify.BulkOpExpired:
			return nil, errBulkTerminal(string(current.Status), current.ErrorCode)
		}
		if op != nil {
			break
		}
		log.Printf("  poll: status=%s objectCount=%s", current.Status, current.ObjectCount)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(bulkPollInterval):
		}
	}
	if op.URL == "" {
		// Empty catalog — caller protects against this case.
		return map[string]struct{}{}, nil
	}
	body, err := c.FetchBulkJSONL(ctx, op.URL)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	scanner := shopify.ScanBulkJSONL(body)
	out := make(map[string]struct{}, 256)
	for scanner.Scan() {
		var head struct {
			ID       string `json:"id"`
			ParentID string `json:"__parentId"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &head); err != nil {
			continue
		}
		if head.ParentID != "" {
			continue
		}
		// Top-level Product. Extract numeric tail from gid://shopify/Product/123.
		if idx := strings.LastIndex(head.ID, "/"); idx > 0 && idx < len(head.ID)-1 {
			out[head.ID[idx+1:]] = struct{}{}
		}
	}
	return out, scanner.Err()
}

func buildPlaceholders(start, n int) string {
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		parts[i] = "$" + itoa(start+i)
	}
	return strings.Join(parts, ",")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// Sentinel errors below — small wrapper functions instead of `errors.New` so the
// main flow reads cleanly. None are returned from outside the package.

type errString string

func (e errString) Error() string { return string(e) }

const errPollTimeout = errString("bulk op poll exceeded timeout")
const errNoCurrentOp = errString("no current bulk op (race?)")

func errBulkTerminal(status, code string) error {
	return errString("bulk op terminal status " + status + " errorCode=" + code)
}

// Pool exposes the underlying pgxpool for ad-hoc queries. We don't add a public
// method on the postgres.Client to avoid leaking the pool elsewhere; instead we
// access through a thin internal helper that reads a typed exported method if
// one exists. If postgres.Client.Pool() doesn't exist yet, this CLI won't
// compile and the operator should add the method (one-line getter).
//
// (No assertion here — the build error is the contract.)
var _ = (*postgres.Client)(nil)

// pgxAware lets us cast to a tx-capable interface; not used externally.
type pgxAware interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}
