package handlers

import (
	"testing"

	"keepstar_v5/internal/domain"
)

// The preview's sample selection must prefer products that can fill a
// card's atoms — junk rows without media made the owner 
// preset as "scary-broken" (2026-06-12). Junk first in catalog order, rich
// later: the rich ones must win, in stable catalog order.
func TestPickRichestSamplesPrefersRichProducts(t *testing.T) {
	junk := func(id, name string) domain.Product {
		return domain.Product{ID: id, Name: name}
	}
	rich := func(id, name string) domain.Product {
		return domain.Product{
			ID: id, Name: name,
			Images:      []string{"https://img.test/" + id + ".jpg"},
			Brand:       "Acme",
			Rating:      4.5,
			Description: "a real description",
		}
	}
	products := []domain.Product{
		junk("j1", "Generic Lip Balm"),
		junk("j2", "Minimal Listing No Identifiers"),
		rich("r1", "Ginger Glow Set"),
		junk("j3", "Lavender Test Row"),
		rich("r2", "Freckle SPF"),
		rich("r3", "Centella Toner"),
	}

	got := pickRichestSamples(products, 3)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	// Rich ones win, and keep their original catalog order (r1, r2, r3).
	for i, want := range []string{"r1", "r2", "r3"} {
		if got[i].ID != want {
			t.Errorf("got[%d] = %s, want %s (full: %v)", i, got[i].ID, want, ids(got))
		}
	}
}

// Fewer products than requested: keep everything, no panic, no reorder.
func TestPickRichestSamplesSmallCatalog(t *testing.T) {
	products := []domain.Product{{ID: "a"}, {ID: "b"}}
	got := pickRichestSamples(products, 5)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("small catalog mangled: %v", ids(got))
	}
}

// All-junk catalog: selection still returns count rows in catalog order —
// the preview degrades to whatever exists rather than erroring.
func TestPickRichestSamplesAllJunkKeepsOrder(t *testing.T) {
	products := []domain.Product{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}
	got := pickRichestSamples(products, 2)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("all-junk order broken: %v", ids(got))
	}
}

func ids(ps []domain.Product) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}
