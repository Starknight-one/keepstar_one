package domain

import "time"

// MappingArtifactV2 is the new (post-rebuild) per-tenant mapping artifact.
// One discovery_v2 run produces one of these; apply_v2 consumes it to
// translate every inbox_item into master_products + per-vertical row.
//
// Stored in catalog.tenant_catalog_schema.mapping_artifact JSONB. Version=2
// distinguishes from legacy MappingArtifact (Version=1) — old struct stays
// in the codebase until apply_v2 fully replaces merge_apply.
//
// The shape is intentionally lean:
//   - vertical pins which per-vertical table apply_v2 should write to
//   - field_map is an ordered list of (from, to, transform) rules
//   - no junk rules, no brand mapping, no match strategy config — those
//     were curator/promotion features which the rebuild dropped.
type MappingArtifactV2 struct {
	Version  int                `json:"version"`           // 2
	Vertical string             `json:"vertical"`          // cosmetics | electronics | furniture | apparel | unknown
	FieldMap []FieldMappingRule `json:"field_map"`
	Notes    string             `json:"notes,omitempty"`   // agent's free-form summary
	BuiltAt  time.Time          `json:"built_at"`
}

// FieldMappingRule routes one source-side JSONB path to one of our targets.
//
// From: dotted path into inbox_items.raw (JSONB). Top-level only for v1;
// future versions can support nested with jsonpath.
//
// To: one of these forms —
//   master.<column>     → master_products column (name, brand, description, vertical, sku, ...)
//   <vertical>.<column> → per-vertical table column (e.g. cosmetics.skin_type)
//   tier3.<key>         → master_products.tier3 JSONB key (fallback for non-mapped attributes)
//
// Transform: optional value normalization. Supported in apply_v2:
//   lowercase             → strings.ToLower
//   trim                  → strings.TrimSpace
//   split:<delim>         → strings.Split (returns array — only valid for *.text[] targets)
//   ml_from_string        → parse "30 ml" → 30 (int, ml)
//   g_from_string         → parse "200 g" → 200
//   bool_from_yesno       → "yes"/"true"/"1" → true; else false
//   int                   → strconv.Atoi
//   numeric               → ParseFloat
//
// Default: literal value used when From is missing in the source row.
type FieldMappingRule struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Transform string `json:"transform,omitempty"`
	Default   string `json:"default,omitempty"`
}
