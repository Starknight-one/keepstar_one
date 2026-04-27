package usecases

import (
	"encoding/json"
	"testing"
)

func TestParsePriceCents(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"0", 0},
		{"12", 1200},
		{"12.34", 1234},
		{"12.3", 1230},
		{"12.345", 1234}, // truncate to 2 decimals
		{"0.99", 99},
		{"1234.56", 123456},
		{"abc", 0},
	}
	for _, c := range cases {
		if got := parsePriceCents(c.in); got != c.want {
			t.Errorf("parsePriceCents(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestExtractNumericID(t *testing.T) {
	cases := map[string]string{
		"":                                "",
		"12345":                           "12345",
		"gid://shopify/Product/12345":     "12345",
		"gid://shopify/ProductVariant/77": "77",
		"gid://shopify/Collection/ABC":    "gid://shopify/Collection/ABC", // non-numeric falls through
	}
	for in, want := range cases {
		if got := extractNumericID(in); got != want {
			t.Errorf("extractNumericID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStripHTML(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"<p>Hello</p>", "Hello"},
		{"<div>foo<br>bar</div>", "foo bar"},
		{"  <p>  spaced  </p>  ", "spaced"},
		{"plain text", "plain text"},
		{"<a href='x'>link</a> and <b>bold</b>", "link and bold"},
	}
	for _, c := range cases {
		if got := stripHTML(c.in); got != c.want {
			t.Errorf("stripHTML(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseCSVTags(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b , c", []string{"a", "b", "c"}},
		{",,a,,", []string{"a"}},
	}
	for _, c := range cases {
		got := parseCSVTags(c.in)
		if !sliceEqual(got, c.want) {
			t.Errorf("parseCSVTags(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseTagsField(t *testing.T) {
	// JSON array variant.
	if got := parseTagsField(json.RawMessage(`["a","b","c"]`)); !sliceEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("array tags wrong: %v", got)
	}
	// CSV string variant (Shopify webhook style).
	if got := parseTagsField(json.RawMessage(`"a, b, c"`)); !sliceEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("string tags wrong: %v", got)
	}
	// Empty → nil.
	if got := parseTagsField(nil); got != nil {
		t.Errorf("nil input should be nil, got %v", got)
	}
}

func TestParseBulkProduct(t *testing.T) {
	// Realistic-ish bulk JSONL row (after _v2_* merge).
	payload := json.RawMessage(`{
		"id": "gid://shopify/Product/12345",
		"title": "Vanilla Body Lotion",
		"handle": "vanilla-lotion",
		"descriptionHtml": "<p>Soft and hydrating.</p>",
		"vendor": "Nice Brand",
		"productType": "Body Care",
		"tags": ["bestseller", "vegan"],
		"featuredImage": {"url": "https://cdn/main.jpg", "altText": "main"},
		"_v2_variants": [
			{"id": "gid://shopify/ProductVariant/100", "sku": "VBL-50", "title": "50ml", "price": "9.99", "barcode": "0123456789012", "inventoryQuantity": 5,
			 "selectedOptions": [{"name": "Size", "value": "50ml"}],
			 "inventoryItem": {"measurement": {"weight": {"value": 60.0, "unit": "GRAMS"}}}},
			{"id": "gid://shopify/ProductVariant/101", "sku": "VBL-100", "title": "100ml", "price": "14.99", "inventoryQuantity": 10,
			 "selectedOptions": [{"name": "Size", "value": "100ml"}]}
		],
		"_v2_metafields": [
			{"namespace": "custom", "key": "scent", "type": "single_line_text_field", "value": "vanilla"}
		],
		"_v2_collections": [
			{"id": "gid://shopify/Collection/77", "handle": "body-care", "title": "Body Care"}
		]
	}`)

	view, err := parseBulkProduct(payload)
	if err != nil {
		t.Fatalf("parseBulkProduct error: %v", err)
	}
	if view.sourceID != "12345" {
		t.Errorf("sourceID = %q, want 12345", view.sourceID)
	}
	if view.title != "Vanilla Body Lotion" {
		t.Errorf("title = %q", view.title)
	}
	if view.vendor != "Nice Brand" {
		t.Errorf("vendor = %q", view.vendor)
	}
	if !sliceEqual(view.tags, []string{"bestseller", "vegan"}) {
		t.Errorf("tags = %v", view.tags)
	}
	if len(view.media) != 1 || view.media[0]["url"] != "https://cdn/main.jpg" {
		t.Errorf("media = %v", view.media)
	}
	if len(view.variants) != 2 {
		t.Fatalf("variants len = %d", len(view.variants))
	}
	v0 := view.variants[0]
	if v0["sku"] != "VBL-50" {
		t.Errorf("v0 sku = %v", v0["sku"])
	}
	if v0["price_cents"] != 999 {
		t.Errorf("v0 price_cents = %v", v0["price_cents"])
	}
	if v0["barcode"] != "0123456789012" {
		t.Errorf("v0 barcode = %v", v0["barcode"])
	}
	opts, _ := v0["options"].(map[string]string)
	if opts["size"] != "50ml" {
		t.Errorf("v0 options.size = %v", opts)
	}
	if v0["weight_value"].(float64) != 60.0 || v0["weight_unit"] != "GRAMS" {
		t.Errorf("v0 weight = %v / %v", v0["weight_value"], v0["weight_unit"])
	}

	if len(view.metafields) != 1 || view.metafields[0]["key"] != "scent" {
		t.Errorf("metafields wrong: %v", view.metafields)
	}
	if len(view.collections) != 1 || view.collections[0]["handle"] != "body-care" {
		t.Errorf("collections wrong: %v", view.collections)
	}

	// toListing aggregates: min price = 999, total stock = 15.
	listing := view.toListing("tenant-x")
	if listing.PriceCents != 999 {
		t.Errorf("PriceCents = %d, want 999", listing.PriceCents)
	}
	if listing.StockQuantity != 15 {
		t.Errorf("StockQuantity = %d, want 15", listing.StockQuantity)
	}
	if listing.SourceSystem != "shopify" || listing.SourceID != "12345" {
		t.Errorf("source = %s/%s", listing.SourceSystem, listing.SourceID)
	}
	if listing.OriginalName != "Vanilla Body Lotion" {
		t.Errorf("OriginalName = %q", listing.OriginalName)
	}
	if listing.Description != "Soft and hydrating." {
		t.Errorf("Description = %q", listing.Description)
	}
	if listing.PayloadHash == "" {
		t.Error("PayloadHash empty — expected sha256")
	}
	if got, _ := listing.RawAttributes["sku"].(string); got != "VBL-50" {
		t.Errorf("rawAttributes.sku = %q", got)
	}
	tags, _ := listing.RawAttributes["tags"].([]string)
	if !sliceEqual(tags, []string{"bestseller", "vegan"}) {
		t.Errorf("rawAttributes.tags = %v", tags)
	}
}

func TestParseWebhookProduct(t *testing.T) {
	// Realistic Shopify products/update webhook body.
	body := []byte(`{
		"id": 67890,
		"title": "Body Wash",
		"handle": "body-wash",
		"body_html": "<p>Refreshing</p>",
		"vendor": "Acme",
		"product_type": "Bath",
		"tags": "shower, gel, bestseller",
		"image": {"src": "https://cdn/wash.jpg", "alt": "wash"},
		"images": [
			{"src": "https://cdn/wash.jpg", "alt": "wash"},
			{"src": "https://cdn/wash-2.jpg", "alt": "alt view"}
		],
		"options": [{"name": "Size"}, {"name": "Scent"}],
		"variants": [
			{"id": 200, "sku": "BW-S-CITRUS", "title": "Small / Citrus", "price": "8.50", "inventory_quantity": 3, "grams": 250, "option1": "Small", "option2": "Citrus"},
			{"id": 201, "sku": "BW-L-LAVENDER", "title": "Large / Lavender", "price": "15.00", "inventory_quantity": 7, "grams": 500, "option1": "Large", "option2": "Lavender"}
		]
	}`)

	view, err := parseWebhookProduct(body)
	if err != nil {
		t.Fatalf("parseWebhookProduct error: %v", err)
	}
	if view.sourceID != "67890" {
		t.Errorf("sourceID = %q", view.sourceID)
	}
	if !sliceEqual(view.tags, []string{"shower", "gel", "bestseller"}) {
		t.Errorf("tags = %v", view.tags)
	}
	if len(view.media) != 2 {
		t.Errorf("media should dedupe featured but keep alt: got %d", len(view.media))
	}
	if view.media[0]["url"] != "https://cdn/wash.jpg" {
		t.Errorf("media[0] = %v", view.media[0])
	}
	if len(view.variants) != 2 {
		t.Fatalf("variants len = %d", len(view.variants))
	}
	v0 := view.variants[0]
	if v0["price_cents"] != 850 {
		t.Errorf("v0 price = %v", v0["price_cents"])
	}
	opts, _ := v0["options"].(map[string]string)
	if opts["size"] != "Small" || opts["scent"] != "Citrus" {
		t.Errorf("v0 options = %v", opts)
	}
	if v0["weight_value"].(float64) != 250.0 || v0["weight_unit"] != "g" {
		t.Errorf("v0 weight = %v / %v", v0["weight_value"], v0["weight_unit"])
	}

	listing := view.toListing("tenant-y")
	if listing.PriceCents != 850 || listing.StockQuantity != 10 {
		t.Errorf("aggregates: price=%d stock=%d", listing.PriceCents, listing.StockQuantity)
	}
	if listing.Description != "Refreshing" {
		t.Errorf("Description = %q", listing.Description)
	}
}

func TestParseWebhookProduct_MissingID(t *testing.T) {
	_, err := parseWebhookProduct([]byte(`{"title": "no id"}`))
	if err == nil {
		t.Error("expected error on missing id")
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
