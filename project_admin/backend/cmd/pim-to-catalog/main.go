// Command pim-to-catalog is the PROJECTION SEAM: it builds the v5 engine's read-model
// from the canonical sources. It reads canonical product data from PIM (name, brand,
// description, category, typed attributes incl. image_url) and per-tenant commercial
// facts (price/stock/rating) from the Price-Stock service over HTTP, joins them by the
// source product id (Amazon ASIN), and writes the admin catalog.* read-model
// (master_products / master_variants / categories / listings) + the search projection
// with embeddings via the proven CatalogV2WriterAdapter.
//
// It OWNS no data — PIM owns canonical, Price-Stock owns price/stock; this only joins
// and indexes them into what the engine queries (CQRS read-model / cache).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	openaiAdapter "keepstar-admin/internal/adapters/openai"
	"keepstar-admin/internal/adapters/postgres"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type pimProduct struct {
	ASIN        string
	Name        string
	Brand       string
	Description string
	Attributes  map[string]any
	CatSlug     string // PIM category key (unique) -> master_categories.slug
	CatName     string
}

type offer struct {
	PriceCents int
	Currency   string
	Stock      int
	Rating     *float64
}

func main() {
	for _, p := range []string{"../../project/.env", ".env"} {
		if err := godotenv.Load(p); err == nil {
			break
		}
	}
	adminDB := flag.String("admin-db", os.Getenv("DATABASE_URL"), "admin catalog DATABASE_URL (read-model target)")
	pimDB := flag.String("pim-db", os.Getenv("PIM_DATABASE_URL"), "PIM DATABASE_URL (canonical source)")
	priceStock := flag.String("pricestock", "http://localhost:8095", "Price-Stock service base URL")
	pimSource := flag.String("pim-source", "furniture-demo", "external_id_map.source in PIM")
	tenantSlug := flag.String("tenant", "pim-furniture-demo", "demo tenant slug (admin + Price-Stock)")
	vertical := flag.String("vertical", "furniture", "vertical class for masters/categories")
	limit := flag.Int("limit", 0, "max products to project (0 = all)")
	flag.Parse()
	if *adminDB == "" || *pimDB == "" {
		log.Fatal("admin-db (DATABASE_URL) and pim-db (PIM_DATABASE_URL) are required")
	}
	if err := run(*adminDB, *pimDB, *priceStock, *pimSource, *tenantSlug, *vertical, *limit); err != nil {
		log.Fatalf("ERROR: %v", err)
	}
}

