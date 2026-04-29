// cleanup-tenant-stale — surgical cleanup of stale Shopify-imported listings
// for one tenant. Removes catalog.products, catalog.shopify_raw_imports, and
// catalog.tenant_categories rows whose source_id / external_id no longer
// exists in the merchant's current Shopify catalog. Safe to re-run:
// idempotent and only touches Shopify-sourced rows.
//
// Algorithm:
//
//   1. Fetch the current Shopify product list via Bulk Operations (full pull,
//      same query as harvester-lite). Collect both product GIDs (top-level
//      rows) and collection GIDs (child rows with __parentId pointing at a
//      product — collections come bundled per-product in the JSONL stream).
//   2. By default, print what would be deleted and exit (dry-run).
//   3. With -apply, run three DELETEs in one transaction:
//        DELETE FROM catalog.products
//          WHERE tenant_id=$1 AND source_system='shopify'
//            AND source_id NOT IN (current_product_set);
//        DELETE FROM catalog.shopify_raw_imports
//          WHERE tenant_id=$1 AND source_kind='product'
//            AND source_id NOT IN (current_product_set);
//        DELETE FROM catalog.tenant_categories
//          WHERE tenant_id=$1
//            AND external_id LIKE 'gid://shopify/Collection/%'
//            AND external_id NOT IN (current_collection_set);
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

	currentProductIDs, currentProductGIDs, currentCollectionGIDs, err := fetchCurrentCatalog(ctx, shopifyClient, *shop, full.Credentials)
	if err != nil {
		log.Fatalf("fetch current catalog: %v", err)
	}
	log.Printf("current Shopify catalog: %d products, %d collections", len(currentProductIDs), len(currentCollectionGIDs))
	if len(currentProductIDs) == 0 {
		log.Fatal("refusing to proceed with empty current set — would delete everything for this tenant")
	}

	// Build VARIADIC arg list for product NOT IN — catalog.products stores
	// source_id as bare numeric (e.g. "8289361100989").
	prodArgs := make([]any, 0, len(currentProductIDs)+1)
	prodArgs = append(prodArgs, tenantID)
	for id := range currentProductIDs {
		prodArgs = append(prodArgs, id)
	}
	prodPlaceholders := buildPlaceholders(2, len(currentProductIDs))

	// shopify_raw_imports stores source_id as the FULL GID (e.g.
	// "gid://shopify/Product/8289361100989"). Same products, different format,
	// so we need a parallel arg list keyed on GID.
	gidArgs := make([]any, 0, len(currentProductGIDs)+1)
	gidArgs = append(gidArgs, tenantID)
	for gid := range currentProductGIDs {
		gidArgs = append(gidArgs, gid)
	}
	gidPlaceholders := buildPlaceholders(2, len(currentProductGIDs))

	// Build VARIADIC arg list for collection NOT IN. Empty collection set is
	// allowed: it means every Shopify-sourced tenant_category is stale.
	collArgs := make([]any, 0, len(currentCollectionGIDs)+1)
	collArgs = append(collArgs, tenantID)
	for gid := range currentCollectionGIDs {
		collArgs = append(collArgs, gid)
	}
	collPlaceholders := buildPlaceholders(2, len(currentCollectionGIDs))

	var prodCount, stagingCount, catCount int
	if err := dbClient.Pool().QueryRow(ctx,
		"SELECT COUNT(*) FROM catalog.products WHERE tenant_id=$1 AND source_system='shopify' AND source_id NOT IN ("+prodPlaceholders+")",
		prodArgs...).Scan(&prodCount); err != nil {
		log.Fatalf("count stale products: %v", err)
	}
	if err := dbClient.Pool().QueryRow(ctx,
		"SELECT COUNT(*) FROM catalog.shopify_raw_imports WHERE tenant_id=$1 AND source_kind='product' AND source_id NOT IN ("+gidPlaceholders+")",
		gidArgs...).Scan(&stagingCount); err != nil {
		log.Fatalf("count stale staging: %v", err)
	}
	if err := dbClient.Pool().QueryRow(ctx,
		stalCategoriesCountSQL(collPlaceholders, len(currentCollectionGIDs)),
		collArgs...).Scan(&catCount); err != nil {
		log.Fatalf("count stale tenant_categories: %v", err)
	}

	log.Printf("stale rows to delete: catalog.products=%d, shopify_raw_imports=%d, tenant_categories=%d",
		prodCount, stagingCount, catCount)

	// Show a few sample stale rows so the operator can sanity-check before -apply.
	rows, err := dbClient.Pool().Query(ctx,
		"SELECT source_id, COALESCE(original_name, name, '') FROM catalog.products WHERE tenant_id=$1 AND source_system='shopify' AND source_id NOT IN ("+prodPlaceholders+") ORDER BY created_at DESC LIMIT 10",
		prodArgs...)
	if err == nil {
		log.Println("sample stale catalog.products (up to 10):")
		for rows.Next() {
			var sid, name string
			_ = rows.Scan(&sid, &name)
			log.Printf("  source_id=%s name=%q", sid, truncate(name, 60))
		}
		rows.Close()
	}

	if catCount > 0 {
		catRows, err := dbClient.Pool().Query(ctx,
			stalCategoriesSampleSQL(collPlaceholders, len(currentCollectionGIDs)),
			collArgs...)
		if err == nil {
			log.Println("sample stale tenant_categories (up to 10):")
			for catRows.Next() {
				var name, extID string
				_ = catRows.Scan(&name, &extID)
				log.Printf("  name=%q external_id=%s", truncate(name, 60), extID)
			}
			catRows.Close()
		}
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
		"DELETE FROM catalog.products WHERE tenant_id=$1 AND source_system='shopify' AND source_id NOT IN ("+prodPlaceholders+")",
		prodArgs...)
	if err != nil {
		log.Fatalf("delete from catalog.products: %v", err)
	}

	tag2, err := tx.Exec(ctx,
		"DELETE FROM catalog.shopify_raw_imports WHERE tenant_id=$1 AND source_kind='product' AND source_id NOT IN ("+gidPlaceholders+")",
		gidArgs...)
	if err != nil {
		log.Fatalf("delete from staging: %v", err)
	}

	tag3, err := tx.Exec(ctx,
		stalCategoriesDeleteSQL(collPlaceholders, len(currentCollectionGIDs)),
		collArgs...)
	if err != nil {
		log.Fatalf("delete from tenant_categories: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("commit: %v", err)
	}
	log.Printf("APPLIED: catalog.products deleted=%d, shopify_raw_imports deleted=%d, tenant_categories deleted=%d",
		tag1.RowsAffected(), tag2.RowsAffected(), tag3.RowsAffected())
}

