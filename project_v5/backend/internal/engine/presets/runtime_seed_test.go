package presets

// Runtime-v1 seed contracts (RUNTIME_SPEC.md §5.3, R20/R21, §6.2).
// The 8 new presets must (a) round-trip through engine.Document, (b) bind
// their cross-lane vocabularies through the REAL zero-LLM chain
// (Materialise → ExpandReplicates → ResolveAndInline → BindData), and
// (c) obey the binding-vocabulary governance: every fieldBinding key in
// every seed is camelCase under engine.SnakeToCamel — THE only normalizer.
// A brand guard walks every seed for purple-family colors (project rule:
// blue #5BA4D9 / orange #F0924A, no purple anywhere).

import (
	"encoding/json"
	"regexp"
	"strconv"
	"testing"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine"
)

// runtimeSeeds — the 8 runtime-v1 presets with their structural
// expectations: root node id and whether the seed carries a replicate
// template frame.
var runtimeSeeds = []struct {
	name      string
	rootID    string
	replicate bool
}{
	{"design_system_preview", "design-system", false},
	{"uploader_card", "uploader", false},
	{"operation_card", "op-list", true},
	{"registration_form", "registration-form", false},
	{"booking_form", "booking-form", false},
	{"lead_table", "lead-list", true},
	{"lead_detail", "lead-detail", false},
	{"success_plaque", "plaque", false},
	{"surface_links", "surface-links", true},
	{"manifest_summary", "manifest-summary", true},
}

func parseSeed(t *testing.T, name string) *engine.Document {
	t.Helper()
	raw, ok := SystemPresetSeeds[name]
	if !ok {
		t.Fatalf("seed %q not in SystemPresetSeeds", name)
	}
	var doc engine.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal seed %q: %v", name, err)
	}
	return &doc
}

// runPipeline runs the production zero-LLM chain over a parsed seed.
func runPipeline(t *testing.T, doc *engine.Document, data []map[string]any) *engine.Document {
	t.Helper()
	out := engine.Materialise(doc, nil)
	engine.ExpandReplicates(out, len(data))
	if stats := engine.ResolveAndInline(out); len(stats.Failed) > 0 {
		t.Fatalf("ResolveAndInline failed refs: %v", stats.Failed)
	}
	engine.BindData(out, data)
	return out
}

// TestRuntimeSeedsRoundTrip — every runtime-v1 seed parses, has exactly
// one root child with the expected id, and carries (or not) a replicate
// template frame. DefaultReplicate must agree with the seed structure.
func TestRuntimeSeedsRoundTrip(t *testing.T) {
	for _, tc := range runtimeSeeds {
		t.Run(tc.name, func(t *testing.T) {
			doc := parseSeed(t, tc.name)
			if len(doc.Children) != 1 {
				t.Fatalf("expected 1 root child, got %d", len(doc.Children))
			}
			root := doc.Children[0]
			if engine.NodeID(root) != tc.rootID {
				t.Errorf("root id = %q, want %q", engine.NodeID(root), tc.rootID)
			}
			hasReplicate := false
			engine.WalkNodes(root, func(n engine.Node, _ int) {
				if rep, _ := n["replicate"].(bool); rep {
					hasReplicate = true
				}
			})
			if rep, _ := root["replicate"].(bool); rep {
				hasReplicate = true
			}
			if hasReplicate != tc.replicate {
				t.Errorf("replicate frame present = %v, want %v", hasReplicate, tc.replicate)
			}
			if SystemPresetDefaultReplicate[tc.name] != tc.replicate {
				t.Errorf("SystemPresetDefaultReplicate[%q] = %v, want %v",
					tc.name, SystemPresetDefaultReplicate[tc.name], tc.replicate)
			}
		})
	}
}

