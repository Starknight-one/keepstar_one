package tools

// The surface-links handover through compose_turn — the last beat of the
// onboarding flow (V2_SPEC L11: the critical path hardens first).
//
// Two contracts live here:
//  1. Rendering a preset whose data source is a ZERO-INPUT manifest step's
//     synthetic EntitySet STAGES AND APPLIES that step server-side (owner's
//     law, handoff 2026-07-28 #1 — the same law already encoded for form
//     submit and file upload).
//  2. The rendered block carries the real URLs regardless of how the model
//     spelled its `replicate` argument.

import (
	"context"
	"errors"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine/presets"
)

// seedPresetFor serves the REAL embedded seed as a published preset — the
// document the tenant actually renders, not a test-local stand-in.
func seedPresetFor(t *testing.T, name string) *domain.Preset {
	t.Helper()
	raw, ok := presets.SystemPresetSeeds[name]
	if !ok {
		t.Fatalf("seed %q missing from SystemPresetSeeds", name)
	}
	return &domain.Preset{
		ID: "pr-" + name, TenantID: "t-1", Name: name,
		Status: domain.PresetStatusPublished, DocumentJSON: raw,
	}
}

const (
	testStorefrontURL = "https://v5.example.test/s/acme-realty"
	testCRMURL        = "https://v5.example.test/crm/acme-realty?k=surface-token-1"
)

// surfaceLinkSet mirrors what meta_apply_manifest.go writes into the data
// zone once issue_surface_urls applies: {label, url, surface} per surface.
func surfaceLinkSet() domain.EntitySet {
	return domain.EntitySet{
		Slug:      "surfaceLink",
		Name:      "Surface links",
		Synthetic: true,
		Fields: []domain.FieldDef{
			{Key: "label", Label: "Surface", Type: domain.FieldText},
			{Key: "url", Label: "URL", Type: domain.FieldText},
			{Key: "surface", Label: "Kind", Type: domain.FieldText},
		},
		Records: []domain.EntityRecord{
			{ID: "storefront", EntitySlug: "surfaceLink", Data: map[string]any{
				"label": "Storefront", "url": testStorefrontURL, "surface": "storefront"}},
			{ID: "crm", EntitySlug: "surfaceLink", Data: map[string]any{
				"label": "CRM", "url": testCRMURL, "surface": "crm"}},
		},
	}
}

// fakeManifestGate stands in for *usecases.ManifestApplier: it records the
// ops it was asked to ensure and — like the real applier — lands the step's
// synthetic EntitySet in the session's data zone.
type fakeManifestGate struct {
	state   *minStatePort
	calls   []string
	applied bool
	err     error
}

func (g *fakeManifestGate) EnsureZeroInputStep(_ context.Context, _, op string) (bool, error) {
	g.calls = append(g.calls, op)
	if g.err != nil {
		return false, g.err
	}
	if g.applied {
		return false, nil // terminal — already live
	}
	g.applied = true
	g.state.state.Current.Data.Entities = append(g.state.state.Current.Data.Entities, surfaceLinkSet())
	return true, nil
}

// boundURLs collects every fieldBinding→content pair a rendered document
// carries, keyed by binding name with the values in document order.
func boundValues(doc map[string]interface{}, binding string) []string {
	var out []string
	var walk func(any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]interface{}:
			if fb, _ := v["fieldBinding"].(string); fb == binding {
				if c, ok := v["content"].(string); ok {
					out = append(out, c)
				}
			}
			for _, child := range v {
				walk(child)
			}
		case []interface{}:
			for _, item := range v {
				walk(item)
			}
		}
	}
	walk(doc)
	return out
}

