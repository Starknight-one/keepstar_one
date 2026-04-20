package usecases

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"keepstar-admin/internal/adapters/shopify"
)

// shopifyProductToImportItem maps a Shopify REST product into the generic
// ImportItem the pipeline consumes. First-variant-wins for price/SKU/stock —
// multi-variant support is a separate epic needing a schema migration.
// Kept in usecases/ (not the adapter) to dodge an import cycle: the adapter
// must remain a leaf package with no usecase dependency.
func shopifyProductToImportItem(p shopify.ShopifyProduct) ImportItem {
	item := ImportItem{
		Name:         p.Title,
		Brand:        p.Vendor,
		Category:     p.ProductType,
		SourceSystem: "shopify",
		SourceID:     strconv.FormatInt(p.ID, 10),
	}

	if len(p.Variants) > 0 {
		v := p.Variants[0]
		item.SKU = strings.TrimSpace(v.SKU)
		item.Price = parseShopifyPriceCents(v.Price)
		item.Stock = v.InventoryQuantity
	}
	if item.SKU == "" {
		if p.Handle != "" {
			item.SKU = "shopify-" + p.Handle
		} else {
			item.SKU = fmt.Sprintf("shopify-%d", p.ID)
		}
	}

	for _, img := range p.Images {
		if img.Src != "" {
			item.Images = append(item.Images, img.Src)
		}
	}

	if p.Tags != "" {
		for _, t := range strings.Split(p.Tags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				item.Tags = append(item.Tags, t)
			}
		}
	}

	if p.BodyHTML != "" {
		if item.Attributes == nil {
			item.Attributes = map[string]any{}
		}
		item.Attributes["description"] = stripShopifyHTML(p.BodyHTML)
	}

	// Fold metafields into attributes so B7 metadata-driven binding can see
	// them in the chat engine. Key format "{namespace}.{key}" keeps
	// collisions clean across namespaces.
	for _, mf := range p.Metafields {
		if mf.Value == "" {
			continue
		}
		if item.Attributes == nil {
			item.Attributes = map[string]any{}
		}
		item.Attributes[mf.Namespace+"."+mf.Key] = mf.Value
	}

	return item
}

// parseShopifyPriceCents converts Shopify's string price ("12.99") to int cents.
// Missing decimals → whole dollars: "12" → 1200.
func parseShopifyPriceCents(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if idx := strings.Index(s, "."); idx >= 0 {
		dollars, _ := strconv.Atoi(s[:idx])
		decimals := s[idx+1:]
		if len(decimals) > 2 {
			decimals = decimals[:2]
		}
		for len(decimals) < 2 {
			decimals += "0"
		}
		cents, _ := strconv.Atoi(decimals)
		return dollars*100 + cents
	}
	whole, _ := strconv.Atoi(s)
	return whole * 100
}

var shopifyHTMLTagRE = regexp.MustCompile(`<[^>]+>`)

func stripShopifyHTML(s string) string {
	s = shopifyHTMLTagRE.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}