// stalCategoriesCountSQL / stalCategoriesSampleSQL / stalCategoriesDeleteSQL
// build SQL that handles the empty-collection-set edge case: when the merchant
// catalog has zero collections, "NOT IN ()" is a syntax error, so we collapse
// the predicate to just the prefix filter and tenant scope.
func stalCategoriesCountSQL(placeholders string, n int) string {
	if n == 0 {
		return "SELECT COUNT(*) FROM catalog.tenant_categories WHERE tenant_id=$1 AND external_id LIKE 'gid://shopify/Collection/%'"
	}
	return "SELECT COUNT(*) FROM catalog.tenant_categories WHERE tenant_id=$1 AND external_id LIKE 'gid://shopify/Collection/%' AND external_id NOT IN (" + placeholders + ")"
}

func stalCategoriesSampleSQL(placeholders string, n int) string {
	if n == 0 {
		return "SELECT name, external_id FROM catalog.tenant_categories WHERE tenant_id=$1 AND external_id LIKE 'gid://shopify/Collection/%' ORDER BY created_at DESC LIMIT 10"
	}
	return "SELECT name, external_id FROM catalog.tenant_categories WHERE tenant_id=$1 AND external_id LIKE 'gid://shopify/Collection/%' AND external_id NOT IN (" + placeholders + ") ORDER BY created_at DESC LIMIT 10"
}

