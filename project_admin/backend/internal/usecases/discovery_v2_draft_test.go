package usecases

import (
	"strings"
	"testing"

	"keepstar-admin/internal/domain"
)

// Tests for the builder-pattern validators in discoveryDraft. Each test
// names the failure mode it guards against — they exist so the contract
// can't silently regress.

// === set_classifying_field ===

func TestDraft_SetClassifyingField_NotInInbox_ReturnsTypoSuggestions(t *testing.T) {
	d := newDiscoveryDraft()
	err := d.setClassifyingField("ingredeints", // typo
		[]string{"ingredients", "product_type", "brand"},
		[]string{"Skincare", "Makeup"})
	if err == nil {
		t.Fatal("expected error for typoed field")
	}
	// Levenshtein should put "ingredients" closer than the others.
	if !strings.Contains(err.Error(), "ingredients") {
		t.Errorf("error should mention typo candidate 'ingredients': %v", err)
	}
}

func TestDraft_SetClassifyingField_NoStringSamples_Rejected(t *testing.T) {
	d := newDiscoveryDraft()
	// brand_id is in inbox, but the only samples are blank — rejected.
	err := d.setClassifyingField("brand_id",
		[]string{"brand_id", "name"},
		[]string{"", "  ", ""})
	if err == nil {
		t.Fatal("expected error when no string samples present")
	}
	if !strings.Contains(err.Error(), "string sample") &&
		!strings.Contains(err.Error(), "categorical") {
		t.Errorf("error should explain why field was rejected: %v", err)
	}
}

func TestDraft_SetClassifyingField_HappyPath(t *testing.T) {
	d := newDiscoveryDraft()
	err := d.setClassifyingField("primary_category",
		[]string{"primary_category", "product_name"},
		[]string{"Skincare", "Makeup"})
	if err != nil {
		t.Fatalf("happy path: %v", err)
	}
	if d.ClassifyingField != "primary_category" {
		t.Errorf("ClassifyingField = %q, want primary_category", d.ClassifyingField)
	}
}

// === add_branch ===

func TestDraft_AddBranch_UnknownVertical_Rejected(t *testing.T) {
	d := newDiscoveryDraft()
	err := d.addBranch("skincare") // not in allowedVerticals
	if err == nil {
		t.Fatal("expected error for unknown vertical")
	}
}

func TestDraft_AddBranch_Duplicate_Rejected(t *testing.T) {
	d := newDiscoveryDraft()
	_ = d.addBranch("cosmetics")
	err := d.addBranch("cosmetics")
	if err == nil {
		t.Fatal("expected error for duplicate branch")
	}
}

// === add_field_mapping ===

func TestDraft_AddFieldMapping_BranchMissing_Rejected(t *testing.T) {
	d := newDiscoveryDraft()
	err := d.addFieldMapping("cosmetics", "title", "master.name", "", "", "",
		[]string{"title"})
	if err == nil {
		t.Fatal("expected error when branch not declared")
	}
}

func TestDraft_AddFieldMapping_FromTypo_ReturnsSuggestions(t *testing.T) {
	d := newDiscoveryDraft()
	_ = d.addBranch("cosmetics")
	err := d.addFieldMapping("cosmetics", "ingredeints", "tier3.ingredients", "", "", "",
		[]string{"ingredients", "title", "brand"})
	if err == nil {
		t.Fatal("expected error for typoed from")
	}
	if !strings.Contains(err.Error(), "ingredients") {
		t.Errorf("error should suggest 'ingredients': %v", err)
	}
}

func TestDraft_AddFieldMapping_PhantomMasterColumn_Rejected(t *testing.T) {
	d := newDiscoveryDraft()
	_ = d.addBranch("cosmetics")
	err := d.addFieldMapping("cosmetics", "title", "master.foobar", "", "", "",
		[]string{"title"})
	if err == nil {
		t.Fatal("expected error for unknown master column")
	}
}

func TestDraft_AddFieldMapping_CosmeticsOutsideBranch_Rejected(t *testing.T) {
	d := newDiscoveryDraft()
	_ = d.addBranch("electronics")
	err := d.addFieldMapping("electronics", "skin_type", "cosmetics.skin_type", "", "", "",
		[]string{"skin_type"})
	if err == nil {
		t.Fatal("expected error for cosmetics.* target outside cosmetics branch")
	}
}

func TestDraft_AddFieldMapping_SplitOnScalarTarget_Rejected(t *testing.T) {
	d := newDiscoveryDraft()
	_ = d.addBranch("cosmetics")
	// split returns []string; master.name is text scalar — incompatible.
	err := d.addFieldMapping("cosmetics", "title", "master.name", "split", ",", "",
		[]string{"title"})
	if err == nil {
		t.Fatal("expected error for split transform on scalar target")
	}
}

