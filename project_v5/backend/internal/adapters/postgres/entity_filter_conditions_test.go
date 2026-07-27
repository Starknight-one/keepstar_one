package postgres

import (
	"strings"
	"testing"
	"time"

	"keepstar_v5/internal/domain"
)

// Entity-plane generic attr predicates follow the catalogFilterConditions
// discipline: keys are SnakeToCamel-normalized then identifier-gated before
// interpolation (they enter SQL text), values always travel as parameters,
// numeric bounds are jsonb_typeof-guarded, iteration order is sorted so the
// generated SQL is byte-stable.
func TestEntityFilterConditionsGenericAttrs(t *testing.T) {
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	conds, args, argNum := entityFilterConditions(domain.RecordFilter{
		Status:      "new",
		Since:       &since,
		RefEntityID: "3b241101-e2bb-4255-8caf-4136c566a962",
		Attrs:       map[string]string{"deal_type": "sale", "bad key; DROP": "x", "empty": ""},
		AttrMin:     map[string]float64{"beds": 2},
		AttrMax:     map[string]float64{"beds": 4},
	}, nil, 3)

	joined := strings.Join(conds, " AND ")
	for _, want := range []string{
		"status = $3",
		"created_at >= $4",
		"ref_entity_id = $5::uuid",
		"data->>'dealType' = $6", // snake_case in → camelCase key probed
		"jsonb_typeof(data->'dealType') = 'array'",
		"jsonb_typeof(data->'beds') = 'number'",
		"(data->>'beds')::numeric >= $7",
		"(data->>'beds')::numeric <= $8",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing predicate %q in: %s", want, joined)
		}
	}
	if strings.Contains(joined, "DROP") {
		t.Fatalf("unsafe key leaked into SQL: %s", joined)
	}
	if strings.Contains(joined, "empty") {
		t.Errorf("empty-valued attr should be dropped: %s", joined)
	}
	if len(args) != 6 || argNum != 9 {
		t.Errorf("args/argNum: %v %d", args, argNum)
	}
}

// Sort input is either a known column or an identifier-gated data key; junk
// falls back to the created_at index order and always carries the id
// tiebreak.
func TestEntitySortClause(t *testing.T) {
	cases := []struct{ field, order, want string }{
		{"", "", "created_at DESC, id"},
		{"createdAt", "asc", "created_at ASC, id"},
		// snake_case spellings from the query schema's sort enum
		// (entityQuerySchema sortFields lists "created_at") must hit the
		// real columns, not a nonexistent data->>'createdAt' key.
		{"created_at", "asc", "created_at ASC, id"},
		{"updated_at", "desc", "updated_at DESC, id"},
		{"updatedAt", "desc", "updated_at DESC, id"},
		{"status", "ASC", "status ASC, id"},
		{"preferred_time", "asc", "data->>'preferredTime' ASC, id"},
		{"beds", "", "data->>'beds' DESC, id"},
		{"x'; DROP TABLE--", "asc", "created_at ASC, id"},
	}
	for _, c := range cases {
		if got := entitySortClause(c.field, c.order); got != c.want {
			t.Errorf("entitySortClause(%q, %q) = %q, want %q", c.field, c.order, got, c.want)
		}
	}
}
