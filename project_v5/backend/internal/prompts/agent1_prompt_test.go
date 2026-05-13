package prompts

import (
	"encoding/json"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
)

func TestBuildAgent1ContextPrompt_ColdSession(t *testing.T) {
	// No data, no rendered → raw query passes through unchanged. The whole
	// point is to not pay tokens for an empty <state> envelope.
	got := BuildAgent1ContextPrompt(domain.StateMeta{}, nil, "hi")
	if got != "hi" {
		t.Errorf("cold session: got %q, want raw query", got)
	}
}

func TestBuildAgent1ContextPrompt_LoadedNoRender(t *testing.T) {
	meta := domain.StateMeta{ProductCount: 5}
	got := BuildAgent1ContextPrompt(meta, nil, "show more")

	if !strings.HasPrefix(got, "<state>\n") {
		t.Fatalf("missing <state> envelope: %q", got)
	}
	body := extractStateBody(t, got)
	if body["loaded_products"] != float64(5) {
		t.Errorf("loaded_products=%v, want 5", body["loaded_products"])
	}
	if _, ok := body["rendered"]; ok {
		t.Error("rendered key should be absent when no items")
	}
	// Liked/cart counts must NOT appear (regression test for chunk-16 drop).
	for _, banned := range []string{"liked_count", "cart_count", "available_fields"} {
		if _, ok := body[banned]; ok {
			t.Errorf("dropped key %q resurfaced", banned)
		}
	}
}

func TestBuildAgent1ContextPrompt_RenderedSubset(t *testing.T) {
	meta := domain.StateMeta{ProductCount: 50}
	rendered := []RenderedItem{
		{ID: "p1", Name: "Snail Cream", Brand: "COSRX", Price: 350000, Rating: 4.7, Images: []string{"https://x/1.jpg"}, MarketingClaim: "for dry skin"},
		{ID: "p2", Name: "Hyaluronic Toner", Brand: "COSRX"},
	}
	got := BuildAgent1ContextPrompt(meta, rendered, "у первого добавь рейтинг")

	body := extractStateBody(t, got)
	arr, ok := body["rendered"].([]interface{})
	if !ok {
		t.Fatalf("rendered missing or wrong type: %T", body["rendered"])
	}
	if len(arr) != 2 {
		t.Fatalf("rendered len=%d, want 2", len(arr))
	}
	first := arr[0].(map[string]interface{})
	if first["id"] != "p1" || first["brand"] != "COSRX" || first["marketing_claim"] != "for dry skin" {
		t.Errorf("p1 fields wrong: %v", first)
	}
	// p2 has no rating / no marketing_claim — those keys must be omitted
	// (omitempty), not present as zero/empty.
	second := arr[1].(map[string]interface{})
	for _, banned := range []string{"rating", "marketing_claim", "images", "price"} {
		if _, ok := second[banned]; ok {
			t.Errorf("p2 should omit empty %q, got %v", banned, second[banned])
		}
	}
}

func TestBuildRenderedSubset_OutOfRangeIndices(t *testing.T) {
	products := []domain.Product{
		{ID: "p0", Name: "Zero"},
		{ID: "p1", Name: "One"},
	}
	got := BuildRenderedSubset(products, []int{1, 5, -1, 0})
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2 (idx 5 and -1 silently dropped)", len(got))
	}
	if got[0].ID != "p1" || got[1].ID != "p0" {
		t.Errorf("order broken: %v", got)
	}
}

func TestBuildRenderedSubset_EmptyInputs(t *testing.T) {
	if BuildRenderedSubset(nil, []int{0}) != nil {
		t.Error("nil products → expect nil")
	}
	if BuildRenderedSubset([]domain.Product{{ID: "p"}}, nil) != nil {
		t.Error("nil indices → expect nil")
	}
}

// extractStateBody pulls the JSON object out of the "<state>\n%s\n</state>"
// envelope and parses it. Test helper only.
func extractStateBody(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	const head = "<state>\n"
	const tail = "\n</state>"
	i := strings.Index(s, head)
	j := strings.Index(s, tail)
	if i < 0 || j < 0 {
		t.Fatalf("envelope not found in %q", s)
	}
	body := s[i+len(head) : j]
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("state body not JSON: %v", err)
	}
	return out
}