// TestSeedFieldBindingsCamelCase — §6.2 governance over EVERY system seed:
// each fieldBinding key must be camelCase (SnakeToCamel fixed point) and
// match ^[a-z][a-zA-Z0-9]*$.
func TestSeedFieldBindingsCamelCase(t *testing.T) {
	keyShape := regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`)
	for name := range SystemPresetSeeds {
		doc := parseSeed(t, name)
		for _, root := range doc.Children {
			engine.WalkNodes(root, func(n engine.Node, _ int) {
				fb, _ := n["fieldBinding"].(string)
				if fb == "" {
					return
				}
				if engine.SnakeToCamel(fb) != fb || !keyShape.MatchString(fb) {
					t.Errorf("seed %q node %q: fieldBinding %q is not camelCase", name, engine.NodeID(n), fb)
				}
			})
		}
	}
}

// hexColor matches 6- or 8-digit hex color literals inside seed JSON.
var hexColor = regexp.MustCompile(`#([0-9a-fA-F]{6})`)

// TestNoPurpleInSeeds — brand rule: no purple anywhere. Flags colors in
// the purple/violet family: red and blue channels both substantial while
// green trails both (the purple signature). Brand blue (#5BA4D9),
// orange (#F0924A), neutrals and greens all pass.
func TestNoPurpleInSeeds(t *testing.T) {
	for name, raw := range SystemPresetSeeds {
		for _, m := range hexColor.FindAllStringSubmatch(string(raw), -1) {
			r, _ := strconv.ParseInt(m[1][0:2], 16, 32)
			g, _ := strconv.ParseInt(m[1][2:4], 16, 32)
			b, _ := strconv.ParseInt(m[1][4:6], 16, 32)
			min := r
			if b < min {
				min = b
			}
			if r >= 80 && b >= 80 && float64(g) < 0.9*float64(min) {
				t.Errorf("seed %q: color #%s reads as purple (r=%d g=%d b=%d) — brand forbids purple", name, m[1], r, g, b)
			}
		}
	}
}

// opCardData — the synthetic `opCard` EntitySet vocabulary (R23):
// LibraryHit identity + card content, camelCased by EntityToMap.
func opCardData() []map[string]any {
	recs := []domain.EntityRecord{
		{
			ID:         "op-book-showing",
			EntitySlug: "opCard",
			Data: map[string]any{
				"name":           "book_showing",
				"kind":           "schedule_slot",
				"title":          "Book a showing",
				"description":    "Books a visit against a listing with deterministic time guards.",
				"input_summary":  "Contact details plus a preferred date and time, linked to a listing",
				"does":           "Creates a lead record and fires the booking automation",
				"output_summary": "A lead in the pipeline, visible to the CRM",
				"why":            "Turns storefront interest into a working lead",
			},
		},
		{
			ID:         "op-lead-search",
			EntitySlug: "opCard",
			Data: map[string]any{
				"name":           "lead_search",
				"kind":           "query",
				"title":          "Search leads",
				"description":    "Finds leads by status, time window and listing.",
				"input_summary":  "A search query with optional filters",
				"does":           "Queries the lead records",
				"output_summary": "Matching leads",
				"why":            "The CRM's daily question: any new leads?",
			},
		},
	}
	out := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		out = append(out, engine.EntityToMap(r, nil))
	}
	return out
}

// TestOperationCardRendersThroughPipeline — replicates over the synthetic
// opCard set and binds title / kind / description + the four card rows
// (inputSummary / does / outputSummary / why).
func TestOperationCardRendersThroughPipeline(t *testing.T) {
	data := opCardData()
	doc := runPipeline(t, parseSeed(t, "operation_card"), data)

	list := engine.FindNodeByID(doc, "op-list")
	if list == nil {
		t.Fatal("op-list frame missing after pipeline")
	}
	clones := engine.Children(list)
	if len(clones) != len(data) {
		t.Fatalf("expected %d op-card clones, got %d", len(data), len(clones))
	}
	for i, clone := range clones {
		got := map[string]any{}
		engine.WalkNodes(clone, func(n engine.Node, _ int) {
			if fb, _ := n["fieldBinding"].(string); fb != "" {
				got[fb] = n["content"]
			}
		})
		for _, key := range []string{"title", "kind", "description", "inputSummary", "does", "outputSummary", "why"} {
			if got[key] != data[i][key] {
				t.Errorf("clone[%d] %s = %v, want %v", i, key, got[key], data[i][key])
			}
		}
	}
}

