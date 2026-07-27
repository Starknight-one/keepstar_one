package engine

import (
	"fmt"
	"strings"
	"time"

	"keepstar_v5/internal/domain"
)

// entityDatetimeLayout is the human-readable form of the derived
// `<key>Formatted` / `createdAtFormatted` datetime keys.
const entityDatetimeLayout = "Jan 2, 2006 3:04 PM"

// EntityToMap flattens an EntityRecord into a single map[string]any for
// binding — the entity-plane sibling of ProductToMap (RUNTIME_SPEC.md §4.5).
// Entity sets bind through the exact same engine.BindData path as products;
// this map IS the binding vocabulary entity presets are authored against.
//
// Layers, earlier wins on key collision:
//
//  1. System keys — the contract every entity preset can rely on:
//     `id` (what InjectDefaultActions resolves the action entity from),
//     `entity` (slug), `status`, `statusLabel`, `refEntityId`,
//     `createdAt` (+`createdAtFormatted`).
//  2. Record Data — keys normalised through SnakeToCamel (THE only
//     normalizer, §6.2). Denormalized catalog keys (refTitle, refImage)
//     ride here, written by EntityWrite at create time.
//  3. Derived keys from the definition snapshot: money → `<key>Formatted`
//     ("$1,250"), enum → `<key>Label` (value-set label), datetime →
//     `<key>Formatted`.
//
// set may be nil (or carry no Fields — synthetic sets, R23): the record
// still flattens, only the definition-derived keys are skipped.
func EntityToMap(r domain.EntityRecord, set *domain.EntitySet) map[string]any {
	m := make(map[string]any)

	// 1. System keys.
	if r.ID != "" {
		m["id"] = r.ID
	}
	if r.EntitySlug != "" {
		m["entity"] = r.EntitySlug
	}
	if r.Status != "" {
		m["status"] = r.Status
		m["statusLabel"] = entityStatusLabel(r.Status, set)
	}
	if r.RefEntityID != "" {
		m["refEntityId"] = r.RefEntityID
	}
	if !r.CreatedAt.IsZero() {
		m["createdAt"] = r.CreatedAt.Format(time.RFC3339)
		m["createdAtFormatted"] = r.CreatedAt.Format(entityDatetimeLayout)
	}

	// 2. Record data — fills only missing keys so the system contract
	// (`id` above all) can never be shadowed by a data key.
	for k, v := range r.Data {
		ck := SnakeToCamel(k)
		if _, exists := m[ck]; !exists {
			m[ck] = v
		}
	}

	if set == nil {
		return m
	}

	// 3. Derived keys from the definition snapshot.
	for _, f := range set.Fields {
		key := SnakeToCamel(f.Key)
		raw, ok := m[key]
		if !ok || raw == nil {
			continue
		}
		switch f.Type {
		case domain.FieldMoney:
			if cents, ok := toIntCents(raw); ok {
				m[key+"Formatted"] = formatUSDCents(cents)
			}
		case domain.FieldEnum:
			if f.ValueSetRef == "" {
				continue
			}
			value := fmt.Sprintf("%v", raw)
			label := value
			if l, ok := set.Labels[f.ValueSetRef][value]; ok && l != "" {
				label = l
			}
			m[key+"Label"] = label
		case domain.FieldDatetime:
			if s, ok := raw.(string); ok {
				if t, ok := parseEntityDatetime(s); ok {
					m[key+"Formatted"] = t.Format(entityDatetimeLayout)
				}
			}
		}
	}

	return m
}

// entityStatusLabel resolves the display label for a record status: the
// first enum field (definition order — deterministic) whose value set knows
// the value wins; fallback is the raw status string so `statusLabel` never
// binds blank.
func entityStatusLabel(status string, set *domain.EntitySet) string {
	if set == nil {
		return status
	}
	for _, f := range set.Fields {
		if f.Type != domain.FieldEnum || f.ValueSetRef == "" {
			continue
		}
		if l, ok := set.Labels[f.ValueSetRef][status]; ok && l != "" {
			return l
		}
	}
	return status
}

// parseEntityDatetime accepts the ISO-8601 shapes the operation validator
// lets through (R18 datetime_iso8601): full RFC3339 first, then the
// zone-less spellings widgets and LLMs commonly emit.
func parseEntityDatetime(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// toIntCents coerces a JSONB-round-tripped money value to integer cents.
// Storage is cents (R18); JSON unmarshal yields float64, direct Go writes
// yield int shapes.
func toIntCents(raw any) (int, bool) {
	switch v := raw.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	}
	return 0, false
}

// formatUSDCents renders integer cents as a USD display string with
// thousand separators and no minor units ("$1,250") — same convention as
// the catalog adapter's formatPrice (USD-only product decision).
func formatUSDCents(cents int) string {
	major := cents / 100
	negative := major < 0
	if negative {
		major = -major
	}
	str := fmt.Sprintf("%d", major)
	var b strings.Builder
	for i, c := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			b.WriteString(",")
		}
		b.WriteRune(c)
	}
	if negative {
		return "-$" + b.String()
	}
	return "$" + b.String()
}
