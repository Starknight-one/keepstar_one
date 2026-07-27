package engine

import (
	"testing"
	"time"

	"keepstar_v5/internal/domain"
)

func leadSet() *domain.EntitySet {
	return &domain.EntitySet{
		Slug: "lead",
		Name: "Lead",
		Fields: []domain.FieldDef{
			{Key: "name", Label: "Name", Type: domain.FieldText},
			{Key: "phone", Label: "Phone", Type: domain.FieldPhone},
			{Key: "budget", Label: "Budget", Type: domain.FieldMoney},
			{Key: "status", Label: "Status", Type: domain.FieldEnum, ValueSetRef: "lead_pipeline"},
			{Key: "preferredTime", Label: "Preferred time", Type: domain.FieldDatetime},
		},
		Labels: map[string]map[string]string{
			"lead_pipeline": {"new": "New", "contacted": "Contacted"},
		},
	}
}

// TestEntityToMapSystemKeys — the binding contract every entity preset can
// rely on: id (InjectDefaultActions contract), entity, status(+Label),
// refEntityId, createdAt(+Formatted).
func TestEntityToMapSystemKeys(t *testing.T) {
	created := time.Date(2026, 7, 27, 14, 30, 0, 0, time.UTC)
	rec := domain.EntityRecord{
		ID:          "rec-1",
		EntitySlug:  "lead",
		Status:      "new",
		RefEntityID: "prod-9",
		CreatedAt:   created,
		Data:        map[string]any{"name": "Ana", "refTitle": "2BR Apartment"},
	}
	m := EntityToMap(rec, leadSet())

	if m["id"] != "rec-1" {
		t.Errorf("id = %v, want rec-1", m["id"])
	}
	if m["entity"] != "lead" {
		t.Errorf("entity = %v, want lead", m["entity"])
	}
	if m["status"] != "new" {
		t.Errorf("status = %v, want new", m["status"])
	}
	if m["statusLabel"] != "New" {
		t.Errorf("statusLabel = %v, want New (value-set label)", m["statusLabel"])
	}
	if m["refEntityId"] != "prod-9" {
		t.Errorf("refEntityId = %v, want prod-9", m["refEntityId"])
	}
	if m["refTitle"] != "2BR Apartment" {
		t.Errorf("refTitle = %v, want denormalized pass-through", m["refTitle"])
	}
	if m["createdAt"] != "2026-07-27T14:30:00Z" {
		t.Errorf("createdAt = %v, want RFC3339", m["createdAt"])
	}
	if m["createdAtFormatted"] != "Jul 27, 2026 2:30 PM" {
		t.Errorf("createdAtFormatted = %v", m["createdAtFormatted"])
	}
}

// TestEntityToMapDerivedKeys — money → <key>Formatted ("$1,250"), enum →
// <key>Label, datetime → <key>Formatted. Cents may arrive as float64
// (JSONB round-trip) or int.
func TestEntityToMapDerivedKeys(t *testing.T) {
	rec := domain.EntityRecord{
		ID:         "rec-2",
		EntitySlug: "lead",
		Status:     "contacted",
		Data: map[string]any{
			"budget":        float64(125000), // cents → "$1,250"
			"status":        "contacted",
			"preferredTime": "2026-08-01T15:00:00Z",
		},
	}
	m := EntityToMap(rec, leadSet())

	if m["budgetFormatted"] != "$1,250" {
		t.Errorf("budgetFormatted = %v, want $1,250", m["budgetFormatted"])
	}
	if m["statusLabel"] != "Contacted" {
		t.Errorf("statusLabel = %v, want Contacted", m["statusLabel"])
	}
	if m["preferredTimeFormatted"] != "Aug 1, 2026 3:00 PM" {
		t.Errorf("preferredTimeFormatted = %v", m["preferredTimeFormatted"])
	}
	// Raw values survive next to their derived twins.
	if m["budget"] != float64(125000) {
		t.Errorf("budget raw = %v, want 125000", m["budget"])
	}
	if m["preferredTime"] != "2026-08-01T15:00:00Z" {
		t.Errorf("preferredTime raw = %v", m["preferredTime"])
	}
}

// TestEntityToMapCamelCaseDiscipline — Data keys route through SnakeToCamel
// (THE only normalizer, §6.2) and a data key can never shadow a system key.
func TestEntityToMapCamelCaseDiscipline(t *testing.T) {
	rec := domain.EntityRecord{
		ID:         "rec-3",
		EntitySlug: "lead",
		Data: map[string]any{
			"preferred_time": "2026-08-01T15:00", // snake in the wild
			"id":             "vendor-junk",      // must NOT shadow record id
		},
	}
	m := EntityToMap(rec, leadSet())

	if _, exists := m["preferred_time"]; exists {
		t.Error("snake_case key survived — must normalise to camelCase")
	}
	if m["preferredTime"] != "2026-08-01T15:00" {
		t.Errorf("preferredTime = %v", m["preferredTime"])
	}
	// Zone-less datetime still derives Formatted.
	if m["preferredTimeFormatted"] != "Aug 1, 2026 3:00 PM" {
		t.Errorf("preferredTimeFormatted = %v", m["preferredTimeFormatted"])
	}
	if m["id"] != "rec-3" {
		t.Errorf("id = %v — record id must win over data key", m["id"])
	}
}

// TestEntityToMapNilSet — synthetic / definition-less sets (R23) still
// flatten: system keys + data pass-through, no derived keys, statusLabel
// falls back to the raw status.
func TestEntityToMapNilSet(t *testing.T) {
	rec := domain.EntityRecord{
		ID:         "rec-4",
		EntitySlug: "opCard",
		Status:     "new",
		Data:       map[string]any{"does": "creates a lead", "budget": float64(5000)},
	}
	m := EntityToMap(rec, nil)

	if m["does"] != "creates a lead" {
		t.Errorf("does = %v", m["does"])
	}
	if m["statusLabel"] != "new" {
		t.Errorf("statusLabel = %v, want raw fallback", m["statusLabel"])
	}
	if _, exists := m["budgetFormatted"]; exists {
		t.Error("derived key present without a definition snapshot")
	}
}

// TestEntityToMapUnknownEnumValue — a status outside the value set binds
// its raw value as the label rather than blank (presets never render empty
// badges).
func TestEntityToMapUnknownEnumValue(t *testing.T) {
	rec := domain.EntityRecord{
		ID:         "rec-5",
		EntitySlug: "lead",
		Status:     "ghosted",
		Data:       map[string]any{"status": "ghosted"},
	}
	m := EntityToMap(rec, leadSet())
	if m["statusLabel"] != "ghosted" {
		t.Errorf("statusLabel = %v, want raw fallback", m["statusLabel"])
	}
}

func TestFormatUSDCents(t *testing.T) {
	cases := []struct {
		cents int
		want  string
	}{
		{125000, "$1,250"},
		{99, "$0"},
		{100, "$1"},
		{123456789, "$1,234,567"},
		{-125000, "-$1,250"},
		{0, "$0"},
	}
	for _, c := range cases {
		if got := formatUSDCents(c.cents); got != c.want {
			t.Errorf("formatUSDCents(%d) = %q, want %q", c.cents, got, c.want)
		}
	}
}
