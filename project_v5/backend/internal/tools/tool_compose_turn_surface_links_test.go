package tools

// The surface-links handover through compose_turn — the last beat of the
// onboarding flow (V2_SPEC L11: the critical path hardens first).
//
// Two contracts live here, and both are COMPOSE_TURN's half of the tail:
//  1. Rendering a preset whose data source is a ZERO-INPUT manifest step's
//     synthetic EntitySet calls the gate before assembly and RE-READS the
//     session state afterwards, so the block binds what the apply wrote
//     (owner's law, handoff 2026-07-28 #1 — the same law already encoded
//     for form submit and file upload).
//  2. The rendered block carries the real URLs regardless of how the model
//     spelled its `replicate` argument.
//
// What is NOT proven here: that the applier behind the gate can reach the
// data zone at all. tools cannot import usecases (usecases imports tools),
// so the gate below is a stand-in. That seam — real *usecases.ManifestApplier
// + real operations.NewSyntheticSetPublisher + this real tool — is driven end
// to end in internal/usecases/manifest_apply_handover_test.go. Keep the two
// files in step: a stand-in that writes what production cannot write is how
// the empty-handover bug survived a green suite once already.

import (
	"context"
	"encoding/json"
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

// fakeManifestGate stands in for the whole production seam behind the gate —
// *usecases.ManifestApplier PLUS the injected synthetic-set publisher: it
// records the ops it was asked to ensure and lands the step's EntitySet in
// the session's data zone, which is what that pair does together (the applier
// alone holds only an OnboardingStatePort and cannot).
//
// So the write below is the STIMULUS for the assertions here, never the
// evidence: what these tests check is that compose_turn calls the gate once
// for a tabled preset and re-reads state after it. Whether the real applier
// performs that write is a different contract, asserted against the real
// parts in internal/usecases/manifest_apply_handover_test.go.
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

// TestSurfaceLinksRendersRealURLs — handoff tail #3. The surfaceLink set is
// in the data zone, yet the card reached the user with no addresses on it.
// The cause is the `replicate` argument: only the exact source name fans the
// row template out, and the model is free to omit it, pass a count, or
// mistype it — in every one of those cases the replicate frame collapses (or
// binds catalog products) and the handover shows a headline over nothing.
//
// A preset authored against a synthetic set declares its own source, so the
// URLs land however the model spelled the argument. Same law as everything
// else on this path: the artifact must not depend on the model.
func TestSurfaceLinksRendersRealURLs(t *testing.T) {
	cases := []struct {
		name      string
		replicate any
		products  []domain.Product
	}{
		{"source named exactly", "surfaceLink", nil},
		{"replicate omitted", nil, nil},
		{"replicate omitted with a seeded catalog", nil, []domain.Product{{ID: "p1", Name: "Flat 12B"}}},
		{"replicate given as a count", "2", []domain.Product{{ID: "p1", Name: "Flat 12B"}, {ID: "p2", Name: "Flat 14A"}}},
		{"source mistyped", "surfaceLinks", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := newMinStatePort(tc.products)
			state.state.Current.Data.Entities = []domain.EntitySet{surfaceLinkSet()}
			tool := composeTool(state, map[string]*domain.Preset{
				"surface_links": seedPresetFor(t, "surface_links"),
			})
			ctx, sink := collectorCtx()

			block := map[string]interface{}{"kind": "render", "preset": "surface_links"}
			if tc.replicate != nil {
				block["replicate"] = tc.replicate
			}
			res, err := tool.Execute(ctx,
				domain.ToolContext{SessionID: "sess-1", TenantSlug: "acme-realty"},
				map[string]interface{}{"blocks": []interface{}{block}})
			if err != nil {
				t.Fatalf("Execute error: %v", err)
			}
			if res.IsError {
				t.Fatalf("ToolResult IsError: %s", res.Content)
			}

			doc := sink.Blocks()[0].Document
			if got := boundValues(doc, "label"); len(got) != 2 || got[0] != "Storefront" || got[1] != "CRM" {
				t.Errorf("labels = %v, want [Storefront CRM]", got)
			}
			urls := boundValues(doc, "url")
			if len(urls) != 2 || urls[0] != testStorefrontURL || urls[1] != testCRMURL {
				t.Fatalf("urls = %v, want [%s %s]", urls, testStorefrontURL, testCRMURL)
			}
			// Visible is not enough: the handover is an address the user has
			// to open, so each URL node is a link the widget can dispatch.
			for i, node := range nodesWithBinding(doc, "url") {
				if w, _ := node["wrapper"].(string); w != string(domain.WrapperLink) {
					t.Errorf("url node %d wrapper = %q, want link", i, w)
				}
				act, _ := node["action"].(map[string]interface{})
				if act == nil {
					t.Fatalf("url node %d has no action — the address is not tappable", i)
				}
				if kind, _ := act["kind"].(string); kind != string(domain.UserActionExternalLink) {
					t.Errorf("url node %d action kind = %q, want external_link", i, kind)
				}
				params, _ := act["params"].(map[string]interface{})
				if got, _ := params["url"].(string); got != urls[i] {
					t.Errorf("url node %d action url = %q, want %q (the bound address)", i, got, urls[i])
				}
			}
		})
	}
}