// demoLeadData — realistic lead records flattened through EntityToMap with
// the demo lead definition snapshot (exec_demo_bundle.go vocabulary).
func demoLeadData(t *testing.T) []map[string]any {
	t.Helper()
	set := &domain.EntitySet{
		Slug: "lead",
		Name: "Leads",
		Fields: []domain.FieldDef{
			{Key: "name", Label: "Name", Type: domain.FieldText},
			{Key: "phone", Label: "Phone", Type: domain.FieldPhone},
			{Key: "email", Label: "Email", Type: domain.FieldEmail},
			{Key: "message", Label: "Message", Type: domain.FieldText},
			{Key: "preferredTime", Label: "Preferred time", Type: domain.FieldDatetime},
			{Key: "status", Label: "Status", Type: domain.FieldEnum, ValueSetRef: "lead_pipeline"},
		},
		Labels: map[string]map[string]string{
			"lead_pipeline": {"new": "New", "contacted": "Contacted"},
		},
	}
	created := time.Date(2026, 7, 20, 14, 30, 0, 0, time.UTC)
	recs := []domain.EntityRecord{
		{
			ID:          "lead-1",
			EntitySlug:  "lead",
			Status:      "new",
			RefEntityID: "listing-77",
			CreatedAt:   created,
			Data: map[string]any{
				"name":          "Ana Souza",
				"phone":         "+15550123456",
				"email":         "ana@example.com",
				"message":       "Is the balcony furnished?",
				"preferredTime": "2026-08-01T15:00",
				"status":        "new",
				"refTitle":      "2BR apartment on Maple St",
			},
		},
		{
			ID:         "lead-2",
			EntitySlug: "lead",
			Status:     "contacted",
			CreatedAt:  created.Add(2 * time.Hour),
			Data: map[string]any{
				"name":          "Bruno Lima",
				"phone":         "+15559876543",
				"preferredTime": "2026-08-02T11:00",
				"status":        "contacted",
				"refTitle":      "Loft near the marina",
			},
		},
	}
	out := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		out = append(out, engine.EntityToMap(r, set))
	}
	return out
}

// TestLeadTableRendersThroughPipeline — fans out one row per lead and
// binds the CRM vocabulary: name, statusLabel (value-set label), phone,
// refTitle, preferredTimeFormatted, createdAtFormatted.
func TestLeadTableRendersThroughPipeline(t *testing.T) {
	data := demoLeadData(t)
	doc := runPipeline(t, parseSeed(t, "lead_table"), data)

	list := engine.FindNodeByID(doc, "lead-list")
	if list == nil {
		t.Fatal("lead-list frame missing after pipeline")
	}
	clones := engine.Children(list)
	if len(clones) != len(data) {
		t.Fatalf("expected %d lead rows, got %d", len(data), len(clones))
	}
	wantBound := []string{"name", "statusLabel", "phone", "refTitle", "preferredTimeFormatted", "createdAtFormatted"}
	for i, clone := range clones {
		got := map[string]any{}
		engine.WalkNodes(clone, func(n engine.Node, _ int) {
			if fb, _ := n["fieldBinding"].(string); fb != "" {
				got[fb] = n["content"]
			}
		})
		for _, key := range wantBound {
			want, present := data[i][key]
			if !present {
				continue // optional key absent on this record (refTitle etc.)
			}
			if got[key] != want {
				t.Errorf("row[%d] %s = %v, want %v", i, key, got[key], want)
			}
		}
	}
	// Value-set label resolution proves the EntityToMap seam end to end.
	if data[0]["statusLabel"] != "New" || data[1]["statusLabel"] != "Contacted" {
		t.Errorf("statusLabel derivation broken: %v / %v", data[0]["statusLabel"], data[1]["statusLabel"])
	}
}

