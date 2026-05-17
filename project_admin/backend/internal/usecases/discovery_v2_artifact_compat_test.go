package usecases

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"keepstar-admin/internal/domain"
)

func TestLiftV2ToV3_SingleBranch(t *testing.T) {
	now := time.Now().UTC()
	v2 := &domain.MappingArtifactV2{
		Version:  2,
		Vertical: "cosmetics",
		FieldMap: []domain.FieldMappingRule{
			{From: "title", To: "master.name"},
			{From: "vendor", To: "master.brand"},
		},
		Notes:   "legacy artifact",
		BuiltAt: now,
	}
	v3 := domain.LiftV2ToV3(v2)
	if v3 == nil {
		t.Fatal("LiftV2ToV3 returned nil")
	}
	if v3.Version != 3 {
		t.Errorf("version = %d, want 3", v3.Version)
	}
	if len(v3.Branches) != 1 {
		t.Fatalf("branches = %d, want 1", len(v3.Branches))
	}
	if v3.Branches[0].Vertical != "cosmetics" {
		t.Errorf("branch vertical = %q, want cosmetics", v3.Branches[0].Vertical)
	}
	if !reflect.DeepEqual(v3.Branches[0].FieldMap, v2.FieldMap) {
		t.Errorf("field_map mismatch:\n  got %+v\n  want %+v", v3.Branches[0].FieldMap, v2.FieldMap)
	}
	if v3.Notes != "legacy artifact" {
		t.Errorf("notes = %q, want 'legacy artifact'", v3.Notes)
	}
}

func TestLiftV2ToV3_EmptyVerticalBecomesUnknown(t *testing.T) {
	v2 := &domain.MappingArtifactV2{Version: 2, Vertical: ""}
	v3 := domain.LiftV2ToV3(v2)
	if v3.Branches[0].Vertical != "unknown" {
		t.Errorf("empty vertical → %q, want unknown", v3.Branches[0].Vertical)
	}
}

func TestLiftV2ToV3_NilReturnsNil(t *testing.T) {
	if v := domain.LiftV2ToV3(nil); v != nil {
		t.Errorf("nil input → %v, want nil", v)
	}
}

func TestFindBranch_ExactMatch(t *testing.T) {
	art := &domain.MappingArtifactV3{
		Branches: []domain.VerticalBranch{
			{Vertical: "cosmetics"},
			{Vertical: "electronics"},
			{Vertical: "unknown"},
		},
	}
	if b := art.FindBranch("electronics"); b == nil || b.Vertical != "electronics" {
		t.Errorf("electronics: got %+v", b)
	}
}

func TestFindBranch_CaseInsensitive(t *testing.T) {
	art := &domain.MappingArtifactV3{Branches: []domain.VerticalBranch{{Vertical: "Cosmetics"}}}
	if b := art.FindBranch("COSMETICS"); b == nil || b.Vertical != "Cosmetics" {
		t.Errorf("case-insensitive: got %+v", b)
	}
}

func TestFindBranch_FallbackToUnknown(t *testing.T) {
	art := &domain.MappingArtifactV3{
		Branches: []domain.VerticalBranch{
			{Vertical: "cosmetics"},
			{Vertical: "unknown"},
		},
	}
	b := art.FindBranch("ski")
	if b == nil || b.Vertical != "unknown" {
		t.Errorf("ski → fallback unknown: got %+v", b)
	}
}

func TestFindBranch_NoMatchNoFallback(t *testing.T) {
	art := &domain.MappingArtifactV3{Branches: []domain.VerticalBranch{{Vertical: "cosmetics"}}}
	if b := art.FindBranch("ski"); b != nil {
		t.Errorf("no fallback: got %+v, want nil", b)
	}
}

func TestFindBranch_NilArtifact(t *testing.T) {
	var art *domain.MappingArtifactV3
	if b := art.FindBranch("cosmetics"); b != nil {
		t.Errorf("nil artifact: got %+v, want nil", b)
	}
}

func TestVerticalNames(t *testing.T) {
	art := &domain.MappingArtifactV3{
		Branches: []domain.VerticalBranch{
			{Vertical: "cosmetics"},
			{Vertical: "electronics"},
			{Vertical: "unknown"},
		},
	}
	got := art.VerticalNames()
	want := []string{"cosmetics", "electronics", "unknown"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestPeekArtifactVersion(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"v2", `{"version":2,"vertical":"cosmetics"}`, 2},
		{"v3", `{"version":3,"branches":[]}`, 3},
		{"no version", `{"vertical":"cosmetics"}`, 0},
		{"malformed", `{not json`, 0},
		{"empty", ``, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := domain.PeekArtifactVersion([]byte(tc.body)); got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestMappingArtifactV3_JSONRoundtrip(t *testing.T) {
	original := &domain.MappingArtifactV3{
		Version: 3,
		Branches: []domain.VerticalBranch{
			{
				Vertical: "cosmetics",
				FieldMap: []domain.FieldMappingRule{
					{From: "title", To: "master.name"},
					{From: "metafields.skin.type", To: "cosmetics.skin_type", Transform: "split:,"},
				},
			},
			{
				Vertical: "unknown",
				FieldMap: []domain.FieldMappingRule{{From: "title", To: "master.name"}},
			},
		},
		ClassifyRules: []domain.ClassifyRule{
			{When: "brand = 'Apple'", ThenVertical: "electronics"},
		},
		Notes:   "two branches",
		BuiltAt: time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC),
	}
	body, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if domain.PeekArtifactVersion(body) != 3 {
		t.Errorf("PeekArtifactVersion on marshaled bytes: not 3")
	}
	var restored domain.MappingArtifactV3
	if err := json.Unmarshal(body, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(&restored, original) {
		t.Errorf("roundtrip mismatch:\n  got %+v\n  want %+v", &restored, original)
	}
}
