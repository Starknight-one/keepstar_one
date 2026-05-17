package usecases

import (
	"context"
	"errors"
	"testing"
)

// fakeAliasLookup is a minimal in-memory VerticalAliasLookup. The real one
// hits catalog.vertical_aliases — we replicate that contract in tests so
// we don't take a DB dependency on a pure-Go classifier.
type fakeAliasLookup struct {
	table map[string]string // lowercased product_type → vertical
	err   error
}

func (f *fakeAliasLookup) LookupVertical(_ context.Context, productType string) (string, bool, error) {
	if f.err != nil {
		return "", false, f.err
	}
	v, ok := f.table[lower(productType)]
	return v, ok, nil
}

func lower(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			r += 32
		}
		out = append(out, r)
	}
	return string(out)
}

func TestClassifyVertical_AliasHit(t *testing.T) {
	lookup := &fakeAliasLookup{table: map[string]string{
		"laptop":   "electronics",
		"skincare": "cosmetics",
	}}
	v, src := ClassifyVertical(context.Background(), lookup, nil, ClassifyContext{
		ProductType: "Laptop",
	})
	if v != "electronics" || src != ClassifySourceAlias {
		t.Fatalf("alias hit: got (%q, %q), want (electronics, alias)", v, src)
	}
}

func TestClassifyVertical_AliasMissThenRuleHit(t *testing.T) {
	lookup := &fakeAliasLookup{table: map[string]string{}}
	rules := []ClassifyRule{
		{When: "brand = 'Apple'", ThenVertical: "electronics"},
	}
	v, src := ClassifyVertical(context.Background(), lookup, rules, ClassifyContext{
		ProductType: "Doodad",
		Brand:       "apple",
	})
	if v != "electronics" || src != ClassifySourceRule {
		t.Fatalf("rule hit: got (%q, %q), want (electronics, rule)", v, src)
	}
}

func TestClassifyVertical_AllMiss(t *testing.T) {
	lookup := &fakeAliasLookup{table: map[string]string{}}
	rules := []ClassifyRule{
		{When: "brand = 'Nike'", ThenVertical: "apparel"},
	}
	v, src := ClassifyVertical(context.Background(), lookup, rules, ClassifyContext{
		ProductType: "Tool",
		Brand:       "Acme",
	})
	if v != "unknown" || src != ClassifySourceUnknown {
		t.Fatalf("all miss: got (%q, %q), want (unknown, unknown)", v, src)
	}
}

func TestClassifyVertical_EmptyProductTypeAndNoRules(t *testing.T) {
	lookup := &fakeAliasLookup{table: map[string]string{"laptop": "electronics"}}
	v, src := ClassifyVertical(context.Background(), lookup, nil, ClassifyContext{
		ProductType: "",
	})
	if v != "unknown" || src != ClassifySourceUnknown {
		t.Fatalf("empty pt: got (%q, %q), want (unknown, unknown)", v, src)
	}
}

func TestClassifyVertical_AliasPriorityOverRule(t *testing.T) {
	lookup := &fakeAliasLookup{table: map[string]string{"laptop": "electronics"}}
	rules := []ClassifyRule{
		{When: "product_type contains 'laptop'", ThenVertical: "furniture"},
	}
	v, src := ClassifyVertical(context.Background(), lookup, rules, ClassifyContext{
		ProductType: "Laptop",
	})
	if v != "electronics" || src != ClassifySourceAlias {
		t.Fatalf("alias should win over rule: got (%q, %q)", v, src)
	}
}

func TestClassifyVertical_LookupErrorFallsThroughToRules(t *testing.T) {
	lookup := &fakeAliasLookup{err: errors.New("db down")}
	rules := []ClassifyRule{
		{When: "brand = 'Apple'", ThenVertical: "electronics"},
	}
	v, src := ClassifyVertical(context.Background(), lookup, rules, ClassifyContext{
		ProductType: "Laptop", Brand: "Apple",
	})
	if v != "electronics" || src != ClassifySourceRule {
		t.Fatalf("lookup err: got (%q, %q), want (electronics, rule)", v, src)
	}
}

func TestMatchesRule(t *testing.T) {
	cases := []struct {
		name string
		when string
		ctx  ClassifyContext
		want bool
	}{
		{"product_type contains hit",
			"product_type contains 'sofa'",
			ClassifyContext{ProductType: "Modern Sofa"}, true},
		{"product_type contains miss",
			"product_type contains 'sofa'",
			ClassifyContext{ProductType: "Chair"}, false},
		{"product_type contains case-insensitive",
			"product_type contains 'SOFA'",
			ClassifyContext{ProductType: "modern sofa"}, true},
		{"brand exact",
			"brand = 'Apple'",
			ClassifyContext{Brand: "apple"}, true},
		{"brand exact with whitespace",
			"brand = 'Apple'",
			ClassifyContext{Brand: "  Apple  "}, true},
		{"brand exact miss",
			"brand = 'Apple'",
			ClassifyContext{Brand: "Samsung"}, false},
		{"name contains",
			"name contains 'serum'",
			ClassifyContext{Name: "Niacinamide Serum 10%"}, true},
		{"tag exact in slice",
			"tag = 'new'",
			ClassifyContext{Tags: []string{"sale", "new"}}, true},
		{"tag exact miss",
			"tag = 'new'",
			ClassifyContext{Tags: []string{"sale"}}, false},
		{"unknown DSL returns false",
			"product_type startswith 'laptop'",
			ClassifyContext{ProductType: "laptop pro"}, false},
		{"unquoted literal returns false",
			"brand = Apple",
			ClassifyContext{Brand: "Apple"}, false},
		{"empty when returns false",
			"",
			ClassifyContext{Brand: "Apple"}, false},
		{"double quotes also work",
			`brand = "Apple"`,
			ClassifyContext{Brand: "Apple"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesRule(tc.when, tc.ctx)
			if got != tc.want {
				t.Errorf("matchesRule(%q) = %v, want %v", tc.when, got, tc.want)
			}
		})
	}
}
