// Package prompts assembles the Agent2 system prompt:
//   - the static base body teaching the LLM how to call visual_assembly
//   - the per-tenant <fields> block listing the catalog vocabulary
//   - (later) the <tenant_design_context> block listing tenant presets
//
// AssembleSystemPrompt is the only public entry. Every other helper is
// internal.
package prompts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"unicode"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// FormatFieldsBlock returns a `<fields entity="...">` XML block listing
// every published field definition for (tenant, entityType), enriched
// with up to `sampleLimit` real values per field.
//
// Block format (mirrors V4's prompt_compose_widgets.go formatFieldsBlock):
//
//	<fields entity="product">
//	images         image/url       label="Images"           samples=[...]
//	name           text/string     label="Name"             samples=[...]
//	price          number/currency label="Price" unit="USD" samples=[...]
//	...
//	</fields>
//
// Sorting is by FieldDefinition.Priority ASC (so the most important
// fields appear first), tie-broken by FieldName ASC for byte-stable
// output. Stable bytes matter — the block goes into the cached system
// prompt, and any reordering busts the cache.
//
// SampleLimit ≤ 0 disables samples (block becomes purely declarative).
// Sample fetch errors are non-fatal: the block lands without samples
// rather than blocking prompt assembly.
func FormatFieldsBlock(
	ctx context.Context,
	fdPort ports.FieldDefinitionPort,
	tenantSlugOrID string,
	entityType domain.EntityType,
	sampleLimit int,
) (string, error) {
	// Published per-tenant vocabulary is best-effort ENRICHMENT, not a gate.
	// A read failure (e.g. catalog.field_definitions is absent) must not kill
	// the Agent2 turn — degrade to a data-derived block, mirroring Agent1's
	// fail-open digest.
	defs, err := fdPort.ListFieldDefinitions(ctx, tenantSlugOrID, entityType)
	if err != nil {
		slog.WarnContext(ctx, "list field definitions failed; building data-derived <fields>", "tenant", tenantSlugOrID, "err", err)
		defs = nil
	}

	// Real sample values per field, read straight from the catalog data
	// (independent of field_definitions). This is BOTH the sample column AND,
	// for fields with no published definition, the source of truth for which
	// fields exist — so a brand-new tenant/vertical renders with zero config.
	samples := map[string][]interface{}{}
	if sampleLimit > 0 {
		if got, err := fdPort.SampleFieldValues(ctx, tenantSlugOrID, entityType, sampleLimit); err == nil {
			samples = got
		}
	}

	// Authoritative published definitions first, ordered by priority.
	defined := make(map[string]struct{}, len(defs))
	for _, d := range defs {
		defined[d.FieldName] = struct{}{}
	}
	sort.Slice(defs, func(i, j int) bool {
		if defs[i].Priority != defs[j].Priority {
			return defs[i].Priority < defs[j].Priority
		}
		return defs[i].FieldName < defs[j].FieldName
	})

	// Then any field present in the data with no published definition,
	// synthesised from its sample value and sorted by name for byte-stable
	// (cache-safe) prompt output.
	dataOnly := make([]ports.FieldDefinition, 0, len(samples))
	for name, vals := range samples {
		if _, ok := defined[name]; ok {
			continue
		}
		dataOnly = append(dataOnly, inferFieldDef(name, entityType, vals))
	}
	sort.Slice(dataOnly, func(i, j int) bool { return dataOnly[i].FieldName < dataOnly[j].FieldName })

	var b strings.Builder
	fmt.Fprintf(&b, "<fields entity=\"%s\">\n", entityType)
	for _, d := range defs {
		writeFieldRow(&b, d, samples[d.FieldName])
	}
	for _, d := range dataOnly {
		writeFieldRow(&b, d, samples[d.FieldName])
	}
	b.WriteString("</fields>\n")
	return b.String(), nil
}