// TestLeadDetailBindsRecordAndAdvanceActions — single-entity bind (no
// replicate): the hidden id input receives the record id as `value` (the
// §5.2 hidden-input contract FormProvider merges into advance_lead
// params), and the submit atoms carry the operation_invoke actions.
func TestLeadDetailBindsRecordAndAdvanceActions(t *testing.T) {
	data := demoLeadData(t)[:1]
	doc := runPipeline(t, parseSeed(t, "lead_detail"), data)

	root := engine.FindNodeByID(doc, "lead-detail")
	if root == nil {
		t.Fatal("lead-detail frame missing after pipeline")
	}
	if fid, _ := root["formId"].(string); fid != "lead_detail" {
		t.Errorf("root formId = %q, want lead_detail (FormProvider mount)", fid)
	}

	hidden := engine.FindNodeByID(doc, "ld-record-id")
	if hidden == nil {
		t.Fatal("hidden id input missing")
	}
	if hidden["value"] != "lead-1" {
		t.Errorf("hidden id input value = %v, want lead-1 (BindData non-text target)", hidden["value"])
	}

	wantTransitions := map[string]string{
		"ld-adv-contacted": "contacted",
		"ld-adv-booked":    "showing_booked",
	}
	for id, wantStatus := range wantTransitions {
		btn := engine.FindNodeByID(doc, id)
		if btn == nil {
			t.Fatalf("submit %q missing", id)
		}
		action, _ := btn["action"].(map[string]any)
		if action == nil {
			t.Fatalf("submit %q has no action", id)
		}
		if action["kind"] != "operation_invoke" || action["operation"] != "advance_lead" {
			t.Errorf("submit %q action = %v, want operation_invoke advance_lead", id, action)
		}
		params, _ := action["params"].(map[string]any)
		if params == nil || params["to_status"] != wantStatus {
			t.Errorf("submit %q params = %v, want to_status=%s", id, params, wantStatus)
		}
	}
}

// TestBookingFormBindsListing — the form is drilled from a listing and
// binds the ProductToMap vocabulary: hidden listingId ← id, header ← name.
// The submit invokes book_showing; field names match the demo lead
// definition keys (schedule_slot input schema).
func TestBookingFormBindsListing(t *testing.T) {
	data := []map[string]any{
		{"id": "listing-77", "name": "2BR apartment on Maple St", "priceFormatted": "$425,000"},
	}
	doc := runPipeline(t, parseSeed(t, "booking_form"), data)

	hidden := engine.FindNodeByID(doc, "booking-listing-id")
	if hidden == nil {
		t.Fatal("hidden listingId input missing")
	}
	if hidden["value"] != "listing-77" {
		t.Errorf("listingId value = %v, want listing-77", hidden["value"])
	}
	if hidden["name"] != "listingId" {
		t.Errorf("hidden input name = %v, want listingId (schedule_slot link_field)", hidden["name"])
	}

	header := engine.FindNodeByID(doc, "booking-listing-name")
	if header == nil || header["content"] != "2BR apartment on Maple St" {
		t.Errorf("listing header bound = %v, want listing name", header["content"])
	}

	submit := engine.FindNodeByID(doc, "booking-submit")
	action, _ := submit["action"].(map[string]any)
	if action == nil || action["kind"] != "operation_invoke" || action["operation"] != "book_showing" {
		t.Errorf("booking submit action = %v, want operation_invoke book_showing", action)
	}

	// Field names are the executor's input keys (§4.2 demo instance).
	wantNames := map[string]string{
		"booking-time":  "preferredTime",
		"booking-name":  "name",
		"booking-phone": "phone",
	}
	for id, wantName := range wantNames {
		n := engine.FindNodeByID(doc, id)
		if n == nil {
			t.Fatalf("form field %q missing", id)
		}
		if n["name"] != wantName {
			t.Errorf("field %q name = %v, want %q", id, n["name"], wantName)
		}
	}
}

