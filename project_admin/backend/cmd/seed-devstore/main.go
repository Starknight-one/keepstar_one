// seed-devstore — populates the Shopify dev-store with a curated set of test
// products covering 8 scenarios that exercise the merge-agent matching logic
// (curator-driven pivot 2026-04-27 Этап 3). Run after wiping existing products
// (--reset) to start clean.
//
// Auth: pass shop + token via flags. Token can be either:
//   1. The OAuth offline access token from admin.tenant_integrations (decrypt
//      first via cmd/decrypt-shopify-token)
//   2. A custom-app access token created in Shopify Admin → Apps → Develop apps
//      (faster for local seed runs). Same scopes apply.
//
// Usage:
//   SHOPIFY_SHOP=keepstar-neaqpan1.myshopify.com \
//   SHOPIFY_TOKEN=shpat_xxx \
//   go run ./cmd/seed-devstore -reset
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"keepstar-admin/internal/adapters/shopify"
)

func main() {
	reset := flag.Bool("reset", false, "delete all existing products and collections before seeding")
	scenariosOnly := flag.String("only", "", "comma-separated scenario IDs to run (default: all 8)")
	flag.Parse()
	_ = scenariosOnly // future: filter scenarios

	shop := os.Getenv("SHOPIFY_SHOP")
	token := os.Getenv("SHOPIFY_TOKEN")
	if shop == "" || token == "" {
		log.Fatal("SHOPIFY_SHOP and SHOPIFY_TOKEN required (export them or .env)")
	}
	if _, ok := shopify.ValidateShopDomain(shop); !ok {
		log.Fatalf("SHOPIFY_SHOP must be *.myshopify.com (got %q)", shop)
	}

	client := shopify.NewClient("", "", "2026-04", "")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if *reset {
		log.Printf("=== reset: deleting existing products and collections ===")
		n, err := client.ProductsDeleteAll(ctx, shop, token)
		if err != nil {
			log.Fatalf("reset products: %v", err)
		}
		log.Printf("deleted %d products", n)
		n, err = client.CollectionsDeleteAll(ctx, shop, token)
		if err != nil {
			log.Fatalf("reset collections: %v", err)
		}
		log.Printf("deleted %d collections", n)
	}

	log.Printf("=== creating collections ===")
	collections := map[string]string{} // friendly key → GID
	collectionSpecs := []struct {
		key, title, handle, descr string
	}{
		// Scenario 3.4 uses "Skincare Solutions" instead of canonical "Face Care"
		// to test fuzzy category matching.
		{"skincare", "Skincare Solutions", "skincare-solutions",
			"Daily-routine skincare — toners, serums, creams, masks."},
		{"bestsellers", "Bestsellers", "bestsellers",
			"Top-selling products this season."},
		{"furniture", "Furniture", "furniture",
			"Solid wood and upholstered pieces (scenario 3.5: new vertical)."},
		{"footwear", "Footwear", "footwear",
			"Shoes and sneakers (scenario 3.6: multi-axis variants)."},
	}
	for _, c := range collectionSpecs {
		gid, err := client.CollectionCreate(ctx, shop, token, c.title, c.handle, c.descr)
		if err != nil {
			log.Fatalf("create collection %q: %v", c.title, err)
		}
		collections[c.key] = gid
		log.Printf("  + %s (%s)", c.title, gid)
	}

	scenarios := buildScenarios(collections)

	log.Printf("=== creating %d products across 8 scenarios ===", len(scenarios))
	created := 0
	for _, s := range scenarios {
		gid, err := client.ProductCreate(ctx, shop, token, s.input)
		if err != nil {
			log.Printf("FAIL %s — %s: %v", s.scenarioID, s.input.Title, err)
			continue
		}
		created++
		log.Printf("  [%s] + %s (%s)", s.scenarioID, s.input.Title, gid)
	}
	log.Printf("=== done: %d/%d products created ===", created, len(scenarios))
}

type scenarioProduct struct {
	scenarioID string
	input      shopify.ProductCreateInput
}