func TestDraft_AddFieldMapping_SplitWithoutDelim_Rejected(t *testing.T) {
	d := newDiscoveryDraft()
	_ = d.addBranch("cosmetics")
	err := d.addFieldMapping("cosmetics", "ingredients", "cosmetics.key_ingredients", "split", "", "",
		[]string{"ingredients"})
	if err == nil {
		t.Fatal("expected error when split_delim is empty")
	}
}

func TestDraft_AddFieldMapping_NumericOnTextMaster_Rejected(t *testing.T) {
	d := newDiscoveryDraft()
	_ = d.addBranch("cosmetics")
	// transform=numeric produces a number; master.name is text.
	err := d.addFieldMapping("cosmetics", "price", "master.name", "numeric", "", "",
		[]string{"price"})
	if err == nil {
		t.Fatal("expected error for numeric transform on text master column")
	}
}

func TestDraft_AddFieldMapping_DuplicateFromTo_Rejected(t *testing.T) {
	d := newDiscoveryDraft()
	_ = d.addBranch("cosmetics")
	if err := d.addFieldMapping("cosmetics", "title", "master.name", "", "", "",
		[]string{"title"}); err != nil {
		t.Fatalf("first mapping should succeed: %v", err)
	}
	err := d.addFieldMapping("cosmetics", "title", "master.name", "", "", "",
		[]string{"title"})
	if err == nil {
		t.Fatal("expected error for duplicate mapping")
	}
}

func TestDraft_AddFieldMapping_HappyPath_SplitOnArray(t *testing.T) {
	d := newDiscoveryDraft()
	_ = d.addBranch("cosmetics")
	err := d.addFieldMapping("cosmetics", "ingredients", "cosmetics.key_ingredients", "split", ",", "",
		[]string{"ingredients"})
	if err != nil {
		t.Fatalf("split→cosmetics array should succeed: %v", err)
	}
	// Canonical transform string is folded with the delim.
	if len(d.Branches[0].FieldMap) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(d.Branches[0].FieldMap))
	}
	if d.Branches[0].FieldMap[0].Transform != "split:," {
		t.Errorf("transform stored as %q, want split:,", d.Branches[0].FieldMap[0].Transform)
	}
}

// === add_classify_rule ===

func TestDraft_AddClassifyRule_ThenVerticalWithoutBranch_Rejected(t *testing.T) {
	d := newDiscoveryDraft()
	_ = d.addBranch("cosmetics")
	// thenVertical=haircare — no branch declared.
	err := d.addClassifyRule("product_type", "contains", "shampoo", "haircare", nil)
	if err == nil {
		t.Fatal("expected error: then_vertical refers to undeclared branch")
	}
}

func TestDraft_AddClassifyRule_DeadRule_Rejected(t *testing.T) {
	d := newDiscoveryDraft()
	_ = d.addBranch("cosmetics")
	// Value 'serum' matches none of the supplied samples.
	err := d.addClassifyRule("product_type", "contains", "serum", "cosmetics",
		[]string{"Cream", "Toner", "Mask"})
	if err == nil {
		t.Fatal("expected error: dead rule matches 0 samples")
	}
}

func TestDraft_AddClassifyRule_HappyPath_StoresCanonicalDSL(t *testing.T) {
	d := newDiscoveryDraft()
	_ = d.addBranch("cosmetics")
	err := d.addClassifyRule("product_type", "contains", "Skincare", "cosmetics",
		[]string{"Skincare", "Makeup"})
	if err != nil {
		t.Fatalf("happy path: %v", err)
	}
	if len(d.ClassifyRules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(d.ClassifyRules))
	}
	want := "product_type contains 'Skincare'"
	if d.ClassifyRules[0].When != want {
		t.Errorf("When = %q, want %q (canonical DSL with single quotes)", d.ClassifyRules[0].When, want)
	}
}

// === Finalize ===

func TestDraft_Finalize_ReturnsV3Artifact(t *testing.T) {
	d := newDiscoveryDraft()
	_ = d.setClassifyingField("product_type",
		[]string{"product_type", "title"},
		[]string{"Cream"})
	_ = d.addBranch("cosmetics")
	_ = d.addFieldMapping("cosmetics", "title", "master.name", "", "", "",
		[]string{"product_type", "title"})

	art := d.Finalize()
	if art == nil {
		t.Fatal("Finalize returned nil")
	}
	if art.Version != 3 {
		t.Errorf("version = %d, want 3", art.Version)
	}
	if art.ClassifyingField != "product_type" {
		t.Errorf("classifying_field = %q, want product_type", art.ClassifyingField)
	}
	if len(art.Branches) != 1 || art.Branches[0].Vertical != "cosmetics" {
		t.Errorf("branches = %+v, want one cosmetics branch", art.Branches)
	}
}