// writeFieldRow emits one line of the fields block. Columns (in order):
//
//	fieldName  type/subtype   label="..."   [unit="..."]   [slot=...]   samples=[...]
//
// Optional columns (unit, slot, samples) are elided when empty so rows
// stay compact for tenants with thin metadata.
func writeFieldRow(b *strings.Builder, d ports.FieldDefinition, fieldSamples []interface{}) {
	fmt.Fprintf(b, "%-18s ", d.FieldName)
	fmt.Fprintf(b, "%s/%s ", d.AtomType, d.AtomSubtype)
	fmt.Fprintf(b, "label=%q", d.Label)
	if d.Unit != "" {
		fmt.Fprintf(b, " unit=%q", d.Unit)
	}
	if d.DefaultSlot != "" {
		fmt.Fprintf(b, " slot=%s", d.DefaultSlot)
	}
	if len(fieldSamples) > 0 {
		raw, err := json.Marshal(fieldSamples)
		if err == nil {
			fmt.Fprintf(b, " samples=%s", raw)
		}
	}
	b.WriteByte('\n')
}

// inferFieldDef synthesises a FieldDefinition for a catalog field that has no
// published definition row, guessing atom type/subtype from the field name and
// a sample value. The LLM still receives the field name + real samples, which
// the <fields> "slot → field" playbook uses to place it — this is what lets a
// new tenant or vertical render with zero per-field configuration.
func inferFieldDef(name string, entityType domain.EntityType, samples []interface{}) ports.FieldDefinition {
	d := ports.FieldDefinition{
		FieldName:   name,
		EntityType:  entityType,
		AtomType:    domain.AtomTypeText,
		AtomSubtype: domain.SubtypeString,
		Label:       humanizeLabel(name),
		Priority:    100, // emitted after published definitions (which use low priorities)
	}
	var sample interface{}
	if len(samples) > 0 {
		sample = samples[0]
	}
	switch {
	case looksLikeImageField(name, sample):
		d.AtomType, d.AtomSubtype = domain.AtomTypeImage, domain.SubtypeURL
	case isNumericValue(sample):
		d.AtomType = domain.AtomTypeNumber
		switch {
		case nameContainsAny(name, "price", "cost", "amount", "msrp"):
			d.AtomSubtype = domain.SubtypeCurrency
		case nameContainsAny(name, "rating", "stars", "score", "review"):
			d.AtomSubtype = domain.SubtypeRating
		default:
			d.AtomSubtype = domain.SubtypeFloat
		}
	}
	return d
}

// humanizeLabel turns a field name (camelCase, snake_case or kebab-case) into a
// space-separated Title Case label, e.g. "stockQuantity" → "Stock Quantity".
func humanizeLabel(name string) string {
	var words []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	var prev rune
	for i, r := range name {
		switch {
		case r == '_' || r == '-' || r == ' ':
			flush()
		case i > 0 && unicode.IsUpper(r) && !unicode.IsUpper(prev):
			flush()
			cur.WriteRune(r)
		default:
			cur.WriteRune(r)
		}
		prev = r
	}
	flush()
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// looksLikeImageField detects image fields by name hint or an image-URL sample.
func looksLikeImageField(name string, sample interface{}) bool {
	if nameContainsAny(name, "image", "img", "photo", "picture", "thumbnail") {
		return true
	}
	if s, ok := sample.(string); ok {
		s = strings.ToLower(s)
		if strings.HasPrefix(s, "http") &&
			(strings.Contains(s, ".jpg") || strings.Contains(s, ".jpeg") ||
				strings.Contains(s, ".png") || strings.Contains(s, ".webp") ||
				strings.Contains(s, ".gif")) {
			return true
		}
	}
	return false
}

func isNumericValue(v interface{}) bool {
	switch v.(type) {
	case int, int32, int64, float32, float64, json.Number:
		return true
	}
	return false
}

func nameContainsAny(name string, subs ...string) bool {
	lower := strings.ToLower(name)
	for _, s := range subs {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// AssembleSystemPrompt joins the base body, the <fields> block, and any
// future blocks into a single string with stable separators.
//
// Order matters for the prompt cache: base prompt and tools are cached
// (V4 pattern) and the fields block typically also stable per-tenant
// during a session, so it sits inside the cacheable prefix. Anything
// dynamic per-turn (tree_map, conversation history) is appended by the
// adapter — NOT here.
func AssembleSystemPrompt(base, fieldsBlock string) string {
	if fieldsBlock == "" {
		return base
	}
	return base + "\n\n" + fieldsBlock
}