// buildScenarios returns 25-30 products covering the 8 dev-store scenarios from
// docs/New features/admin_catalog_curator_pivot_2026-04-27.md §3.
func buildScenarios(coll map[string]string) []scenarioProduct {
	skincare := []string{coll["skincare"]}
	skincareBest := []string{coll["skincare"], coll["bestsellers"]}
	furniture := []string{coll["furniture"]}
	footwear := []string{coll["footwear"]}

	// Stable image URLs hosted by Shopify CDN (already used in the existing
	// dev-store snowboard products). Shopify will fetch and re-host them.
	imgCosmetic := "https://cdn.shopify.com/s/files/1/0769/2681/2349/files/Main.jpg"
	imgFurniture := "https://images.unsplash.com/photo-1555041469-a586c61ea9bc?w=800"
	imgShoes := "https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800"

	mfText := func(ns, key, val string) shopify.MetafieldInput {
		return shopify.MetafieldInput{Namespace: ns, Key: key, Type: "single_line_text_field", Value: val}
	}
	mfNum := func(ns, key, val string) shopify.MetafieldInput {
		return shopify.MetafieldInput{Namespace: ns, Key: key, Type: "number_decimal", Value: val}
	}

	out := []scenarioProduct{}

	// ── 3.1 Полный матч с heybabes-мастером (3 cosmetics, real GTINs/SKUs from heybabes)
	// Picks 3 known heybabes brands — merge agent should match these via gtin
	// or vendor+sku cascade. Note: heybabes itself doesn't have GTINs in DB
	// (we checked), so 3.1 is mainly testing vendor+sku fuzzy matching path.
	out = append(out,
		scenarioProduct{"3.1", shopify.ProductCreateInput{
			Title: "COSRX Advanced Snail 96 Mucin Power Essence",
			Vendor: "COSRX", ProductType: "Essence",
			Tags:           []string{"essence", "snail-mucin", "hydration"},
			DescriptionHTML: "<p>Korean essence with 96% snail secretion filtrate.</p>",
			ImageURLs:       []string{imgCosmetic},
			CollectionGIDs:  skincareBest,
			Variants: []shopify.VariantInput{{
				SKU: "COSRX-SNAIL-100", PriceDecimal: "29.99", InventoryQty: 50,
			}},
		}},
		scenarioProduct{"3.1", shopify.ProductCreateInput{
			Title: "MEDI-PEEL Peptide 9 Volume Bio Tox Cream",
			Vendor: "MEDI-PEEL", ProductType: "Cream",
			Tags:           []string{"cream", "peptides", "anti-aging"},
			DescriptionHTML: "<p>Anti-aging peptide cream from Korea.</p>",
			ImageURLs:       []string{imgCosmetic},
			CollectionGIDs:  skincare,
			Variants: []shopify.VariantInput{{
				SKU: "MEDIPEEL-PEPT9-50", PriceDecimal: "42.00", InventoryQty: 30,
			}},
		}},
		scenarioProduct{"3.1", shopify.ProductCreateInput{
			Title: "The Saem Concealer Cover Perfection Tip",
			Vendor: "The Saem", ProductType: "Concealer",
			Tags:           []string{"concealer", "makeup"},
			DescriptionHTML: "<p>Cover perfection tip concealer.</p>",
			ImageURLs:       []string{imgCosmetic},
			CollectionGIDs:  skincare,
			Variants: []shopify.VariantInput{{
				SKU: "SAEM-COVER-1.5", PriceDecimal: "8.50", InventoryQty: 100,
			}},
		}},
	)

	// ── 3.2 Матч + обогащение (3 cosmetics с дополнительными metafield'ами).
	// Merge agent should propose: link to existing master, propagate new
	// metafields (scent, spf, packaging) up to master.
	out = append(out,
		scenarioProduct{"3.2", shopify.ProductCreateInput{
			Title: "Some By Mi AHA-BHA-PHA 30 Days Miracle Toner",
			Vendor: "Some By Mi", ProductType: "Toner",
			Tags:           []string{"toner", "exfoliating", "acne"},
			DescriptionHTML: "<p>Mild daily exfoliating toner.</p>",
			ImageURLs:       []string{imgCosmetic},
			CollectionGIDs:  skincare,
			Variants: []shopify.VariantInput{{
				SKU: "SOMEBYMI-MIRACLE-150", PriceDecimal: "18.00", InventoryQty: 60,
			}},
			Metafields: []shopify.MetafieldInput{
				mfText("custom", "scent", "subtle floral"),
				mfText("custom", "packaging", "amber glass with pump"),
				mfText("custom", "fragrance_free", "false"),
			},
		}},
		scenarioProduct{"3.2", shopify.ProductCreateInput{
			Title: "Fraijour Original Artemisia Calming Suncream",
			Vendor: "Fraijour", ProductType: "Suncream",
			Tags:           []string{"sunscreen", "spf", "calming"},
			DescriptionHTML: "<p>Mineral SPF50+ with artemisia extract.</p>",
			ImageURLs:       []string{imgCosmetic},
			CollectionGIDs:  skincare,
			Variants: []shopify.VariantInput{{
				SKU: "FRAIJOUR-SUN50-50", PriceDecimal: "22.00", InventoryQty: 40,
			}},
			Metafields: []shopify.MetafieldInput{
				mfNum("custom", "spf", "50"),
				mfText("custom", "spf_type", "mineral"),
				mfText("custom", "scent", "herbal"),
			},
		}},
		scenarioProduct{"3.2", shopify.ProductCreateInput{
			Title: "Ma:nyo Pure Cleansing Oil",
			Vendor: "Ma:nyo", ProductType: "Cleanser",
			Tags:           []string{"cleanser", "oil-cleanser"},
			DescriptionHTML: "<p>Pure plant-derived cleansing oil.</p>",
			ImageURLs:       []string{imgCosmetic},
			CollectionGIDs:  skincare,
			Variants: []shopify.VariantInput{{
				SKU: "MANYO-OIL-200", PriceDecimal: "26.00", InventoryQty: 35,
			}},
			Metafields: []shopify.MetafieldInput{
				mfText("custom", "scent", "lightly herbal"),
				mfText("custom", "vegan", "true"),
				mfText("custom", "key_oils", "jojoba, sunflower"),
			},
		}},
	)

	// ── 3.3 Матч, у тенанта меньше данных (3 cosmetics с минимальными полями).
	// Merge agent should propose link, NOT propose downgrading master.
	out = append(out,
		scenarioProduct{"3.3", shopify.ProductCreateInput{
			Title:          "Papa Recipe Bombee Honey Mask",
			Vendor:         "Papa Recipe",
			Tags:           []string{"sheet-mask"},
			ImageURLs:      []string{imgCosmetic},
			CollectionGIDs: skincare,
			Variants: []shopify.VariantInput{{
				SKU: "PAPA-HONEY-1", PriceDecimal: "3.50", InventoryQty: 200,
			}},
		}},
		scenarioProduct{"3.3", shopify.ProductCreateInput{
			Title:    "Derma Factory Niacinamide 12% Serum",
			Vendor:   "Derma Factory",
			ImageURLs: []string{imgCosmetic},
			CollectionGIDs: skincare,
			Variants: []shopify.VariantInput{{
				SKU: "DERMA-NIA12-30", PriceDecimal: "11.00", InventoryQty: 25,
			}},
		}},
		scenarioProduct{"3.3", shopify.ProductCreateInput{
			Title:          "IsNtree Hyaluronic Acid Toner",
			Vendor:         "IsNtree",
			CollectionGIDs: skincare,
			// Intentionally no images, no metafields, no description.
			Variants: []shopify.VariantInput{{
				SKU: "ISNTREE-HA-200", PriceDecimal: "16.00", InventoryQty: 20,
			}},
		}},
	)

	// ── 3.4 — already encoded: products in "Skincare Solutions" collection
	// instead of canonical "Face Care". Tests fuzzy category match.
	// (No new products needed; 3.1-3.3 above are in Skincare Solutions.)

	// ── 3.5 Нет матча, новая vertical (5 furniture products)
	out = append(out,
		scenarioProduct{"3.5", shopify.ProductCreateInput{
			Title: "Walnut Dining Chair", Vendor: "Heritage Woodworks",
			ProductType: "Chair", Tags: []string{"dining", "wood", "modern"},
			DescriptionHTML: "<p>Solid walnut dining chair with linen upholstery.</p>",
			ImageURLs:       []string{imgFurniture},
			CollectionGIDs:  furniture,
			Variants: []shopify.VariantInput{{
				SKU: "HW-WALNUT-CHAIR", PriceDecimal: "289.00", InventoryQty: 12,
				WeightGrams: 4500,
			}},
			Metafields: []shopify.MetafieldInput{
				mfText("custom", "wood_type", "walnut"),
				mfText("custom", "upholstery", "linen"),
				mfText("custom", "assembly_required", "yes"),
			},
		}},
		scenarioProduct{"3.5", shopify.ProductCreateInput{
			Title: "Oak Coffee Table", Vendor: "Heritage Woodworks",
			ProductType: "Table", Tags: []string{"living-room", "wood"},
			DescriptionHTML: "<p>Oak coffee table with hairpin legs.</p>",
			ImageURLs:       []string{imgFurniture},
			CollectionGIDs:  furniture,
			Variants: []shopify.VariantInput{{
				SKU: "HW-OAK-COFFEE", PriceDecimal: "449.00", InventoryQty: 8,
				WeightGrams: 12000,
			}},
			Metafields: []shopify.MetafieldInput{
				mfText("custom", "wood_type", "oak"),
				mfText("custom", "assembly_required", "no"),
			},
		}},
		scenarioProduct{"3.5", shopify.ProductCreateInput{
			Title: "Brass Floor Lamp", Vendor: "Northlight",
			ProductType: "Lighting", Tags: []string{"lamp", "brass", "art-deco"},
			DescriptionHTML: "<p>Mid-century brass floor lamp.</p>",
			ImageURLs:       []string{imgFurniture},
			CollectionGIDs:  furniture,
			Variants: []shopify.VariantInput{{
				SKU: "NL-BRASS-FLOOR", PriceDecimal: "319.00", InventoryQty: 15,
			}},
			Metafields: []shopify.MetafieldInput{
				mfText("custom", "material", "brass"),
				mfText("custom", "bulb_socket", "E27"),
			},
		}},
		scenarioProduct{"3.5", shopify.ProductCreateInput{
			Title: "Linen Reading Armchair", Vendor: "Cushion & Sons",
			ProductType: "Chair", Tags: []string{"armchair", "linen"},
			DescriptionHTML: "<p>Wide linen armchair, lounge-style.</p>",
			ImageURLs:       []string{imgFurniture},
			CollectionGIDs:  furniture,
			Variants: []shopify.VariantInput{{
				SKU: "CS-LINEN-CHAIR", PriceDecimal: "699.00", InventoryQty: 5,
				WeightGrams: 22000,
			}},
			Metafields: []shopify.MetafieldInput{
				mfText("custom", "upholstery", "linen"),
				mfText("custom", "frame_material", "kiln-dried oak"),
			},
		}},
		scenarioProduct{"3.5", shopify.ProductCreateInput{
			Title: "Marble Side Table", Vendor: "Stone & Steel",
			ProductType: "Table", Tags: []string{"side-table", "marble"},
			DescriptionHTML: "<p>Carrara marble top, blackened steel base.</p>",
			ImageURLs:       []string{imgFurniture},
			CollectionGIDs:  furniture,
			Variants: []shopify.VariantInput{{
				SKU: "SS-MARBLE-SIDE", PriceDecimal: "229.00", InventoryQty: 10,
				WeightGrams: 8500,
			}},
			Metafields: []shopify.MetafieldInput{
				mfText("custom", "stone_type", "Carrara marble"),
				mfText("custom", "base_material", "blackened steel"),
			},
		}},
	)

	// ── 3.6 Multi-axis variants (1 shoe with Color × Size × Material)
	multiAxisVariants := []shopify.VariantInput{
		{SKU: "TR-RUNNER-BLK-9-MESH", PriceDecimal: "129.00", InventoryQty: 8,
			OptionValues: []string{"Black", "9", "Mesh"}, WeightGrams: 280},
		{SKU: "TR-RUNNER-BLK-10-MESH", PriceDecimal: "129.00", InventoryQty: 6,
			OptionValues: []string{"Black", "10", "Mesh"}, WeightGrams: 290},
		{SKU: "TR-RUNNER-WHT-9-LEATHER", PriceDecimal: "139.00", InventoryQty: 4,
			OptionValues: []string{"White", "9", "Leather"}, WeightGrams: 310},
		{SKU: "TR-RUNNER-WHT-10-LEATHER", PriceDecimal: "139.00", InventoryQty: 5,
			OptionValues: []string{"White", "10", "Leather"}, WeightGrams: 320},
		{SKU: "TR-RUNNER-NVY-9-MESH", PriceDecimal: "129.00", InventoryQty: 3,
			OptionValues: []string{"Navy", "9", "Mesh"}, WeightGrams: 280},
		{SKU: "TR-RUNNER-NVY-10-MESH", PriceDecimal: "129.00", InventoryQty: 7,
			OptionValues: []string{"Navy", "10", "Mesh"}, WeightGrams: 290},
	}
	out = append(out,
		scenarioProduct{"3.6", shopify.ProductCreateInput{
			Title:           "Trail Runner Sneaker",
			Vendor:          "PaceLab",
			ProductType:     "Sneakers",
			Tags:            []string{"running", "trail", "lightweight"},
			DescriptionHTML: "<p>All-terrain runner with breathable upper.</p>",
			ImageURLs:       []string{imgShoes},
			CollectionGIDs:  footwear,
			OptionNames:     []string{"Color", "Size", "Material"},
			Variants:        multiAxisVariants,
			Metafields: []shopify.MetafieldInput{
				mfText("custom", "drop_mm", "8"),
				mfText("custom", "category", "trail"),
			},
		}},
	)

	// ── 3.7 Junk variants (gift wrap, sample sachet — should be detected by junk_detector)
	out = append(out,
		scenarioProduct{"3.7", shopify.ProductCreateInput{
			Title:           "Gift Wrap Service",
			Vendor:          "Keepstar Store",
			ProductType:     "Service",
			Tags:            []string{"gift", "wrap"},
			DescriptionHTML: "<p>Add gift wrapping to your order.</p>",
			CollectionGIDs:  []string{}, // intentionally no collection
			Variants: []shopify.VariantInput{
				{SKU: "GIFT-WRAP-S", PriceDecimal: "5.00", InventoryQty: 999,
					OptionValues: []string{"Small"}},
				{SKU: "GIFT-WRAP-L", PriceDecimal: "8.00", InventoryQty: 999,
					OptionValues: []string{"Large"}},
			},
			OptionNames: []string{"Size"},
		}},
		scenarioProduct{"3.7", shopify.ProductCreateInput{
			Title:           "Sample Sachet",
			Vendor:          "Keepstar Store",
			ProductType:     "Sample",
			Tags:            []string{"sample"},
			DescriptionHTML: "<p>2ml sample sachet — try before you buy.</p>",
			Variants: []shopify.VariantInput{{
				SKU: "SAMPLE-2ML", PriceDecimal: "2.00", InventoryQty: 999,
			}},
		}},
	)

	// ── 3.8 Edge cases (missing fields, very long title, empty metafields)
	out = append(out,
		scenarioProduct{"3.8", shopify.ProductCreateInput{
			Title:    "Minimal Listing No Identifiers",
			Vendor:   "Anonymous",
			Variants: []shopify.VariantInput{{
				PriceDecimal: "9.99", InventoryQty: 10,
				// no SKU, no barcode
			}},
		}},
		scenarioProduct{"3.8", shopify.ProductCreateInput{
			Title:           "Lavender Hand Cream — A Very Long Title That Goes On And On Because Some Merchants Stuff Keywords Into Titles For SEO And It Becomes Quite Unwieldy When Rendering In Tight UI Spaces",
			Vendor:          "Boutique de Provence",
			ProductType:     "Hand Cream",
			DescriptionHTML: "<p>Hydrating lavender-infused hand cream.</p>",
			CollectionGIDs:  skincare,
			Variants: []shopify.VariantInput{{
				SKU: "BDP-LAVENDER-HAND", PriceDecimal: "14.00", InventoryQty: 18,
			}},
			Metafields: []shopify.MetafieldInput{
				// Empty values — agent should skip these proposals
				mfText("custom", "ingredients_full", ""),
			},
		}},
		scenarioProduct{"3.8", shopify.ProductCreateInput{
			Title: "Generic Lip Balm",
			// no vendor, no productType, no tags
			ImageURLs: []string{imgCosmetic},
			Variants: []shopify.VariantInput{{
				SKU: "LIP-001", PriceDecimal: "4.00", InventoryQty: 50,
			}},
		}},
	)

	return out
}

// _ tells Go we're aware fmt is used in build-time logging (it is, via log).
var _ = fmt.Sprintf
