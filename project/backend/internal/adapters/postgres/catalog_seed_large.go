package postgres

import (
	"context"
	"fmt"
)

// mp is a master product seed entry (package-private, shared across seed files).
type mp struct {
	sku, name, desc, brand, catSlug, images, attrs, ownerSlug string
}

// listing is a product listing seed entry (package-private, shared across seed files).
type listing struct {
	tenantSlug, mpSKU string
	price, stock      int
	rating            float64
}

// SeedLargeCatalog adds a large catalog (~500+ master products, ~700+ listings) for realistic search testing.
// Idempotent: only seeds if master_products count < 200.
func SeedLargeCatalog(ctx context.Context, client *Client) error {
	var count int
	err := client.pool.QueryRow(ctx, "SELECT COUNT(*) FROM catalog.master_products").Scan(&count)
	if err != nil {
		return fmt.Errorf("check master products count: %w", err)
	}
	if count >= 200 {
		return nil // already seeded
	}

	tx, err := client.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// ── New Tenants (services) ──────────────────────────────────────────
	tenantIDs := make(map[string]string)
	newTenants := []struct{ slug, name, ttype, settings string }{
		{"beautylab", "Beauty Lab", "service", `{"theme":"warm","currency":"RUB"}`},
		{"autofix", "AutoFix", "service", `{"theme":"dark","currency":"RUB"}`},
		{"fitzone", "FitZone", "service", `{"theme":"bright","currency":"RUB"}`},
		{"homeservice", "HomeService", "service", `{"theme":"light","currency":"RUB"}`},
	}
	for _, t := range newTenants {
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO catalog.tenants (slug, name, type, settings)
			VALUES ($1, $2, $3, $4::jsonb)
			ON CONFLICT (slug) DO UPDATE SET slug=catalog.tenants.slug RETURNING id
		`, t.slug, t.name, t.ttype, t.settings).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert tenant %s: %w", t.slug, err)
		}
		tenantIDs[t.slug] = id
	}

	// ── Get Existing Tenant IDs ─────────────────────────────────────────
	for _, slug := range []string{"nike", "sportmaster", "techstore", "fashionhub"} {
		var id string
		err = tx.QueryRow(ctx, `SELECT id FROM catalog.tenants WHERE slug=$1`, slug).Scan(&id)
		if err != nil {
			return fmt.Errorf("get tenant %s: %w", slug, err)
		}
		tenantIDs[slug] = id
	}

	// ── Categories ──────────────────────────────────────────────────────
	catIDs := make(map[string]string)

	// Get existing categories
	existingCats := []string{"sneakers", "running", "basketball", "lifestyle",
		"smartphones", "laptops", "headphones", "tablets",
		"tshirts", "hoodies", "jackets", "pants", "accessories"}
	for _, slug := range existingCats {
		var id string
		err = tx.QueryRow(ctx, `SELECT id FROM catalog.categories WHERE slug=$1`, slug).Scan(&id)
		if err != nil {
			return fmt.Errorf("get category %s: %w", slug, err)
		}
		catIDs[slug] = id
	}

	// New categories
	newCats := []struct{ name, slug, parentSlug string }{
		{"Running Shoes", "running-shoes", "sneakers"},
		{"Basketball Shoes", "basketball-shoes", "sneakers"},
		{"Casual Shoes", "casual-shoes", ""},
		{"Boots", "boots", ""},
		{"Sandals", "sandals", ""},
		{"Cameras", "cameras", ""},
		{"Gaming", "gaming", ""},
		{"Smart Home", "smart-home", ""},
		{"TVs", "tvs", ""},
		{"Smartwatches", "smartwatches", ""},
		{"Haircare", "haircare", ""},
		{"Auto Repair", "auto-repair", ""},
		{"Fitness", "fitness", ""},
		{"Cleaning", "cleaning", ""},
		{"Massage", "massage", ""},
		{"Nail Care", "nail-care", ""},
		{"Training", "training", ""},
	}
	for _, c := range newCats {
		var id string
		if c.parentSlug != "" {
			parentID := catIDs[c.parentSlug]
			err = tx.QueryRow(ctx, `
				INSERT INTO catalog.categories (name, slug, parent_id)
				VALUES ($1, $2, $3)
				ON CONFLICT (slug) DO UPDATE SET slug=catalog.categories.slug RETURNING id
			`, c.name, c.slug, parentID).Scan(&id)
		} else {
			err = tx.QueryRow(ctx, `
				INSERT INTO catalog.categories (name, slug)
				VALUES ($1, $2)
				ON CONFLICT (slug) DO UPDATE SET slug=catalog.categories.slug RETURNING id
			`, c.name, c.slug).Scan(&id)
		}
		if err != nil {
			return fmt.Errorf("insert category %s: %w", c.slug, err)
		}
		catIDs[c.slug] = id
	}

	// ── Master Products ─────────────────────────────────────────────────
	var masterProducts []mp

	// Group A: Shoes
	masterProducts = append(masterProducts, seedShoesProducts()...)
	// Group B: Clothing + Accessories
	masterProducts = append(masterProducts, seedClothingProducts()...)
	// Group C: Electronics (split across 3 files for manageability)
	masterProducts = append(masterProducts, seedElecPhoneProducts()...)   // smartphones, tablets, smartwatches
	masterProducts = append(masterProducts, seedElecAudioProducts()...)   // laptops, headphones, cameras
	masterProducts = append(masterProducts, seedElecHomeProducts()...)    // gaming, smart home, TVs
	// Group D: Services
	masterProducts = append(masterProducts, seedServiceProducts()...)

	mpIDs := make(map[string]string)
	for _, p := range masterProducts {
		catID, ok := catIDs[p.catSlug]
		if !ok {
			return fmt.Errorf("unknown category slug %s for product %s", p.catSlug, p.sku)
		}
		ownerID := tenantIDs["nike"] // default
		if p.ownerSlug != "" {
			ownerID = tenantIDs[p.ownerSlug]
		}
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO catalog.master_products (sku, name, description, brand, category_id, images, attributes, owner_tenant_id)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8)
			ON CONFLICT (sku) DO UPDATE SET sku=catalog.master_products.sku RETURNING id
		`, p.sku, p.name, p.desc, p.brand, catID, p.images, p.attrs, ownerID).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert master product %s: %w", p.sku, err)
		}
		mpIDs[p.sku] = id
	}

	// ── Listings ────────────────────────────────────────────────────────
	var listings []listing
	listings = append(listings, seedShoesListings()...)
	listings = append(listings, seedClothingListings()...)
	listings = append(listings, seedElecPhoneListings()...)
	listings = append(listings, seedElecAudioListings()...)
	listings = append(listings, seedElecHomeListings()...)
	listings = append(listings, seedServiceListings()...)

	for _, l := range listings {
		mpID, ok := mpIDs[l.mpSKU]
		if !ok {
			continue // skip unknown SKU
		}
		tID, ok := tenantIDs[l.tenantSlug]
		if !ok {
			continue
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO catalog.products (tenant_id, master_product_id, price, currency, stock_quantity, rating)
			VALUES ($1, $2, $3, 'RUB', $4, $5)
		`, tID, mpID, l.price, l.stock, l.rating)
		if err != nil {
			return fmt.Errorf("insert listing %s@%s: %w", l.mpSKU, l.tenantSlug, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit large catalog: %w", err)
	}

	return nil
}
