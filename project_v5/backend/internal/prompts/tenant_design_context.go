package prompts

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// FormatTenantDesignContext returns a `<tenant_design_context>` XML block
// listing every published preset for the tenant so Agent2 can pick one by
// name. Mirrors FormatFieldsBlock's style: one compact row per preset,
// byte-stable ordering (presetPort already sorts by name), best-effort —
// a read failure degrades to an empty block rather than killing the turn.
//
// Block format:
//
//	<tenant_design_context>
//	product_card           entity=product replicate=yes  — A responsive product grid card
//	product_detail         entity=product replicate=no   — Single-product detail view
//	...
//	</tenant_design_context>
//
// The list unions tenant-authored presets with the in-process system
// registry (the adapter does the union in ListPublishedPresets), so every
// tenant sees at least the starter set. When the tenant has no presets and
// the adapter returns nothing, the block is omitted entirely (empty string)
// so the cached prompt stays unchanged from the no-presets baseline.
func FormatTenantDesignContext(
	ctx context.Context,
	presetPort ports.PresetPort,
	tenantSlugOrID string,
) (string, error) {
	// Published presets are ENRICHMENT, not a gate. A read failure (e.g.
	// v5_presets absent on a fresh DB) must not kill the Agent2 turn —
	// degrade to no block, mirroring FormatFieldsBlock's fail-open path.
	list, err := presetPort.ListPublishedPresets(ctx, tenantSlugOrID)
	if err != nil {
		slog.WarnContext(ctx, "list published presets failed; omitting <tenant_design_context>", "tenant", tenantSlugOrID, "err", err)
		return "", nil
	}
	if len(list) == 0 {
		return "", nil
	}

	var b strings.Builder
	b.WriteString("<tenant_design_context>\n")
	for _, p := range list {
		writePresetRow(&b, p)
	}
	b.WriteString("</tenant_design_context>\n")
	return b.String(), nil
}

// writePresetRow emits one line of the design-context block. Columns:
//
//	name   entity=<entityType> replicate=<yes|no>  — <description>
//
// The description column is elided when empty so rows stay compact.
func writePresetRow(b *strings.Builder, p domain.Preset) {
	replicate := "no"
	if p.DefaultReplicate {
		replicate = "yes"
	}
	fmt.Fprintf(b, "%-22s entity=%s replicate=%s", p.Name, p.EntityType, replicate)
	if p.Description != "" {
		fmt.Fprintf(b, "  — %s", p.Description)
	}
	b.WriteByte('\n')
}
