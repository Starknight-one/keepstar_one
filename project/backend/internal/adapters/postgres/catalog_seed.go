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

// SeedExtendedCatalog adds extended catalog data (electronics, clothing, accessories).
// Idempotent: checks product count and only seeds if < 50 products exist.
func SeedExtendedCatalog(ctx context.Context, client *Client) error {
	var count int
	err := client.pool.QueryRow(ctx, "SELECT COUNT(*) FROM catalog.products").Scan(&count)
	if err != nil {
		return fmt.Errorf("check products count: %w", err)
	}
	if count >= 50 {
		return nil // already extended
	}

	tx, err := client.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// --- Tenants ---
	var techstoreID, fashionhubID string
	err = tx.QueryRow(ctx, `
		INSERT INTO catalog.tenants (slug, name, type, settings)
		VALUES ('techstore', 'TechStore', 'retailer', '{"theme": "dark", "currency": "RUB"}')
		ON CONFLICT (slug) DO UPDATE SET slug=catalog.tenants.slug RETURNING id
	`).Scan(&techstoreID)
	if err != nil {
		return fmt.Errorf("insert techstore tenant: %w", err)
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO catalog.tenants (slug, name, type, settings)
		VALUES ('fashionhub', 'FashionHub', 'retailer', '{"theme": "light", "currency": "RUB"}')
		ON CONFLICT (slug) DO UPDATE SET slug=catalog.tenants.slug RETURNING id
	`).Scan(&fashionhubID)
	if err != nil {
		return fmt.Errorf("insert fashionhub tenant: %w", err)
	}

	// Get existing tenant IDs
	var nikeID, sportmasterID string
	err = tx.QueryRow(ctx, `SELECT id FROM catalog.tenants WHERE slug='nike'`).Scan(&nikeID)
	if err != nil {
		return fmt.Errorf("get nike tenant: %w", err)
	}
	err = tx.QueryRow(ctx, `SELECT id FROM catalog.tenants WHERE slug='sportmaster'`).Scan(&sportmasterID)
	if err != nil {
		return fmt.Errorf("get sportmaster tenant: %w", err)
	}

	// --- Categories ---
	catIDs := make(map[string]string)
	categories := []struct{ name, slug string }{
		{"Smartphones", "smartphones"},
		{"Laptops", "laptops"},
		{"Headphones", "headphones"},
		{"Tablets", "tablets"},
		{"T-Shirts", "tshirts"},
		{"Hoodies", "hoodies"},
		{"Jackets", "jackets"},
		{"Pants", "pants"},
		{"Accessories", "accessories"},
	}
	for _, c := range categories {
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO catalog.categories (name, slug)
			VALUES ($1, $2)
			ON CONFLICT (slug) DO UPDATE SET slug=catalog.categories.slug RETURNING id
		`, c.name, c.slug).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert category %s: %w", c.slug, err)
		}
		catIDs[c.slug] = id
	}

	// --- Master Products ---
	type mp struct {
		sku, name, desc, brand, catSlug, images, attrs string
	}
	masterProducts := []mp{
		// Smartphones
		{"APPLE-IPHONE-15-PRO", "iPhone 15 Pro", "Titanium design with A17 Pro chip", "Apple", "smartphones", `["https://images.unsplash.com/photo-1695048133142-1a20484d2569?w=800"]`, `{"color":"Natural Titanium","storage":"256GB"}`},
		{"APPLE-IPHONE-15", "iPhone 15", "Dynamic Island and 48MP camera", "Apple", "smartphones", `["https://images.unsplash.com/photo-1696446701796-da61225697cc?w=800"]`, `{"color":"Blue","storage":"128GB"}`},
		{"APPLE-IPHONE-14", "iPhone 14", "A15 Bionic chip with 6-core GPU", "Apple", "smartphones", `["https://images.unsplash.com/photo-1663499482523-1c0c1bae4ce1?w=800"]`, `{"color":"Midnight","storage":"128GB"}`},
		{"SAMSUNG-S24-ULTRA", "Samsung Galaxy S24 Ultra", "Galaxy AI with S Pen", "Samsung", "smartphones", `["https://images.unsplash.com/photo-1610945415295-d9bbf067e59c?w=800"]`, `{"color":"Titanium Gray","storage":"256GB"}`},
		{"SAMSUNG-S24", "Samsung Galaxy S24", "Compact flagship with Galaxy AI", "Samsung", "smartphones", `["https://images.unsplash.com/photo-1610945415295-d9bbf067e59c?w=800"]`, `{"color":"Onyx Black","storage":"128GB"}`},
		{"SAMSUNG-S23", "Samsung Galaxy S23", "Snapdragon 8 Gen 2 performance", "Samsung", "smartphones", `["https://images.unsplash.com/photo-1610945415295-d9bbf067e59c?w=800"]`, `{"color":"Phantom Black","storage":"128GB"}`},
		{"SAMSUNG-A54", "Samsung Galaxy A54", "Mid-range champion with OIS camera", "Samsung", "smartphones", `["https://images.unsplash.com/photo-1610945415295-d9bbf067e59c?w=800"]`, `{"color":"Awesome Graphite","storage":"128GB"}`},
		{"GOOGLE-PIXEL-8-PRO", "Google Pixel 8 Pro", "Google Tensor G3 with AI photography", "Google", "smartphones", `["https://images.unsplash.com/photo-1598327105666-5b89351aff97?w=800"]`, `{"color":"Bay","storage":"128GB"}`},
		{"GOOGLE-PIXEL-8", "Google Pixel 8", "Best of Google AI in a compact phone", "Google", "smartphones", `["https://images.unsplash.com/photo-1598327105666-5b89351aff97?w=800"]`, `{"color":"Hazel","storage":"128GB"}`},
		// Laptops
		{"APPLE-MBA-M3", "MacBook Air M3", "Fanless M3 chip with 18h battery", "Apple", "laptops", `["https://images.unsplash.com/photo-1517336714731-489689fd1ca8?w=800"]`, `{"color":"Midnight","ram":"16GB","storage":"512GB"}`},
		{"APPLE-MBP-14-M3", "MacBook Pro 14 M3", "Pro performance with M3 Pro chip", "Apple", "laptops", `["https://images.unsplash.com/photo-1517336714731-489689fd1ca8?w=800"]`, `{"color":"Space Black","ram":"18GB","storage":"512GB"}`},
		{"DELL-XPS-15", "Dell XPS 15", "InfinityEdge display with 12th Gen Intel", "Dell", "laptops", `["https://images.unsplash.com/photo-1593642702821-c8da6771f0c6?w=800"]`, `{"color":"Platinum Silver","ram":"16GB","storage":"512GB"}`},
		{"DELL-INSPIRON-16", "Dell Inspiron 16", "Everyday laptop with large display", "Dell", "laptops", `["https://images.unsplash.com/photo-1593642702821-c8da6771f0c6?w=800"]`, `{"color":"Dark Blue","ram":"8GB","storage":"256GB"}`},
		{"LENOVO-X1-CARBON", "Lenovo ThinkPad X1 Carbon", "Ultra-light business laptop", "Lenovo", "laptops", `["https://images.unsplash.com/photo-1588872657578-7efd1f1555ed?w=800"]`, `{"color":"Black","ram":"16GB","storage":"512GB"}`},
		{"LENOVO-IDEAPAD-5", "Lenovo IdeaPad 5", "Slim and powerful for everyday use", "Lenovo", "laptops", `["https://images.unsplash.com/photo-1588872657578-7efd1f1555ed?w=800"]`, `{"color":"Abyss Blue","ram":"8GB","storage":"256GB"}`},
		// Headphones
		{"APPLE-AIRPODS-PRO-2", "AirPods Pro 2", "Adaptive Audio with USB-C", "Apple", "headphones", `["https://images.unsplash.com/photo-1606220588913-b3aacb4d2f46?w=800"]`, `{"type":"TWS","anc":true}`},
		{"APPLE-AIRPODS-MAX", "AirPods Max", "Over-ear with H1 chip", "Apple", "headphones", `["https://images.unsplash.com/photo-1625245488600-f03fef636a3c?w=800"]`, `{"type":"Over-ear","anc":true}`},
		{"SONY-WH1000XM5", "Sony WH-1000XM5", "Industry-leading noise cancellation", "Sony", "headphones", `["https://images.unsplash.com/photo-1618366712010-f4ae9c647dcb?w=800"]`, `{"type":"Over-ear","anc":true}`},
		{"SONY-WF1000XM5", "Sony WF-1000XM5", "Premium TWS with LDAC", "Sony", "headphones", `["https://images.unsplash.com/photo-1590658268037-6bf12f032f55?w=800"]`, `{"type":"TWS","anc":true}`},
		{"SAMSUNG-BUDS3-PRO", "Samsung Galaxy Buds3 Pro", "360 Audio with ANC", "Samsung", "headphones", `["https://images.unsplash.com/photo-1590658268037-6bf12f032f55?w=800"]`, `{"type":"TWS","anc":true}`},
		// Tablets
		{"APPLE-IPAD-PRO-M4", "iPad Pro M4", "Ultra Thin with M4 chip", "Apple", "tablets", `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`, `{"size":"11 inch","storage":"256GB"}`},
		{"APPLE-IPAD-AIR-M2", "iPad Air M2", "Portable power with M2", "Apple", "tablets", `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`, `{"size":"10.9 inch","storage":"128GB"}`},
		{"SAMSUNG-TAB-S9", "Samsung Galaxy Tab S9", "AMOLED with S Pen included", "Samsung", "tablets", `["https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800"]`, `{"size":"11 inch","storage":"128GB"}`},
		// Clothing - Hoodies
		{"NIKE-CLUB-HOODIE", "Nike Sportswear Club Hoodie", "Classic fleece pullover hoodie", "Nike", "hoodies", `["https://images.unsplash.com/photo-1556821840-3a63f95609a7?w=800"]`, `{"color":"Black","material":"Fleece"}`},
		{"NIKE-TECH-FLEECE-HOODIE", "Nike Tech Fleece Hoodie", "Lightweight warmth with modern design", "Nike", "hoodies", `["https://images.unsplash.com/photo-1556821840-3a63f95609a7?w=800"]`, `{"color":"Dark Grey","material":"Tech Fleece"}`},
		{"ADIDAS-ESS-HOODIE", "Adidas Essentials Hoodie", "Comfortable cotton blend hoodie", "Adidas", "hoodies", `["https://images.unsplash.com/photo-1556821840-3a63f95609a7?w=800"]`, `{"color":"Navy","material":"Cotton Blend"}`},
		{"ADIDAS-TREFOIL-HOODIE", "Adidas Trefoil Hoodie", "Iconic trefoil logo hoodie", "Adidas", "hoodies", `["https://images.unsplash.com/photo-1556821840-3a63f95609a7?w=800"]`, `{"color":"White","material":"French Terry"}`},
		{"PUMA-LOGO-HOODIE", "Puma Logo Hoodie", "Bold logo fleece hoodie", "Puma", "hoodies", `["https://images.unsplash.com/photo-1556821840-3a63f95609a7?w=800"]`, `{"color":"Red","material":"Fleece"}`},
		// Clothing - T-Shirts
		{"NIKE-DRIFIT-TEE", "Nike Dri-FIT T-Shirt", "Sweat-wicking performance tee", "Nike", "tshirts", `["https://images.unsplash.com/photo-1521572163474-6864f9cf17ab?w=800"]`, `{"color":"Black","material":"Dri-FIT"}`},
		{"NIKE-AIR-TEE", "Nike Air T-Shirt", "Nike Air logo cotton tee", "Nike", "tshirts", `["https://images.unsplash.com/photo-1521572163474-6864f9cf17ab?w=800"]`, `{"color":"White","material":"Cotton"}`},
		{"ADIDAS-TREFOIL-TEE", "Adidas Originals Trefoil Tee", "Classic trefoil cotton tee", "Adidas", "tshirts", `["https://images.unsplash.com/photo-1521572163474-6864f9cf17ab?w=800"]`, `{"color":"Black","material":"Cotton"}`},
		{"ADIDAS-RUN-TEE", "Adidas Run Tee", "AEROREADY running tee", "Adidas", "tshirts", `["https://images.unsplash.com/photo-1521572163474-6864f9cf17ab?w=800"]`, `{"color":"Blue","material":"AEROREADY"}`},
		{"PUMA-ESS-TEE", "Puma Essential Tee", "Everyday comfort tee", "Puma", "tshirts", `["https://images.unsplash.com/photo-1521572163474-6864f9cf17ab?w=800"]`, `{"color":"Grey","material":"Cotton"}`},
		// Clothing - Jackets
		{"TNF-THERMOBALL", "The North Face Thermoball Jacket", "Synthetic insulation for cold weather", "The North Face", "jackets", `["https://images.unsplash.com/photo-1551028719-00167b16eac5?w=800"]`, `{"color":"Black","material":"Thermoball"}`},
		{"TNF-NUPTSE", "The North Face Nuptse Jacket", "Iconic 700-fill down jacket", "The North Face", "jackets", `["https://images.unsplash.com/photo-1551028719-00167b16eac5?w=800"]`, `{"color":"TNF Black","material":"700-fill Down"}`},
		{"NIKE-WINDRUNNER", "Nike Windrunner Jacket", "Lightweight wind protection", "Nike", "jackets", `["https://images.unsplash.com/photo-1551028719-00167b16eac5?w=800"]`, `{"color":"Black/White","material":"Ripstop"}`},
		// Clothing - Pants
		{"ADIDAS-TRACK-PANTS", "Adidas Track Pants", "Classic 3-stripe track pants", "Adidas", "pants", `["https://images.unsplash.com/photo-1624378439575-d8705ad7ae80?w=800"]`, `{"color":"Black","material":"Tricot"}`},
		{"NIKE-TECH-PANTS", "Nike Tech Fleece Pants", "Slim-fit tech fleece joggers", "Nike", "pants", `["https://images.unsplash.com/photo-1624378439575-d8705ad7ae80?w=800"]`, `{"color":"Dark Grey","material":"Tech Fleece"}`},
		{"LEVIS-501", "Levi's 501 Original", "The original straight-fit jeans", "Levi's", "pants", `["https://images.unsplash.com/photo-1624378439575-d8705ad7ae80?w=800"]`, `{"color":"Medium Indigo","material":"Denim"}`},
		{"LEVIS-511", "Levi's 511 Slim", "Slim-fit stretch jeans", "Levi's", "pants", `["https://images.unsplash.com/photo-1624378439575-d8705ad7ae80?w=800"]`, `{"color":"Dark Wash","material":"Stretch Denim"}`},
		// Accessories
		{"NIKE-HERITAGE-BACKPACK", "Nike Heritage Backpack", "Durable everyday backpack", "Nike", "accessories", `["https://images.unsplash.com/photo-1553062407-98eeb64c6a62?w=800"]`, `{"color":"Black","capacity":"25L"}`},
		{"ADIDAS-LINEAR-BACKPACK", "Adidas Linear Backpack", "Sporty everyday backpack", "Adidas", "accessories", `["https://images.unsplash.com/photo-1553062407-98eeb64c6a62?w=800"]`, `{"color":"Black/White","capacity":"22L"}`},
		{"APPLE-WATCH-S9", "Apple Watch Series 9", "S9 SiP with double tap gesture", "Apple", "accessories", `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`, `{"size":"45mm","color":"Midnight"}`},
		{"APPLE-WATCH-SE", "Apple Watch SE", "Essential Apple Watch features", "Apple", "accessories", `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`, `{"size":"44mm","color":"Starlight"}`},
		{"SAMSUNG-WATCH-6", "Samsung Galaxy Watch 6", "Advanced health monitoring", "Samsung", "accessories", `["https://images.unsplash.com/photo-1546868871-af0de0ae72be?w=800"]`, `{"size":"44mm","color":"Graphite"}`},
	}

	mpIDs := make(map[string]string)
	for _, p := range masterProducts {
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO catalog.master_products (sku, name, description, brand, category_id, images, attributes, owner_tenant_id)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8)
			ON CONFLICT (sku) DO UPDATE SET sku=catalog.master_products.sku RETURNING id
		`, p.sku, p.name, p.desc, p.brand, catIDs[p.catSlug], p.images, p.attrs, nikeID).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert master product %s: %w", p.sku, err)
		}
		mpIDs[p.sku] = id
	}

	// --- Listings ---
	type listing struct {
		tenantID, mpSKU string
		price, stock    int
		rating          float64
	}

	listings := []listing{
		// TechStore — all electronics
		{techstoreID, "APPLE-IPHONE-15-PRO", 14999000, 40, 4.9},
		{techstoreID, "APPLE-IPHONE-15", 10999000, 60, 4.8},
		{techstoreID, "APPLE-IPHONE-14", 7999000, 80, 4.7},
		{techstoreID, "SAMSUNG-S24-ULTRA", 13999000, 35, 4.8},
		{techstoreID, "SAMSUNG-S24", 9999000, 50, 4.7},
		{techstoreID, "SAMSUNG-S23", 7499000, 70, 4.6},
		{techstoreID, "SAMSUNG-A54", 3499000, 100, 4.5},
		{techstoreID, "GOOGLE-PIXEL-8-PRO", 10999000, 25, 4.8},
		{techstoreID, "GOOGLE-PIXEL-8", 7999000, 40, 4.7},
		{techstoreID, "APPLE-MBA-M3", 14999000, 30, 4.9},
		{techstoreID, "APPLE-MBP-14-M3", 24999000, 15, 4.9},
		{techstoreID, "DELL-XPS-15", 17999000, 20, 4.7},
		{techstoreID, "DELL-INSPIRON-16", 7999000, 40, 4.4},
		{techstoreID, "LENOVO-X1-CARBON", 19999000, 15, 4.8},
		{techstoreID, "LENOVO-IDEAPAD-5", 8999000, 35, 4.5},
		{techstoreID, "APPLE-AIRPODS-PRO-2", 2999000, 80, 4.8},
		{techstoreID, "APPLE-AIRPODS-MAX", 6999000, 20, 4.7},
		{techstoreID, "SONY-WH1000XM5", 3999000, 40, 4.9},
		{techstoreID, "SONY-WF1000XM5", 2999000, 50, 4.8},
		{techstoreID, "SAMSUNG-BUDS3-PRO", 2499000, 60, 4.6},
		{techstoreID, "APPLE-IPAD-PRO-M4", 12999000, 20, 4.9},
		{techstoreID, "APPLE-IPAD-AIR-M2", 7999000, 30, 4.8},
		{techstoreID, "SAMSUNG-TAB-S9", 7499000, 25, 4.7},
		{techstoreID, "APPLE-WATCH-S9", 4999000, 40, 4.8},
		{techstoreID, "APPLE-WATCH-SE", 2999000, 60, 4.6},
		{techstoreID, "SAMSUNG-WATCH-6", 3499000, 45, 4.5},

		// FashionHub — all clothing + accessories
		{fashionhubID, "NIKE-CLUB-HOODIE", 599000, 80, 4.6},
		{fashionhubID, "NIKE-TECH-FLEECE-HOODIE", 899000, 50, 4.8},
		{fashionhubID, "ADIDAS-ESS-HOODIE", 499000, 90, 4.5},
		{fashionhubID, "ADIDAS-TREFOIL-HOODIE", 649000, 60, 4.6},
		{fashionhubID, "PUMA-LOGO-HOODIE", 449000, 70, 4.4},
		{fashionhubID, "NIKE-DRIFIT-TEE", 349000, 100, 4.7},
		{fashionhubID, "NIKE-AIR-TEE", 299000, 120, 4.5},
		{fashionhubID, "ADIDAS-TREFOIL-TEE", 299000, 100, 4.5},
		{fashionhubID, "ADIDAS-RUN-TEE", 349000, 80, 4.6},
		{fashionhubID, "PUMA-ESS-TEE", 199000, 150, 4.3},
		{fashionhubID, "TNF-THERMOBALL", 1999000, 30, 4.8},
		{fashionhubID, "TNF-NUPTSE", 2999000, 20, 4.9},
		{fashionhubID, "NIKE-WINDRUNNER", 999000, 40, 4.6},
		{fashionhubID, "ADIDAS-TRACK-PANTS", 499000, 80, 4.5},
		{fashionhubID, "NIKE-TECH-PANTS", 799000, 50, 4.7},
		{fashionhubID, "LEVIS-501", 899000, 60, 4.8},
		{fashionhubID, "LEVIS-511", 799000, 70, 4.7},
		{fashionhubID, "NIKE-HERITAGE-BACKPACK", 349000, 90, 4.4},
		{fashionhubID, "ADIDAS-LINEAR-BACKPACK", 299000, 100, 4.3},

		// Nike store — Nike clothing additions
		{nikeID, "NIKE-CLUB-HOODIE", 649000, 60, 4.7},
		{nikeID, "NIKE-TECH-FLEECE-HOODIE", 949000, 40, 4.9},
		{nikeID, "NIKE-DRIFIT-TEE", 399000, 80, 4.8},
		{nikeID, "NIKE-AIR-TEE", 349000, 100, 4.6},
		{nikeID, "NIKE-WINDRUNNER", 1099000, 30, 4.7},
		{nikeID, "NIKE-TECH-PANTS", 849000, 40, 4.8},
		{nikeID, "NIKE-HERITAGE-BACKPACK", 399000, 70, 4.5},

		// Sportmaster — mixed brands clothing + electronics accessories
		{sportmasterID, "NIKE-CLUB-HOODIE", 549000, 50, 4.5},
		{sportmasterID, "ADIDAS-ESS-HOODIE", 449000, 60, 4.4},
		{sportmasterID, "ADIDAS-TREFOIL-HOODIE", 599000, 40, 4.5},
		{sportmasterID, "PUMA-LOGO-HOODIE", 399000, 50, 4.3},
		{sportmasterID, "NIKE-DRIFIT-TEE", 299000, 80, 4.6},
		{sportmasterID, "ADIDAS-TREFOIL-TEE", 249000, 70, 4.4},
		{sportmasterID, "PUMA-ESS-TEE", 179000, 100, 4.2},
		{sportmasterID, "TNF-THERMOBALL", 1799000, 20, 4.7},
		{sportmasterID, "TNF-NUPTSE", 2799000, 15, 4.8},
		{sportmasterID, "NIKE-WINDRUNNER", 899000, 30, 4.5},
		{sportmasterID, "ADIDAS-TRACK-PANTS", 449000, 60, 4.4},
		{sportmasterID, "NIKE-TECH-PANTS", 749000, 40, 4.6},
		{sportmasterID, "LEVIS-501", 849000, 45, 4.7},
		{sportmasterID, "LEVIS-511", 749000, 50, 4.6},
		{sportmasterID, "NIKE-HERITAGE-BACKPACK", 299000, 60, 4.3},
		{sportmasterID, "ADIDAS-LINEAR-BACKPACK", 249000, 70, 4.2},
		{sportmasterID, "APPLE-WATCH-S9", 4799000, 20, 4.7},
		{sportmasterID, "SAMSUNG-WATCH-6", 3299000, 25, 4.4},
	}

	for _, l := range listings {
		mpID, ok := mpIDs[l.mpSKU]
		if !ok {
			continue
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO catalog.products (tenant_id, master_product_id, price, currency, stock_quantity, rating)
			VALUES ($1, $2, $3, 'RUB', $4, $5)
		`, l.tenantID, mpID, l.price, l.stock, l.rating)
		if err != nil {
			return fmt.Errorf("insert extended listing %s: %w", l.mpSKU, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit extended catalog: %w", err)
	}

	return nil
}