func run(adminDB, pimDB, priceStock, pimSource, tenantSlug, vertical string, limit int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	lg := logger.New("info")
	client, err := postgres.NewClient(ctx, adminDB)
	if err != nil {
		return fmt.Errorf("connect admin db: %w", err)
	}
	defer client.Close()
	pool := client.Pool()

	writer := postgres.NewCatalogV2WriterAdapter(client, lg)
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		writer.SetEmbedder(openaiAdapter.NewEmbeddingClient(key, os.Getenv("EMBEDDING_MODEL"), 384))
		fmt.Println("embedder: OpenAI 384-dim (vectors will be generated)")
	} else {
		fmt.Println("embedder: none (FTS-only; embeddings NULL until a keyed run)")
	}

	pim, err := pgxpool.New(ctx, pimDB)
	if err != nil {
		return fmt.Errorf("connect pim db: %w", err)
	}
	defer pim.Close()

	// 1. Ensure the demo tenant exists; v5 resolves it by slug.
	tenantID, err := ensureTenant(ctx, pool, tenantSlug)
	if err != nil {
		return fmt.Errorf("ensure tenant: %w", err)
	}
	fmt.Printf("tenant %q -> %s\n", tenantSlug, tenantID)

	// 2. Canonical from PIM.
	products, err := readPIM(ctx, pim, pimSource)
	if err != nil {
		return fmt.Errorf("read pim: %w", err)
	}
	fmt.Printf("PIM canonical products: %d\n", len(products))

	// 3. Commercial facts from the Price-Stock service.
	offers, err := fetchOffers(priceStock, tenantSlug)
	if err != nil {
		return fmt.Errorf("fetch offers: %w", err)
	}
	fmt.Printf("Price-Stock offers: %d\n", len(offers))

	// 4. Keep only products that have a price (every rendered card must have one).
	kept := products[:0]
	var noOffer int
	for _, p := range products {
		if _, ok := offers[p.ASIN]; ok {
			kept = append(kept, p)
		} else {
			noOffer++
		}
	}
	products = kept
	if limit > 0 && len(products) > limit {
		products = products[:limit]
	}
	fmt.Printf("projecting %d products (%d skipped: no price)\n", len(products), noOffer)
	if len(products) == 0 {
		return fmt.Errorf("nothing to project")
	}

	// 5. Categories -> master_categories, keyed by the PIM category key (unique slug).
	catID, err := upsertCategories(ctx, pool, products, vertical)
	if err != nil {
		return fmt.Errorf("upsert categories: %w", err)
	}
	fmt.Printf("master_categories ensured: %d\n", len(catID))

	// 6. Masters (CALL the proven cascade upsert). results[i] aligns with products[i].
	masterUpserts := make([]ports.MasterProductUpsert, len(products))
	for i, p := range products {
		name := p.Name
		if name == "" {
			name = "Untitled"
		}
		img, _ := p.Attributes["image_url"].(string)
		masterUpserts[i] = ports.MasterProductUpsert{
			SKU:           "pim-" + p.ASIN,
			Name:          name,
			Brand:         p.Brand,
			Description:   p.Description,
			Vertical:      vertical,
			ImageURL:      img,
			OwnerTenantID: tenantID,
		}
	}
	results, err := writer.BulkMatchOrCreateMaster(ctx, masterUpserts)
	if err != nil {
		return fmt.Errorf("match/create masters: %w", err)
	}
	if len(results) != len(products) {
		return fmt.Errorf("master result count %d != products %d", len(results), len(products))
	}

	// 7. Category junction.
	if err := linkCategories(ctx, pool, products, results, catID); err != nil {
		return fmt.Errorf("link categories: %w", err)
	}

	// 8. Tier2 typed attributes (everything except the media url, which is not a spec).
	tier2 := make([]ports.BulkTier2Item, 0, len(products))
	for i, p := range products {
		patch := make(map[string]any, len(p.Attributes))
		for k, v := range p.Attributes {
			if k == "image_url" {
				continue
			}
			patch[k] = v
		}
		if len(patch) > 0 {
			tier2 = append(tier2, ports.BulkTier2Item{MasterProductID: results[i].ID, Patch: patch})
		}
	}
	if _, err := writer.BulkMergeTier2(ctx, tier2); err != nil {
		return fmt.Errorf("merge tier2: %w", err)
	}

	// 9. Listings: price/stock from Price-Stock.
	listings := make([]ports.ListingUpsert, len(products))
	for i, p := range products {
		o := offers[p.ASIN]
		name := p.Name
		if name == "" {
			name = "Untitled"
		}
		listings[i] = ports.ListingUpsert{
			TenantID:        tenantID,
			MasterProductID: results[i].ID,
			Price:           o.PriceCents,
			Currency:        o.Currency,
			Stock:           o.Stock,
			CustomTitle:     name, // mirror canonical name onto the listing so v5 renders it (no tenant override yet)
			SourceSystem:    "pim",
			SourceID:        p.ASIN,
		}
	}
	if _, err := writer.BulkUpsertListing(ctx, listings); err != nil {
		return fmt.Errorf("upsert listings: %w", err)
	}

	// 10. Supplementary: rating (Price-Stock) + images (PIM image_url) onto the listing
	//     — BulkUpsertListing does not carry these, but the v5 card renders both.
	if err := setRatingAndImages(ctx, pool, tenantID, products, offers); err != nil {
		return fmt.Errorf("set rating/images: %w", err)
	}

	// 11. Build the search projection + embeddings.
	n, err := writer.RebuildSearchProjection(ctx, tenantID, nil)
	if err != nil {
		return fmt.Errorf("rebuild projection: %w", err)
	}
	fmt.Printf("DONE: masters=%d listings=%d projection_rows=%d\n", len(results), len(listings), n)
	return nil
}

