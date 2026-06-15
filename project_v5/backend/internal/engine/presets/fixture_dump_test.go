package presets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"keepstar_v5/internal/engine"
)

// TestDumpProductCardFixture regenerates the no-DB, fully-processed
// product_card document the frontend harness consumes via
// window.KeepstarV5Widget.renderDocument(el, doc). It runs the REAL zero-LLM
// pipeline on the embedded product_card seed with realistic fake cosmetics
// products, then writes the result to
// frontend/tests/fixtures/product-card-rendered.json.
//
// Guarded: it only writes when DUMP_PRODUCT_CARD_FIXTURE=1, so the normal
// `go test ./...` run never touches the working tree. Regenerate with:
//
//	DUMP_PRODUCT_CARD_FIXTURE=1 go test ./internal/engine/presets/ \
//	    -run TestDumpProductCardFixture -count=1
//
// When unset the body still exercises the pipeline (so the fixture stays
// buildable) but skips the file write.
func TestDumpProductCardFixture(t *testing.T) {
	var preset engine.Document
	if err := json.Unmarshal(ProductCardJSON, &preset); err != nil {
		t.Fatalf("unmarshal product_card seed: %v", err)
	}

	// Three realistic fake cosmetics products matching the demo aesthetic.
	// Image URLs are stable public placeholders (picsum, seeded so they're
	// deterministic). Product 2 deliberately omits `badge` to prove the
	// optional badge degrades (hideWhenEmpty pill vanishes in the renderer).
	data := []map[string]any{
		{
			"id":             "rice-cleansing-cream",
			"name":           "Rice Cleansing Cream",
			"brand":          "The Saem",
			"priceFormatted": "$119.00",
			"rating":         4.0,
			"heroImage":      "https://picsum.photos/seed/rice-cleanser/600/600",
			"badge":          "Bestseller",
		},
		{
			"id":             "avocado-cleansing-cream",
			"name":           "Avocado Cleansing Cream",
			"brand":          "The Saem",
			"priceFormatted": "$99.00",
			"rating":         4.3,
			"heroImage":      "https://picsum.photos/seed/avocado-cleanser/600/600",
		},
		{
			"id":             "ac-ultimate-spot-cream",
			"name":           "AC Ultimate Spot Cream",
			"brand":          "COSRX",
			"priceFormatted": "$319.00",
			"rating":         4.0,
			"heroImage":      "https://picsum.photos/seed/spot-cream/600/600",
			"badge":          "Sale",
		},
	}

	// REAL pipeline, in the production order:
	// Materialise → ApplyOps → ExpandReplicates → ResolveAndInline →
	// BindData → InjectDefaultActions. product_card references no components
	// post-redesign, so Materialise takes a nil component set and ApplyOps a
	// nil op list (both faithful no-ops here, kept for fidelity).
	doc := engine.Materialise(&preset, nil)
	if err := engine.ApplyOps(doc, nil); err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	engine.ExpandReplicates(doc, len(data))
	if stats := engine.ResolveAndInline(doc); len(stats.Failed) > 0 {
		t.Fatalf("ResolveAndInline failed refs: %v", stats.Failed)
	}
	engine.BindData(doc, data)
	engine.InjectDefaultActions(doc, "product", data)

	if os.Getenv("DUMP_PRODUCT_CARD_FIXTURE") != "1" {
		t.Skip("set DUMP_PRODUCT_CARD_FIXTURE=1 to (re)write the frontend fixture")
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal processed doc: %v", err)
	}
	// backend/internal/engine/presets → repo root → frontend/tests/fixtures
	dest := filepath.Join("..", "..", "..", "..", "frontend", "tests", "fixtures", "product-card-rendered.json")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatalf("mkdir fixtures: %v", err)
	}
	if err := os.WriteFile(dest, append(out, '\n'), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Logf("wrote %d bytes to %s", len(out), dest)
}