// === newDiscoveryDraftFromArtifact (B1 — discovery patch flow) ===

func TestDraft_FromArtifact_RoundTripsViaFinalize(t *testing.T) {
	src := &domain.MappingArtifactV3{
		Version:          3,
		ClassifyingField: "product_type",
		Branches: []domain.VerticalBranch{
			{
				Vertical: "cosmetics",
				FieldMap: []domain.FieldMappingRule{
					{From: "title", To: "master.name"},
					{From: "brand", To: "master.brand"},
				},
			},
			{
				Vertical: "haircare",
				FieldMap: []domain.FieldMappingRule{
					{From: "ingredients", To: "tier3.ingredients", Transform: "split", Default: ""},
				},
			},
		},
		ClassifyRules: []domain.ClassifyRule{
			{When: "product_type contains 'Skincare'", ThenVertical: "cosmetics"},
			{When: "product_type contains 'Hair'", ThenVertical: "haircare"},
			{When: "product_type contains 'Body'", ThenVertical: "cosmetics"},
		},
		Notes: "seeded by alpha-0.9.4 round-trip test",
	}

	draft := newDiscoveryDraftFromArtifact(src)
	if draft == nil {
		t.Fatal("newDiscoveryDraftFromArtifact returned nil")
	}
	if draft.ClassifyingField != "product_type" {
		t.Errorf("ClassifyingField = %q, want product_type", draft.ClassifyingField)
	}
	if len(draft.Branches) != 2 || len(draft.ClassifyRules) != 3 {
		t.Errorf("draft has %d branches / %d rules, want 2 / 3", len(draft.Branches), len(draft.ClassifyRules))
	}

	out := draft.Finalize()
	if out.ClassifyingField != src.ClassifyingField {
		t.Errorf("round-trip ClassifyingField %q != %q", out.ClassifyingField, src.ClassifyingField)
	}
	if len(out.Branches) != len(src.Branches) {
		t.Errorf("round-trip branches %d != %d", len(out.Branches), len(src.Branches))
	}
	if len(out.ClassifyRules) != len(src.ClassifyRules) {
		t.Errorf("round-trip rules %d != %d", len(out.ClassifyRules), len(src.ClassifyRules))
	}
	if out.Notes != src.Notes {
		t.Errorf("round-trip Notes %q != %q", out.Notes, src.Notes)
	}
}

func TestDraft_FromArtifact_NilArtifact_FallsBackToEmpty(t *testing.T) {
	draft := newDiscoveryDraftFromArtifact(nil)
	if draft == nil {
		t.Fatal("expected empty draft, got nil")
	}
	if draft.ClassifyingField != "" || len(draft.Branches) != 0 || len(draft.ClassifyRules) != 0 {
		t.Errorf("nil artifact should produce empty draft, got %+v", draft)
	}
	if draft.toolErrorCount == nil {
		t.Error("toolErrorCount must be initialized to empty map")
	}
}

func TestDraft_FromArtifact_DeepCopiesSlices(t *testing.T) {
	src := &domain.MappingArtifactV3{
		Branches: []domain.VerticalBranch{
			{Vertical: "cosmetics", FieldMap: []domain.FieldMappingRule{{From: "title", To: "master.name"}}},
		},
		ClassifyRules: []domain.ClassifyRule{{When: "x", ThenVertical: "cosmetics"}},
	}
	draft := newDiscoveryDraftFromArtifact(src)

	// Mutate draft slices; src must remain intact.
	draft.Branches = append(draft.Branches, domain.VerticalBranch{Vertical: "extra"})
	draft.ClassifyRules = append(draft.ClassifyRules, domain.ClassifyRule{When: "y", ThenVertical: "extra"})

	if len(src.Branches) != 1 {
		t.Errorf("src.Branches mutated: len = %d, want 1", len(src.Branches))
	}
	if len(src.ClassifyRules) != 1 {
		t.Errorf("src.ClassifyRules mutated: len = %d, want 1", len(src.ClassifyRules))
	}
}

// === Helper: levenshtein sanity check ===

func TestLevenshtein_ExpectedDistances(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"ingredients", "ingredeints", 2}, // swap two chars
		{"product_type", "product_type", 0},
		{"a", "abc", 2},
		{"", "abc", 3},
	}
	for _, c := range cases {
		got := levenshtein(c.a, c.b)
		if got != c.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