// A preset with a declared source but an EMPTY data zone must not invent
// rows — the card collapses to its headline, exactly as before.
func TestSurfaceLinksWithoutTheSetStaysEmpty(t *testing.T) {
	state := newMinStatePort([]domain.Product{{ID: "p1", Name: "Flat 12B"}})
	tool := composeTool(state, map[string]*domain.Preset{
		"surface_links": seedPresetFor(t, "surface_links"),
	})
	ctx, sink := collectorCtx()
	if _, err := tool.Execute(ctx, domain.ToolContext{SessionID: "sess-1", TenantSlug: "acme"},
		map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"kind": "render", "preset": "surface_links"},
		}}); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got := boundValues(sink.Blocks()[0].Document, "url"); len(got) != 0 {
		t.Errorf("urls = %v, want none — nothing was issued yet", got)
	}
}

// nodesWithBinding returns every node carrying the given fieldBinding, in
// document order.
func nodesWithBinding(doc map[string]interface{}, binding string) []map[string]interface{} {
	var out []map[string]interface{}
	var walk func(any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]interface{}:
			if fb, _ := v["fieldBinding"].(string); fb == binding {
				out = append(out, v)
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

// ─── scope of the link pass ──────────────────────────────────────────────

// tenantLinkPresetJSON binds a link-wrapped node to an ordinary record
// field — the shape any list preset over tenant data can take (a lead's
// "website", a listing's "source").
const tenantLinkPresetJSON = `{
  "version": "2.10",
  "children": [{
    "type": "frame", "id": "rows",
    "layout": {"direction": "column", "gap": "sm"},
    "children": [{
      "type": "frame", "id": "row", "replicate": true,
      "layout": {"direction": "row", "gap": "sm"},
      "children": [
        {"type": "text", "id": "site", "fieldBinding": "website", "wrapper": "link"}
      ]
    }]
  }]
}`

// leadSet is a TENANT set: its records come from the entity plane, and a
// public storefront form is one of the things that writes them.
func leadSet(synthetic bool, website string) domain.EntitySet {
	return domain.EntitySet{
		Slug:      "lead",
		Name:      "Leads",
		Synthetic: synthetic,
		Fields:    []domain.FieldDef{{Key: "website", Label: "Website", Type: domain.FieldText}},
		Records: []domain.EntityRecord{
			{ID: "l1", EntitySlug: "lead", Data: map[string]any{"website": website}},
		},
	}
}

// The link pass exists for the addresses the APPLIER mints (storefront,
// CRM). Tenant records are visitor-writable: a public booking form can put
// any address into a lead field, and stamping a dispatching external_link on
// it would give the CRM operator a one-click window.open to whatever a
// stranger typed. So the pass is gated on the bound source being synthetic —
// bound tenant data keeps rendering href="#", the behaviour before the pass
// existed.
//
// The two cases differ ONLY in EntitySet.Synthetic, so the test cannot pass
// for any other reason (same preset, same records, same replicate argument).
func TestLinkActionsOnlyForSyntheticSources(t *testing.T) {
	const attacker = "https://evil.example/steal?s=1"

	run := func(t *testing.T, synthetic bool) map[string]interface{} {
		t.Helper()
		state := newMinStatePort(nil)
		state.state.Current.Data.Entities = []domain.EntitySet{leadSet(synthetic, attacker)}
		tool := composeTool(state, map[string]*domain.Preset{
			"lead_links": {
				ID: "pr-lead-links", TenantID: "t-1", Name: "lead_links",
				Status: domain.PresetStatusPublished, DocumentJSON: json.RawMessage(tenantLinkPresetJSON),
			},
		})
		ctx, sink := collectorCtx()
		if _, err := tool.Execute(ctx, domain.ToolContext{SessionID: "sess-1", TenantSlug: "acme"},
			map[string]interface{}{"blocks": []interface{}{
				map[string]interface{}{"kind": "render", "preset": "lead_links", "replicate": "lead"},
			}}); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		return sink.Blocks()[0].Document
	}

	t.Run("tenant record is rendered but not dispatchable", func(t *testing.T) {
		doc := run(t, false)
		if got := boundValues(doc, "website"); len(got) != 1 || got[0] != attacker {
			t.Fatalf("website = %v — the value must still render, only the action is withheld", got)
		}
		nodes := nodesWithBinding(doc, "website")
		if len(nodes) != 1 {
			t.Fatalf("nodes = %d, want 1", len(nodes))
		}
		if act, exists := nodes[0]["action"]; exists {
			t.Errorf("visitor-supplied URL carries a dispatching action %v — one click "+
				"opens whatever a stranger wrote into the lead", act)
		}
	})

	t.Run("runtime-minted record is dispatchable", func(t *testing.T) {
		doc := run(t, true)
		nodes := nodesWithBinding(doc, "website")
		if len(nodes) != 1 {
			t.Fatalf("nodes = %d, want 1", len(nodes))
		}
		act, _ := nodes[0]["action"].(map[string]interface{})
		if act == nil {
			t.Fatal("synthetic address is not tappable — the gate is too tight and the " +
				"handover card regresses")
		}
		if kind, _ := act["kind"].(string); kind != string(domain.UserActionExternalLink) {
			t.Errorf("action kind = %q, want external_link", kind)
		}
	})
}