func stalCategoriesDeleteSQL(placeholders string, n int) string {
	if n == 0 {
		return "DELETE FROM catalog.tenant_categories WHERE tenant_id=$1 AND external_id LIKE 'gid://shopify/Collection/%'"
	}
	return "DELETE FROM catalog.tenant_categories WHERE tenant_id=$1 AND external_id LIKE 'gid://shopify/Collection/%' AND external_id NOT IN (" + placeholders + ")"
}

// fetchCurrentCatalog runs a fresh Bulk Operations query and returns:
//   - productIDs:    numeric tail of gid://shopify/Product/N (matches the
//     `source_id` format in catalog.products).
//   - productGIDs:   full gid://shopify/Product/N (matches `source_id` in
//     catalog.shopify_raw_imports — note the format differs by table).
//   - collectionGIDs: full gid://shopify/Collection/N — child rows whose ID
//     namespace is Collection (collections are bundled per-product in the bulk
//     stream). Same Collection GID may appear under many products; we dedupe.
func fetchCurrentCatalog(ctx context.Context, c *shopify.Client, shop, token string) (map[string]struct{}, map[string]struct{}, map[string]struct{}, error) {
	if _, err := c.RunBulkProductsQuery(ctx, shop, token); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "already in progress") {
			return nil, nil, nil, err
		}
		log.Println("a bulk op is already running — polling existing")
	}
	deadline := time.Now().Add(bulkOpMaxWait)
	var op *shopify.BulkOperation
	for {
		if time.Now().After(deadline) {
			return nil, nil, nil, errPollTimeout
		}
		current, err := c.GetCurrentBulkOperation(ctx, shop, token)
		if err != nil {
			return nil, nil, nil, err
		}
		if current == nil {
			return nil, nil, nil, errNoCurrentOp
		}
		switch current.Status {
		case shopify.BulkOpCompleted:
			op = current
		case shopify.BulkOpFailed, shopify.BulkOpCanceled, shopify.BulkOpExpired:
			return nil, nil, nil, errBulkTerminal(string(current.Status), current.ErrorCode)
		}
		if op != nil {
			break
		}
		log.Printf("  poll: status=%s objectCount=%s", current.Status, current.ObjectCount)
		select {
		case <-ctx.Done():
			return nil, nil, nil, ctx.Err()
		case <-time.After(bulkPollInterval):
		}
	}
	if op.URL == "" {
		// Empty catalog — caller protects against this case.
		return map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}, nil
	}
	body, err := c.FetchBulkJSONL(ctx, op.URL)
	if err != nil {
		return nil, nil, nil, err
	}
	defer body.Close()

	scanner := shopify.ScanBulkJSONL(body)
	productIDs := make(map[string]struct{}, 256)
	productGIDs := make(map[string]struct{}, 256)
	collections := make(map[string]struct{}, 64)
	for scanner.Scan() {
		var head struct {
			ID       string `json:"id"`
			ParentID string `json:"__parentId"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &head); err != nil {
			continue
		}
		if head.ParentID == "" {
			// Top-level Product node. Keep both formats — catalog.products uses
			// the bare numeric tail, shopify_raw_imports uses the full GID.
			productGIDs[head.ID] = struct{}{}
			if idx := strings.LastIndex(head.ID, "/"); idx > 0 && idx < len(head.ID)-1 {
				productIDs[head.ID[idx+1:]] = struct{}{}
			}
			continue
		}
		// Child row — collect GID if it's a Collection (skip variants, images,
		// metafields).
		if strings.Contains(head.ID, "/Collection/") {
			collections[head.ID] = struct{}{}
		}
	}
	return productIDs, productGIDs, collections, scanner.Err()
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
