package domain

import (
	"encoding/json"
	"testing"
)

func TestComponentJSONRoundTrip(t *testing.T) {
	docBody := `{"version":"2.10","children":[{"type":"frame","id":"price-rating-root","children":[]}]}`
	src := Component{
		ID:           "comp-uuid",
		TenantID:     "tenant-uuid",
		Name:         "price_rating",
		Category:     "atom",
		Version:      1,
		Status:       ComponentStatusPublished,
		DocumentJSON: json.RawMessage(docBody),
	}
	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var dst Component
	if err := json.Unmarshal(raw, &dst); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dst.Name != "price_rating" || dst.Status != ComponentStatusPublished {
		t.Errorf("metadata lost: %+v", dst)
	}
	var parsed map[string]any
	if err := json.Unmarshal(dst.DocumentJSON, &parsed); err != nil {
		t.Fatalf("doc body not valid JSON after round-trip: %v", err)
	}
}
