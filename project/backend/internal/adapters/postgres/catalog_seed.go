package postgres

import (
	"context"
	"fmt"
)

// SeedCatalogData seeds initial catalog data if tables are empty
func SeedCatalogData(ctx context.Context, client *Client) error {
	// Check if tenants already exist
	var count int
	err := client.pool.QueryRow(ctx, "SELECT COUNT(*) FROM catalog.tenants").Scan(&count)
	if err != nil {
		return fmt.Errorf("check tenants: %w", err)
	}

	if count > 0 {
		// Data already exists, skip seeding
		return nil
	}

	// Begin transaction
	tx, err := client.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Seed tenants
	var nikeID, sportmasterID string
	err = tx.QueryRow(ctx, `
		INSERT INTO catalog.tenants (slug, name, type, settings)
		VALUES ('nike', 'Nike Official', 'brand', '{"theme": "dark", "currency": "RUB"}')
		RETURNING id
	`).Scan(&nikeID)
	if err != nil {
		return fmt.Errorf("insert nike tenant: %w", err)
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO catalog.tenants (slug, name, type, settings)
		VALUES ('sportmaster', 'Sportmaster', 'retailer', '{"theme": "light", "currency": "RUB"}')
		RETURNING id
	`).Scan(&sportmasterID)
	if err != nil {
		return fmt.Errorf("insert sportmaster tenant: %w", err)
	}

	// Seed categories
	var sneakersID, runningID, basketballID, lifestyleID string
	err = tx.QueryRow(ctx, `
		INSERT INTO catalog.categories (name, slug)
		VALUES ('Sneakers', 'sneakers')
		RETURNING id
	`).Scan(&sneakersID)
	if err != nil {
		return fmt.Errorf("insert sneakers category: %w", err)
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO catalog.categories (name, slug, parent_id)
		VALUES ('Running', 'running', $1)
		RETURNING id
	`, sneakersID).Scan(&runningID)
	if err != nil {
		return fmt.Errorf("insert running category: %w", err)
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO catalog.categories (name, slug, parent_id)
		VALUES ('Basketball', 'basketball', $1)
		RETURNING id
	`, sneakersID).Scan(&basketballID)
	if err != nil {
		return fmt.Errorf("insert basketball category: %w", err)
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO catalog.categories (name, slug, parent_id)
		VALUES ('Lifestyle', 'lifestyle', $1)
		RETURNING id
	`, sneakersID).Scan(&lifestyleID)
	if err != nil {
		return fmt.Errorf("insert lifestyle category: %w", err)
	}

	// Seed master products (Nike sneakers with Unsplash images)
	masterProducts := []struct {
		sku         string
		name        string
		description string
		brand       string
		categoryID  string
		images      string
		attributes  string
	}{
		{
			sku:         "NIKE-AIR-MAX-90",
			name:        "Nike Air Max 90",
			description: "The Nike Air Max 90 stays true to its OG running roots with the iconic Waffle outsole, stitched overlays and classic TPU accents.",
			brand:       "Nike",
			categoryID:  lifestyleID,
			images:      `["https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800", "https://images.unsplash.com/photo-1600185365926-3a2ce3cdb9eb?w=800"]`,
			attributes:  `{"color": "White/Black", "material": "Leather/Mesh", "sole": "Air Max"}`,
		},
		{
			sku:         "NIKE-AIR-FORCE-1",
			name:        "Nike Air Force 1 '07",
			description: "The radiance lives on in the Nike Air Force 1 '07, the basketball original that puts a fresh spin on what you know best.",
			brand:       "Nike",
			categoryID:  lifestyleID,
			images:      `["https://images.unsplash.com/photo-1595950653106-6c9ebd614d3a?w=800", "https://images.unsplash.com/photo-1584735175315-9d5df23860e6?w=800"]`,
			attributes:  `{"color": "White", "material": "Leather", "sole": "Air"}`,
		},
		{
			sku:         "NIKE-DUNK-LOW",
			name:        "Nike Dunk Low Retro",
			description: "Created for the hardwood but taken to the streets, the Nike Dunk Low Retro returns with crisp overlays and original team colors.",
			brand:       "Nike",
			categoryID:  lifestyleID,
			images:      `["https://images.unsplash.com/photo-1597045566677-8cf032ed6634?w=800", "https://images.unsplash.com/photo-1605408499391-6368c628ef42?w=800"]`,
			attributes:  `{"color": "Black/White", "material": "Leather", "sole": "Rubber"}`,
		},
		{
			sku:         "NIKE-PEGASUS-40",
			name:        "Nike Pegasus 40",
			description: "A springy ride for every run, the Pegasus 40 features responsive Nike React foam and Zoom Air cushioning.",
			brand:       "Nike",
			categoryID:  runningID,
			images:      `["https://images.unsplash.com/photo-1606107557195-0e29a4b5b4aa?w=800", "https://images.unsplash.com/photo-1539185441755-769473a23570?w=800"]`,
			attributes:  `{"color": "Blue/White", "material": "Mesh", "sole": "React Foam"}`,
		},
		{
			sku:         "NIKE-ZOOM-FLY-5",
			name:        "Nike Zoom Fly 5",
			description: "Built for tempo runs and race day, the Zoom Fly 5 has a full-length carbon fiber plate for explosive energy.",
			brand:       "Nike",
			categoryID:  runningID,
			images:      `["https://images.unsplash.com/photo-1551107696-a4b0c5a0d9a2?w=800", "https://images.unsplash.com/photo-1460353581641-37baddab0fa2?w=800"]`,
			attributes:  `{"color": "Volt/Black", "material": "Flyknit", "sole": "ZoomX"}`,
		},
		{
			sku:         "NIKE-LEBRON-21",
			name:        "Nike LeBron 21",
			description: "The LeBron 21 features a lightweight design with Zoom Air cushioning for explosive power on the court.",
			brand:       "Nike",
			categoryID:  basketballID,
			images:      `["https://images.unsplash.com/photo-1579338559194-a162d19bf842?w=800", "https://images.unsplash.com/photo-1552346154-21d32810aba3?w=800"]`,
			attributes:  `{"color": "Purple/Gold", "material": "Synthetic", "sole": "Zoom Air"}`,
		},
		{
			sku:         "NIKE-KD-16",
			name:        "Nike KD 16",
			description: "Kevin Durant's signature shoe delivers lightweight cushioning and responsive energy return.",
			brand:       "Nike",
			categoryID:  basketballID,
			images:      `["https://images.unsplash.com/photo-1600269452121-4f2416e55c28?w=800", "https://images.unsplash.com/photo-1608231387042-66d1773070a5?w=800"]`,
			attributes:  `{"color": "Black/Green", "material": "Mesh/Synthetic", "sole": "Zoom Strobel"}`,
		},
		{
			sku:         "NIKE-JORDAN-1-HIGH",
			name:        "Air Jordan 1 Retro High OG",
			description: "The Air Jordan 1 High is the shoe that started it all. Premium leather and classic colorways.",
			brand:       "Jordan",
			categoryID:  lifestyleID,
			images:      `["https://images.unsplash.com/photo-1607853202273-797f1c22a38e?w=800", "https://images.unsplash.com/photo-1556906781-9a412961c28c?w=800"]`,
			attributes:  `{"color": "Chicago", "material": "Leather", "sole": "Rubber"}`,
		},
	}

	masterProductIDs := make(map[string]string)
	for _, mp := range masterProducts {
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO catalog.master_products (sku, name, description, brand, category_id, images, attributes, owner_tenant_id)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8)
			RETURNING id
		`, mp.sku, mp.name, mp.description, mp.brand, mp.categoryID, mp.images, mp.attributes, nikeID).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert master product %s: %w", mp.sku, err)
		}
		masterProductIDs[mp.sku] = id
	}

	// Seed tenant products (Nike's own listings and Sportmaster's listings)
	type productListing struct {
		tenantID        string
		masterProductID string
		price           int // in kopecks
		stock           int
		rating          float64
	}

	listings := []productListing{
		// Nike's own store listings (higher prices)
		{nikeID, masterProductIDs["NIKE-AIR-MAX-90"], 1299000, 50, 4.8},
		{nikeID, masterProductIDs["NIKE-AIR-FORCE-1"], 1199000, 100, 4.9},
		{nikeID, masterProductIDs["NIKE-DUNK-LOW"], 1199000, 75, 4.7},
		{nikeID, masterProductIDs["NIKE-PEGASUS-40"], 1399000, 60, 4.6},
		{nikeID, masterProductIDs["NIKE-ZOOM-FLY-5"], 1699000, 30, 4.8},
		{nikeID, masterProductIDs["NIKE-LEBRON-21"], 2199000, 25, 4.7},
		{nikeID, masterProductIDs["NIKE-KD-16"], 1799000, 35, 4.5},
		{nikeID, masterProductIDs["NIKE-JORDAN-1-HIGH"], 1899000, 40, 4.9},

		// Sportmaster listings (lower prices, different stock)
		{sportmasterID, masterProductIDs["NIKE-AIR-MAX-90"], 1149000, 30, 4.7},
		{sportmasterID, masterProductIDs["NIKE-AIR-FORCE-1"], 1049000, 80, 4.8},
		{sportmasterID, masterProductIDs["NIKE-DUNK-LOW"], 1099000, 45, 4.6},
		{sportmasterID, masterProductIDs["NIKE-PEGASUS-40"], 1249000, 40, 4.5},
		{sportmasterID, masterProductIDs["NIKE-ZOOM-FLY-5"], 1549000, 20, 4.7},
		{sportmasterID, masterProductIDs["NIKE-LEBRON-21"], 1999000, 15, 4.6},
	}

	for _, l := range listings {
		_, err = tx.Exec(ctx, `
			INSERT INTO catalog.products (tenant_id, master_product_id, price, currency, stock_quantity, rating)
			VALUES ($1, $2, $3, 'RUB', $4, $5)
		`, l.tenantID, l.masterProductID, l.price, l.stock, l.rating)
		if err != nil {
			return fmt.Errorf("insert product listing: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
