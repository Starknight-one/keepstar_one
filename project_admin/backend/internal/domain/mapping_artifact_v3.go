package domain

import (
	"encoding/json"
	"strings"
	"time"
)

// MappingArtifactV3 is the multi-vertical artifact format. Where v2 had a
// single Vertical + flat FieldMap (locking the whole tenant to one class),
// v3 carries one Branch per vertical class plus tenant-wide ClassifyRules
// the agent built to disambiguate ambiguous rows.
//
// Read path is permissive: the adapter accepts v2 input and lifts it into a
// one-branch v3 in memory. Write path always emits v3.
//
// Stored in catalog.tenant_catalog_schema.mapping_artifact JSONB.
type MappingArtifactV3 struct {
	Version int `json:"version"` // 3
	// ClassifyingField names the inbox top-level key that holds the
	// tenant's category value (e.g. "product_type" for Shopify,
	// "primary_category" for Sephora CSV). Apply_v2's classifyContextFromRaw
	// reads raw[ClassifyingField] as the input to vertical lookup and
	// classify_rules. Discovery_v2 picks this in its Step 1 by inspecting
	// list_fields + sample values. Empty → legacy artifact, apply_v2 falls
	// back to a hardcoded synonym list (product_type / productType /
	// primary_category / category / type).
	ClassifyingField string           `json:"classifying_field,omitempty"`
	Branches         []VerticalBranch `json:"branches"`
	ClassifyRules    []ClassifyRule   `json:"classify_rules,omitempty"`
	Notes            string           `json:"notes,omitempty"`
	BuiltAt          time.Time        `json:"built_at"`
}

// VerticalBranch is the per-class rule set. apply_v2 picks one branch per
// inbox row based on the classification cascade. When no branch matches an
// inbox row's vertical, apply_v2 falls back to the "unknown" branch if the
// agent committed one, otherwise it routes raw fields to tier3 with a
// warning.
//
// FieldMap entries follow the same target prefix grammar as v2:
//   master.<col>     → Tier 1 master_products column
//   cosmetics.<col>  → Tier 2 master_cosmetics column (only valid when Vertical='cosmetics')
//   tier3.<key>      → master_products.tier3 JSONB pocket
//
// (Future per-vertical typed tables — master_electronics, master_furniture
// — extend this set; until those tables exist agents emit tier3.*.)
type VerticalBranch struct {
	Vertical string             `json:"vertical"`
	FieldMap []FieldMappingRule `json:"field_map"`
}

// ClassifyRule is the tiny DSL apply_v2 evaluates to decide a row's vertical
// when the alias lookup misses. The evaluator (matchesRule in
// classify_vertical.go) is the canonical reference for syntax.
type ClassifyRule struct {
	When         string `json:"when"`          // "product_type contains 'sofa'" | "brand = 'Apple'" | "name contains 'serum'" | "tag = 'sale'"
	ThenVertical string `json:"then_vertical"` // class produced when When matches
}

// FindBranch returns the branch matching vertical (case-insensitive). When
// no exact branch exists, falls back to the "unknown" branch if the agent
// committed one. Returns nil only when the artifact is empty or built
// without an unknown fallback for an unknown row.
func (a *MappingArtifactV3) FindBranch(vertical string) *VerticalBranch {
	if a == nil {
		return nil
	}
	for i := range a.Branches {
		if strings.EqualFold(a.Branches[i].Vertical, vertical) {
			return &a.Branches[i]
		}
	}
	for i := range a.Branches {
		if strings.EqualFold(a.Branches[i].Vertical, "unknown") {
			return &a.Branches[i]
		}
	}
	return nil
}

// VerticalNames returns the list of branch verticals — useful for action_log
// and curator UI summaries.
func (a *MappingArtifactV3) VerticalNames() []string {
	if a == nil {
		return nil
	}
	out := make([]string, 0, len(a.Branches))
	for _, b := range a.Branches {
		out = append(out, b.Vertical)
	}
	return out
}

// LiftV2ToV3 converts a legacy v2 artifact into a v3 with one branch. Used
// by the adapter Read path so apply_v2 never has to special-case v2.
func LiftV2ToV3(v2 *MappingArtifactV2) *MappingArtifactV3 {
	if v2 == nil {
		return nil
	}
	vertical := v2.Vertical
	if vertical == "" {
		vertical = "unknown"
	}
	return &MappingArtifactV3{
		Version: 3,
		Branches: []VerticalBranch{
			{Vertical: vertical, FieldMap: append([]FieldMappingRule{}, v2.FieldMap...)},
		},
		Notes:   v2.Notes,
		BuiltAt: v2.BuiltAt,
	}
}

// versionProbe is the minimum JSON shape needed to decide which struct to
// unmarshal into. Adapter calls this on the raw artifact bytes.
type versionProbe struct {
	Version int `json:"version"`
}

// PeekArtifactVersion returns the version field of a serialized artifact, or
// 0 when the bytes don't contain a version property. Used by the adapter
// to dispatch between v2 and v3 readers.
func PeekArtifactVersion(body []byte) int {
	var p versionProbe
	if err := json.Unmarshal(body, &p); err != nil {
		return 0
	}
	return p.Version
}
