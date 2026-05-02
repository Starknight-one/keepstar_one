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
	"sort"
	"strings"

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
	defs, err := fdPort.ListFieldDefinitions(ctx, tenantSlugOrID, entityType)
	if err != nil {
		return "", fmt.Errorf("list field definitions: %w", err)
	}
	if len(defs) == 0 {
		// No catalog vocabulary published yet — emit an empty block so the
		// prompt remains structurally consistent across tenants.
		return fmt.Sprintf("<fields entity=\"%s\">\n</fields>\n", entityType), nil
	}

	samples := map[string][]interface{}{}
	if sampleLimit > 0 {
		if got, err := fdPort.SampleFieldValues(ctx, tenantSlugOrID, entityType, sampleLimit); err == nil {
			samples = got
		}
	}

	sort.Slice(defs, func(i, j int) bool {
		if defs[i].Priority != defs[j].Priority {
			return defs[i].Priority < defs[j].Priority
		}
		return defs[i].FieldName < defs[j].FieldName
	})

	var b strings.Builder
	fmt.Fprintf(&b, "<fields entity=\"%s\">\n", entityType)
	for _, d := range defs {
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
