package postgres

import (
	"strings"
	"testing"

	"keepstar_v5/internal/ports"
)

// Generic tier2 predicates: a camelCase attr must probe both spellings and
// both scalar/array shapes; numeric bounds must be typeof-guarded; unsafe
// keys must be dropped entirely (they are interpolated into SQL text).
func TestCatalogFilterConditionsGenericAttrs(t *testing.T) {
	conds, args, argNum := catalogFilterConditions(ports.ProductFilter{
		Attrs:   map[string]string{"dealType": "sale", "bad key; DROP": "x"},
		AttrMin: map[string]float64{"rooms": 2},
		AttrMax: map[string]float64{"rooms": 4},
	}, nil, 1)

	joined := strings.Join(conds, " AND ")
	for _, want := range []string{
		"mp.tier2->>'dealType' = $1",
		"mp.tier2->>'deal_type' = $1",
		"jsonb_typeof(mp.tier2->'rooms') = 'number'",
		"(mp.tier2->>'rooms')::numeric >= $2",
		"(mp.tier2->>'rooms')::numeric <= $3",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing predicate %q in: %s", want, joined)
		}
	}
	if strings.Contains(joined, "DROP") {
		t.Fatalf("unsafe key leaked into SQL: %s", joined)
	}
	if len(args) != 3 || argNum != 4 {
		t.Errorf("args/argNum: %v %d", args, argNum)
	}
}