// TestComposeTurnAutoAppliesZeroInputStepOnRender — the handoff tail #1
// contract. The model composed the handover ("everything is assembled" →
// surface_links) but never staged issue_surface_urls and never called
// apply_manifest. The block must STILL go on the wire with live URLs: the
// server stages + applies the step before assembling it, then re-reads the
// data zone the apply just wrote.
func TestComposeTurnAutoAppliesZeroInputStepOnRender(t *testing.T) {
	state := newMinStatePort(nil)
	gate := &fakeManifestGate{state: state}
	tool := composeTool(state, map[string]*domain.Preset{
		"surface_links": seedPresetFor(t, "surface_links"),
	})
	tool.SetManifestGate(gate)
	ctx, sink := collectorCtx()

	res, err := tool.Execute(ctx,
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "acme-realty"},
		map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"kind": "text", "text": "Everything is assembled."},
			map[string]interface{}{"kind": "render", "preset": "surface_links", "replicate": "surfaceLink"},
		}},
	)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.IsError {
		t.Fatalf("ToolResult IsError: %s", res.Content)
	}

	if len(gate.calls) != 1 || gate.calls[0] != "issue_surface_urls" {
		t.Fatalf("gate calls = %v, want exactly [issue_surface_urls]", gate.calls)
	}
	blocks := sink.Blocks()
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	urls := boundValues(blocks[1].Document, "url")
	if len(urls) != 2 || urls[0] != testStorefrontURL || urls[1] != testCRMURL {
		t.Fatalf("rendered urls = %v, want both issued addresses — the block reached the "+
			"user without the data the apply produced", urls)
	}
}

// A preset outside the table, and a session whose step already applied, must
// not drag the applier into every render (loop guard); a gate failure must
// not lose the turn — the block still renders on the data that exists.
func TestComposeTurnZeroInputGateBoundaries(t *testing.T) {
	t.Run("untabled preset never calls the gate", func(t *testing.T) {
		state := newMinStatePort([]domain.Product{{ID: "p1", Name: "Glow Serum"}})
		gate := &fakeManifestGate{state: state}
		tool := composeTool(state, map[string]*domain.Preset{"product_card": minimalPreset("product_card")})
		tool.SetManifestGate(gate)
		ctx, _ := collectorCtx()
		if _, err := tool.Execute(ctx, domain.ToolContext{SessionID: "sess-1", TenantSlug: "acme"},
			map[string]interface{}{"blocks": []interface{}{
				map[string]interface{}{"kind": "render", "preset": "product_card", "replicate": "products"},
			}}); err != nil {
			t.Fatalf("Execute error: %v", err)
		}
		if len(gate.calls) != 0 {
			t.Errorf("gate called for product_card: %v", gate.calls)
		}
	})

	t.Run("one ensure per call even when the preset repeats", func(t *testing.T) {
		state := newMinStatePort(nil)
		gate := &fakeManifestGate{state: state}
		tool := composeTool(state, map[string]*domain.Preset{
			"surface_links": seedPresetFor(t, "surface_links"),
		})
		tool.SetManifestGate(gate)
		ctx, _ := collectorCtx()
		if _, err := tool.Execute(ctx, domain.ToolContext{SessionID: "sess-1", TenantSlug: "acme"},
			map[string]interface{}{"blocks": []interface{}{
				map[string]interface{}{"kind": "render", "preset": "surface_links", "replicate": "surfaceLink"},
				map[string]interface{}{"kind": "render", "preset": "surface_links", "replicate": "surfaceLink"},
			}}); err != nil {
			t.Fatalf("Execute error: %v", err)
		}
		if len(gate.calls) != 1 {
			t.Errorf("gate calls = %v, want 1 per compose_turn call", gate.calls)
		}
	})

	t.Run("gate failure still ships the turn", func(t *testing.T) {
		state := newMinStatePort(nil)
		state.state.Current.Data.Entities = []domain.EntitySet{surfaceLinkSet()}
		gate := &fakeManifestGate{state: state, err: errors.New("admin down")}
		tool := composeTool(state, map[string]*domain.Preset{
			"surface_links": seedPresetFor(t, "surface_links"),
		})
		tool.SetManifestGate(gate)
		ctx, sink := collectorCtx()
		res, err := tool.Execute(ctx, domain.ToolContext{SessionID: "sess-1", TenantSlug: "acme"},
			map[string]interface{}{"blocks": []interface{}{
				map[string]interface{}{"kind": "render", "preset": "surface_links", "replicate": "surfaceLink"},
			}})
		if err != nil {
			t.Fatalf("Execute error: %v", err)
		}
		if res.IsError {
			t.Fatalf("a failing gate must not fail the turn: %s", res.Content)
		}
		if got := boundValues(sink.Blocks()[0].Document, "url"); len(got) != 2 {
			t.Errorf("urls = %v, want the two already in the data zone", got)
		}
	})
}