func ensureTenant(ctx context.Context, pool *pgxpool.Pool, slug string) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO catalog.tenants (slug, name, type)
		VALUES ($1, $2, 'retailer')
		ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id::text`, slug, slug).Scan(&id)
	return id, err
}

func readPIM(ctx context.Context, pim *pgxpool.Pool, source string) ([]pimProduct, error) {
	rows, err := pim.Query(ctx, `
		SELECT m.external_id, p.name, COALESCE(p.brand,''), COALESCE(p.description,''),
		       p.attributes, COALESCE(c.key,''), COALESCE(c.name,'')
		FROM external_id_map m
		JOIN products p ON p.id = m.entity_id
		LEFT JOIN categories c ON c.id = p.category_id
		WHERE m.source = $1 AND m.entity_type = 'product'`, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pimProduct
	for rows.Next() {
		var p pimProduct
		var attrs []byte
		if err := rows.Scan(&p.ASIN, &p.Name, &p.Brand, &p.Description, &attrs, &p.CatSlug, &p.CatName); err != nil {
			return nil, err
		}
		if len(attrs) > 0 {
			_ = json.Unmarshal(attrs, &p.Attributes)
		}
		if p.Attributes == nil {
			p.Attributes = map[string]any{}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func fetchOffers(base, tenant string) (map[string]offer, error) {
	resp, err := http.Get(base + "/offers?tenant=" + url.QueryEscape(tenant))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("price-stock %d: %s", resp.StatusCode, string(b))
	}
	var body struct {
		Offers []struct {
			ProductRef string   `json:"productRef"`
			PriceCents int      `json:"priceCents"`
			Currency   string   `json:"currency"`
			Stock      int      `json:"stock"`
			Rating     *float64 `json:"rating"`
		} `json:"offers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	m := make(map[string]offer, len(body.Offers))
	for _, o := range body.Offers {
		m[o.ProductRef] = offer{PriceCents: o.PriceCents, Currency: o.Currency, Stock: o.Stock, Rating: o.Rating}
	}
	return m, nil
}

func upsertCategories(ctx context.Context, pool *pgxpool.Pool, products []pimProduct, vertical string) (map[string]string, error) {
	names := map[string]string{}
	for _, p := range products {
		if p.CatSlug != "" {
			names[p.CatSlug] = p.CatName
		}
	}
	out := make(map[string]string, len(names))
	for slug, name := range names {
		var id string
		if name == "" {
			name = slug
		}
		err := pool.QueryRow(ctx, `
			INSERT INTO catalog.master_categories (slug, name, vertical)
			VALUES ($1, $2, $3)
			ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
			RETURNING id::text`, slug, name, vertical).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("category %q: %w", slug, err)
		}
		out[slug] = id
	}
	return out, nil
}

func linkCategories(ctx context.Context, pool *pgxpool.Pool, products []pimProduct, results []ports.MatchResult, catID map[string]string) error {
	b := &pgx.Batch{}
	queued := 0
	for i, p := range products {
		cid, ok := catID[p.CatSlug]
		if p.CatSlug == "" || !ok {
			continue
		}
		b.Queue(`INSERT INTO catalog.master_product_categories (master_product_id, master_category_id)
		         VALUES ($1, $2) ON CONFLICT DO NOTHING`, results[i].ID, cid)
		queued++
	}
	if queued == 0 {
		return nil
	}
	br := pool.SendBatch(ctx, b)
	defer br.Close()
	for i := 0; i < queued; i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func setRatingAndImages(ctx context.Context, pool *pgxpool.Pool, tenantID string, products []pimProduct, offers map[string]offer) error {
	b := &pgx.Batch{}
	for _, p := range products {
		o := offers[p.ASIN]
		images := "[]"
		if img, _ := p.Attributes["image_url"].(string); img != "" {
			if raw, err := json.Marshal([]string{img}); err == nil {
				images = string(raw)
			}
		}
		b.Queue(`UPDATE catalog.products
		         SET rating = COALESCE($1, rating), images = $2::jsonb, updated_at = now()
		         WHERE tenant_id = $3 AND source_system = 'pim' AND source_id = $4`,
			o.Rating, images, tenantID, p.ASIN)
	}
	br := pool.SendBatch(ctx, b)
	defer br.Close()
	for range products {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}