// TestSuccessPlaqueBindsResult — binds the resultBindData vocabulary
// (handler_operations.go cross-lane contract): `summary` is the one
// bound key; the headline stays literal.
func TestSuccessPlaqueBindsResult(t *testing.T) {
	data := []map[string]any{
		{
			"operation":  "book_showing",
			"outcome":    "ok",
			"summary":    "ok: booked lead lead-1 for Aug 1, 2026 3:00 PM",
			"recordId":   "lead-1",
			"entityKind": "lead",
		},
	}
	doc := runPipeline(t, parseSeed(t, "success_plaque"), data)

	summary := engine.FindNodeByID(doc, "plaque-summary")
	if summary == nil || summary["content"] != data[0]["summary"] {
		t.Errorf("plaque summary = %v, want bound result summary", summary["content"])
	}
	title := engine.FindNodeByID(doc, "plaque-title")
	if title == nil || title["content"] != "Request received" {
		t.Errorf("plaque title = %v, want literal headline", title["content"])
	}
}

// TestSurfaceLinksBindsSyntheticSet — binds the synthetic `surfaceLink`
// EntitySet vocabulary (meta_apply_manifest.go cross-lane contract):
// {label, url, surface}, one replicated row per issued surface.
func TestSurfaceLinksBindsSyntheticSet(t *testing.T) {
	data := []map[string]any{
		{"label": "Storefront", "url": "https://v5.example/s/acme-realty", "surface": "storefront"},
		{"label": "CRM", "url": "https://v5.example/crm/acme-realty?k=tok-1", "surface": "crm"},
	}
	doc := runPipeline(t, parseSeed(t, "surface_links"), data)

	rows := replicatedRows(t, doc, "surface-links", len(data)+1)[1:] // child 0 = the literal head row
	for i, want := range data {
		got := boundContent(rows[i])
		if got["label"] != want["label"] {
			t.Errorf("row %d label = %v, want %v", i, got["label"], want["label"])
		}
		if got["url"] != want["url"] {
			t.Errorf("row %d url = %v, want %v", i, got["url"], want["url"])
		}
	}
}

// replicatedRows asserts the container has wantChildren children post-
// expansion and returns them.
func replicatedRows(t *testing.T, doc *engine.Document, containerID string, wantChildren int) []engine.Node {
	t.Helper()
	container := engine.FindNodeByID(doc, containerID)
	if container == nil {
		t.Fatalf("container %q missing after pipeline", containerID)
	}
	children := engine.Children(container)
	if len(children) != wantChildren {
		t.Fatalf("container %q has %d children, want %d", containerID, len(children), wantChildren)
	}
	return children
}

// boundContent collects fieldBinding → content over one row subtree.
func boundContent(row engine.Node) map[string]any {
	got := map[string]any{}
	engine.WalkNodes(row, func(n engine.Node, _ int) {
		if fb, _ := n["fieldBinding"].(string); fb != "" {
			got[fb] = n["content"]
		}
	})
	return got
}

// TestManifestSummaryBindsSyntheticSet — binds the synthetic `manifestStep`
// EntitySet vocabulary (meta_apply_manifest.go cross-lane contract):
// {op, title, status, statusLabel, detail}, one replicated receipt row per
// manifest step.
func TestManifestSummaryBindsSyntheticSet(t *testing.T) {
	data := []map[string]any{
		{"op": "create_tenant", "title": "Create workspace", "status": "applied", "statusLabel": "Done", "detail": "workspace acme-realty"},
		{"op": "register_user", "title": "Register the owner account", "status": "accepted", "statusLabel": "Waiting", "detail": ""},
	}
	doc := runPipeline(t, parseSeed(t, "manifest_summary"), data)

	rows := replicatedRows(t, doc, "manifest-summary", len(data)+1)[1:] // child 0 = the literal head row
	for i, want := range data {
		got := boundContent(rows[i])
		if got["title"] != want["title"] {
			t.Errorf("row %d title = %v, want %v", i, got["title"], want["title"])
		}
		if got["statusLabel"] != want["statusLabel"] {
			t.Errorf("row %d statusLabel = %v, want %v", i, got["statusLabel"], want["statusLabel"])
		}
	}
	if got := boundContent(rows[0]); got["detail"] != "workspace acme-realty" {
		t.Errorf("row 0 detail = %v, want bound detail", got["detail"])
	}
}
