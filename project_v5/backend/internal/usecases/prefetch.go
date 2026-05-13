package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine"
	"keepstar_v5/internal/engine/presets"
	"keepstar_v5/internal/ports"
)

// PrefetchPayload is the 1-level-deep navigation prefetch shipped to
// the frontend with each pipeline response. Mirrors V4's adjacent
// templates pattern (one template per entity type + raw entity list)
// — frontend binds the template with the chosen item on click for an
// instant drill-down with zero round-trip latency.
//
// Schema (also wired in handler_pipeline response):
//
//	{
//	  "adjacentTemplate": {"product": <Document map>},
//	  "entities":         {"product": [<entity map>, ...]}
//	}
//
// adjacentTemplate is keyed by entity type so future expansion (e.g.
// service catalogs) plugs in without breaking the wire shape. Empty
// payload (both maps nil/empty) means "no prefetch available" —
// frontend falls back to a /navigation/expand round-trip.
type PrefetchPayload struct {
	AdjacentTemplate map[string]map[string]interface{}   `json:"adjacentTemplate,omitempty"`
	Entities         map[string][]map[string]interface{} `json:"entities,omitempty"`
}

// PrefetchBuilder materialises the drill-target preset for an entity
// type given the active source preset. Stateless — pulls preset +
// component definitions on demand for each call. Used by the pipeline
// orchestrator after Agent2 returns.
type PrefetchBuilder struct {
	presets    ports.PresetPort
	components ports.ComponentPort
	log        *slog.Logger
}

// NewPrefetchBuilder constructs the builder. Both ports are required.
func NewPrefetchBuilder(presetPort ports.PresetPort, componentPort ports.ComponentPort, log *slog.Logger) *PrefetchBuilder {
	return &PrefetchBuilder{
		presets:    presetPort,
		components: componentPort,
		log:        log,
	}
}

// Build returns a PrefetchPayload for the given source preset against
// the products in state.Current.Data. Returns nil (and logs at debug
// level) when:
//   - sourcePreset is empty (freestyle / modify call shape — no
//     adjacency lookup possible),
//   - sourcePreset has no registered drill target,
//   - the drill-target preset cannot be loaded,
//   - the data zone is empty (no entities to prefetch).
//
// Errors are non-fatal — prefetch is a UX optimisation, never the
// primary render path. Frontend works without it (slower drill).
func (b *PrefetchBuilder) Build(ctx context.Context, tenantSlug string, sourcePreset string, products []domain.Product) *PrefetchPayload {
	if sourcePreset == "" || len(products) == 0 {
		return nil
	}
	target := presets.AdjacentPreset(sourcePreset)
	if target == "" {
		return nil
	}

	preset, err := b.presets.GetPublishedPreset(ctx, tenantSlug, target)
	if err != nil {
		b.log.Debug("prefetch: drill target preset not found", "source", sourcePreset, "target", target, "err", err)
		return nil
	}

	presetDoc, err := unmarshalEngineDoc(preset.DocumentJSON)
	if err != nil {
		b.log.Debug("prefetch: preset doc unmarshal failed", "target", target, "err", err)
		return nil
	}

	componentList, err := b.components.ListPublishedComponents(ctx, tenantSlug)
	if err != nil {
		b.log.Debug("prefetch: list components failed", "err", err)
		return nil
	}
	componentDocs := make([]*engine.Document, 0, len(componentList))
	for _, c := range componentList {
		cd, err := unmarshalEngineDoc(c.DocumentJSON)
		if err != nil {
			continue
		}
		componentDocs = append(componentDocs, cd)
	}

	merged := engine.Materialise(presetDoc, componentDocs)
	// No replicate fan-out for prefetch templates — they bind one
	// entity at a time on the frontend. Resolve refs (so detail
	// components inline now); leave fieldBindings intact so
	// fillTemplate can fill them on click.
	engine.ResolveAndInline(merged)

	templateMap, err := engineDocToMap(merged)
	if err != nil {
		b.log.Debug("prefetch: marshal template failed", "err", err)
		return nil
	}

	entities := make([]map[string]interface{}, 0, len(products))
	for _, p := range products {
		entities = append(entities, engine.ProductToMap(p))
	}

	return &PrefetchPayload{
		AdjacentTemplate: map[string]map[string]interface{}{
			"product": templateMap,
		},
		Entities: map[string][]map[string]interface{}{
			"product": entities,
		},
	}
}

// unmarshalEngineDoc parses a domain.Preset.DocumentJSON / Component
// raw bytes into engine.Document. Local copy of the same helper in
// tool_visual_assembly to avoid cross-package coupling.
func unmarshalEngineDoc(raw json.RawMessage) (*engine.Document, error) {
	var doc engine.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &doc, nil
}

// engineDocToMap round-trips a Document through JSON into the
// map[string]interface{} shape the wire response uses.
func engineDocToMap(doc *engine.Document) (map[string]interface{}, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
