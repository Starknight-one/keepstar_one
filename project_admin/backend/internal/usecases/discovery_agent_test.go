package usecases

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"keepstar-admin/internal/adapters/anthropic"
	"keepstar-admin/internal/domain"
)

// =============================================================================
// ArtifactBuilder tests
// =============================================================================

func TestArtifactBuilder_PrePopulatesFromTier1(t *testing.T) {
	tier1 := AutoMapTier1Result{
		FieldMapping: map[string]domain.FieldMappingTarget{
			"vendor": {Target: "master.brand"},
		},
		CategoryMapping: map[string]domain.CategoryMappingTarget{
			"gid:123": {TenantLabel: "Cleansers", Kind: "category"},
		},
	}
	b := NewArtifactBuilder(tier1)
	a := b.Build("ok", []string{"gtin"}, "master_with_variants")

	if got := a.FieldMapping["vendor"].Target; got != "master.brand" {
		t.Errorf("Tier 1 field not carried: %q", got)
	}
	if got := a.CategoryMapping["gid:123"].TenantLabel; got != "Cleansers" {
		t.Errorf("Tier 1 category not carried: %q", got)
	}
	if a.Version != 1 {
		t.Errorf("Version = %d, want 1", a.Version)
	}
}

func TestArtifactBuilder_AgentOverridesAndAddsTemplate(t *testing.T) {
	b := NewArtifactBuilder(AutoMapTier1Result{})
	b.SetFieldMapping("metafields.custom.scent", domain.FieldMappingTarget{
		Target: "candidate:scent", Type: "enum",
	})
	b.AddTemplate(domain.MasterTemplateProposal{
		Vertical:      "furniture",
		Description:   "tables and chairs",
		MasterFields:  []string{"name", "brand"},
		VariantFields: []string{"color"},
	})
	a := b.Build("done", []string{"gtin"}, "master_with_variants")

	if a.FieldMapping["metafields.custom.scent"].Target != "candidate:scent" {
		t.Errorf("agent mapping missing")
	}
	if len(a.MasterTemplates) != 1 || a.MasterTemplates[0].Vertical != "furniture" {
		t.Errorf("template not recorded: %v", a.MasterTemplates)
	}
}

func TestArtifactBuilder_BuildSnapshotIsImmutable(t *testing.T) {
	b := NewArtifactBuilder(AutoMapTier1Result{})
	b.SetFieldMapping("a", domain.FieldMappingTarget{Target: "master.brand"})
	a := b.Build("notes", []string{"gtin"}, "master_with_variants")
	// Mutate builder after Build — snapshot mustn't change.
	b.SetFieldMapping("a", domain.FieldMappingTarget{Target: "OVERRIDE"})
	if a.FieldMapping["a"].Target != "master.brand" {
		t.Errorf("snapshot was mutated post-Build (got %q)", a.FieldMapping["a"].Target)
	}
}

// =============================================================================
// ToolDispatcher tests — direct dispatch, no LLM in the loop
// =============================================================================

// fakeStaging implements both ShopifyStagingPort and metadataReader.
type fakeStaging struct {
	products []fakeStagingRow
	metadata map[string]json.RawMessage
}

type fakeStagingRow struct {
	sourceID string
	payload  json.RawMessage
}

func (f *fakeStaging) UpsertRaw(_ context.Context, _, _, _ string, _ json.RawMessage) error {
	return nil
}
func (f *fakeStaging) CountByKind(_ context.Context, _ string) (map[string]int, error) {
	return nil, nil
}
func (f *fakeStaging) IterateProducts(_ context.Context, _ string, fn func(sourceID string, payload json.RawMessage, fetchedAt time.Time) error) error {
	for _, r := range f.products {
		if err := fn(r.sourceID, r.payload, time.Now()); err != nil {
			return err
		}
	}
	return nil
}
func (f *fakeStaging) DeleteByTenant(_ context.Context, _ string) error { return nil }

// metadataReader capability
func (f *fakeStaging) GetMetadata(_ context.Context, _, sourceID string) (json.RawMessage, error) {
	if v, ok := f.metadata[sourceID]; ok {
		return v, nil
	}
	return nil, nil
}

func newDispatcher(t *testing.T) (*ToolDispatcher, *ArtifactBuilder, *fakeStaging) {
	t.Helper()
	staging := &fakeStaging{
		products: []fakeStagingRow{
			{
				sourceID: "gid://shopify/Product/1",
				payload:  json.RawMessage(`{"title":"Test Product","vendor":"Acme","handle":"test","productType":"Cream"}`),
			},
			{
				sourceID: "gid://shopify/Product/2",
				payload:  json.RawMessage(`{"title":"Another","vendor":"Acme","handle":"another"}`),
			},
		},
	}
	report := &domain.MetaReport{
		TenantID:      "tenant-1",
		TotalProducts: 2,
		Fields: []domain.FieldStats{
			{Name: "vendor", Path: "vendor", OwnerType: "product", Frequency: 2, InferredType: "string"},
			{Name: "title", Path: "title", OwnerType: "product", Frequency: 2, InferredType: "string"},
		},
	}
	builder := NewArtifactBuilder(AutoMapTier1Result{})
	d := &ToolDispatcher{
		report:   report,
		staging:  staging,
		variants: nil,
		embedder: nil,
		builder:  builder,
	}
	return d, builder, staging
}

func TestDispatch_DescribeField_Found(t *testing.T) {
	d, _, _ := newDispatcher(t)
	out, isErr := d.Dispatch(context.Background(), "describe_field", json.RawMessage(`{"path":"vendor"}`))
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}
	if !strings.Contains(out, `"name":"vendor"`) {
		t.Errorf("output missing name: %s", out)
	}
}

func TestDispatch_DescribeField_NotFound(t *testing.T) {
	d, _, _ := newDispatcher(t)
	out, isErr := d.Dispatch(context.Background(), "describe_field", json.RawMessage(`{"path":"unknown"}`))
	if !isErr {
		t.Errorf("expected isError=true for unknown field")
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("output missing 'not found': %s", out)
	}
}

func TestDispatch_SampleRecords_DefaultLimits(t *testing.T) {
	d, _, _ := newDispatcher(t)
	out, isErr := d.Dispatch(context.Background(), "sample_records", json.RawMessage(`{}`))
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}
	var got struct {
		Count   int                          `json:"count"`
		Samples []map[string]json.RawMessage `json:"samples"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Count != 2 {
		t.Errorf("count=%d, want 2", got.Count)
	}
	if _, ok := got.Samples[0]["title"]; !ok {
		t.Errorf("default fields missing 'title': %v", got.Samples[0])
	}
}

func TestDispatch_SampleRecords_FilterAndFields(t *testing.T) {
	d, _, _ := newDispatcher(t)
	out, isErr := d.Dispatch(context.Background(), "sample_records", json.RawMessage(`{"fields":["productType"],"filter_field":"productType","limit":5}`))
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}
	var got struct {
		Count   int                          `json:"count"`
		Samples []map[string]json.RawMessage `json:"samples"`
	}
	_ = json.Unmarshal([]byte(out), &got)
	// Only product 1 has productType set; 2 should be filtered out.
	if got.Count != 1 {
		t.Errorf("count=%d, want 1 (filter)", got.Count)
	}
	if _, ok := got.Samples[0]["title"]; ok {
		t.Errorf("'title' should not be in selected fields: %v", got.Samples[0])
	}
}

func TestDispatch_ProposeFieldMapping_RecordsAndValidates(t *testing.T) {
	d, b, _ := newDispatcher(t)
	out, isErr := d.Dispatch(context.Background(), "propose_field_mapping", json.RawMessage(`{"field_path":"vendor","target":"master.brand"}`))
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}
	if got := b.fieldMapping["vendor"].Target; got != "master.brand" {
		t.Errorf("not recorded: %q", got)
	}
}

func TestDispatch_ProposeFieldMapping_CandidateRequiresVertical(t *testing.T) {
	d, _, _ := newDispatcher(t)
	out, isErr := d.Dispatch(context.Background(), "propose_field_mapping", json.RawMessage(`{"field_path":"x","target":"candidate:scent"}`))
	if !isErr {
		t.Fatalf("expected error for candidate without vertical")
	}
	if !strings.Contains(out, "vertical is required") {
		t.Errorf("error missing 'vertical is required': %s", out)
	}
}

func TestDispatch_ProposeMasterTemplate_RejectsCosmetics(t *testing.T) {
	d, _, _ := newDispatcher(t)
	out, isErr := d.Dispatch(context.Background(), "propose_master_template", json.RawMessage(`{"vertical":"cosmetics","description":"x","master_fields":["a"]}`))
	if !isErr {
		t.Fatalf("expected error proposing cosmetics")
	}
	if !strings.Contains(out, "already promoted") {
		t.Errorf("error missing 'already promoted': %s", out)
	}
}

func TestDispatch_ProposeMasterTemplate_Success(t *testing.T) {
	d, b, _ := newDispatcher(t)
	out, isErr := d.Dispatch(context.Background(), "propose_master_template", json.RawMessage(`{"vertical":"furniture","description":"chairs and tables","master_fields":["name","brand"],"variant_fields":["color"]}`))
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}
	if len(b.templates) != 1 || b.templates[0].Vertical != "furniture" {
		t.Errorf("template not recorded: %+v", b.templates)
	}
}

func TestDispatch_ProposeCategoryMapping_KindEnumGuard(t *testing.T) {
	d, _, _ := newDispatcher(t)
	out, isErr := d.Dispatch(context.Background(), "propose_category_mapping", json.RawMessage(`{"shopify_collection_id":"gid:1","tenant_label":"Promo","kind":"sale"}`))
	if !isErr {
		t.Fatalf("expected error for invalid kind")
	}
	if !strings.Contains(out, "category|showcase|promo") {
		t.Errorf("error missing kind enum: %s", out)
	}
}

func TestDispatch_CommitArtifact_Finalizes(t *testing.T) {
	d, _, _ := newDispatcher(t)
	d.Dispatch(context.Background(), "propose_field_mapping", json.RawMessage(`{"field_path":"vendor","target":"master.brand"}`))
	out, isErr := d.Dispatch(context.Background(), "commit_artifact", json.RawMessage(`{"agent_notes":"first pass"}`))
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}
	if d.CommittedArtifact == nil {
		t.Fatal("artifact not set")
	}
	if d.CommittedArtifact.AgentNotes != "first pass" {
		t.Errorf("notes = %q", d.CommittedArtifact.AgentNotes)
	}
	if len(d.CommittedArtifact.MatchStrategy) == 0 {
		t.Error("default match strategy not applied")
	}
}

func TestDispatch_UnknownTool(t *testing.T) {
	d, _, _ := newDispatcher(t)
	out, isErr := d.Dispatch(context.Background(), "what_is_this", json.RawMessage(`{}`))
	if !isErr {
		t.Fatalf("expected error for unknown tool")
	}
	if !strings.Contains(out, "unknown tool") {
		t.Errorf("error missing 'unknown tool': %s", out)
	}
}

// =============================================================================
// Loop tests — scripted fake LLM
// =============================================================================

// scriptedSender returns canned MessagesResponse values in sequence. Each call
// to Send pops the next response. Useful for asserting loop control flow
// without burning real tokens.
type scriptedSender struct {
	responses []anthropic.MessagesResponse
	calls     int
}

func (s *scriptedSender) Send(_ context.Context, _ anthropic.MessagesRequest) (*anthropic.MessagesResponse, error) {
	if s.calls >= len(s.responses) {
		// Defensive: should never happen in well-written tests.
		return &anthropic.MessagesResponse{
			Content:    []anthropic.ContentBlock{{Type: "text", Text: "(out of script)"}},
			StopReason: "end_turn",
		}, nil
	}
	r := s.responses[s.calls]
	s.calls++
	return &r, nil
}

func TestDiscover_HappyPath_CommitArtifact(t *testing.T) {
	staging := &fakeStaging{}
	report := &domain.MetaReport{TenantID: "t1", TotalProducts: 1}
	tier1 := AutoMapTier1Result{
		FieldMapping: map[string]domain.FieldMappingTarget{
			"vendor": {Target: "master.brand"},
		},
	}

	// Script: 1 turn — agent emits commit_artifact tool_use, loop terminates.
	llm := &scriptedSender{
		responses: []anthropic.MessagesResponse{
			{
				Content: []anthropic.ContentBlock{
					{Type: "text", Text: "Looks like cosmetics. Committing."},
					{
						Type:  "tool_use",
						ID:    "toolu_1",
						Name:  "commit_artifact",
						Input: json.RawMessage(`{"agent_notes":"trivial mapping"}`),
					},
				},
				StopReason: "tool_use",
				Usage:      anthropic.Usage{InputTokens: 1000, OutputTokens: 100},
			},
		},
	}

	da := NewDiscoveryAgent(llm, staging, &fakeVariantsPort{}, nil, newTestLogger())
	res, err := da.Discover(context.Background(), "t1", report, tier1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != domain.MappingArtifactStatusActive {
		t.Errorf("status = %s, want active", res.Status)
	}
	if res.StopReason != "commit_artifact" {
		t.Errorf("stop = %q", res.StopReason)
	}
	if res.Artifact == nil {
		t.Fatal("artifact nil")
	}
	if res.Artifact.FieldMapping["vendor"].Target != "master.brand" {
		t.Error("Tier 1 mapping lost on commit")
	}
}

func TestDiscover_EndTurnWithoutCommit_NeedsReview(t *testing.T) {
	llm := &scriptedSender{
		responses: []anthropic.MessagesResponse{
			{
				Content:    []anthropic.ContentBlock{{Type: "text", Text: "I give up."}},
				StopReason: "end_turn",
			},
		},
	}
	da := NewDiscoveryAgent(llm, &fakeStaging{}, &fakeVariantsPort{}, nil, newTestLogger())
	res, _ := da.Discover(context.Background(), "t1", &domain.MetaReport{TenantID: "t1"}, AutoMapTier1Result{})
	if res.Status != domain.MappingArtifactStatusNeedsHumanReview {
		t.Errorf("status = %s, want needs_human_review", res.Status)
	}
	if res.StopReason != "end_turn_without_commit" {
		t.Errorf("stop = %q", res.StopReason)
	}
}

// =============================================================================
// A1+A2 — discovery prompt + brand mapping with vertical
// =============================================================================

// Sticky test: legacy `master_cosmetics.X` syntax must not reappear in the
// system prompt. extractTier2 only accepts `master.tier2.*` / `tier2.*` and
// silently drops everything else, so any drift here makes the agent generate
// mappings that get thrown away at apply.
func TestBuildDiscoverySystemPrompt_NoLegacyMasterCosmeticsTargets(t *testing.T) {
	report := &domain.MetaReport{TenantID: "t1"}
	prompt := buildDiscoverySystemPrompt(report, AutoMapTier1Result{}, nil)

	if strings.Contains(prompt, "master_cosmetics.") {
		t.Errorf("system prompt still mentions legacy `master_cosmetics.` target — extractTier2 silently drops these. Found in:\n%s",
			extractContextAround(prompt, "master_cosmetics."))
	}
	if !strings.Contains(prompt, "master.tier2.<key>") {
		t.Error("system prompt missing the `master.tier2.<key>` target instruction — agent won't know how to map vertical-specific fields")
	}
}

func extractContextAround(s, needle string) string {
	idx := strings.Index(s, needle)
	if idx < 0 {
		return ""
	}
	start := idx - 80
	if start < 0 {
		start = 0
	}
	end := idx + len(needle) + 80
	if end > len(s) {
		end = len(s)
	}
	return "..." + s[start:end] + "..."
}

func TestDispatch_ProposeBrandMapping_CreateNewRequiresVertical(t *testing.T) {
	d, _, _ := newDispatcher(t)
	out, isErr := d.Dispatch(context.Background(), "propose_brand_mapping",
		json.RawMessage(`{"vendor":"IKEA","action":"create_new"}`))
	if !isErr {
		t.Fatalf("expected error for create_new without vertical, got: %s", out)
	}
	if !strings.Contains(out, "vertical is required") {
		t.Errorf("error missing 'vertical is required': %s", out)
	}
}

func TestDispatch_ProposeBrandMapping_CreateNewWithVerticalSucceeds(t *testing.T) {
	d, b, _ := newDispatcher(t)
	out, isErr := d.Dispatch(context.Background(), "propose_brand_mapping",
		json.RawMessage(`{"vendor":"IKEA","action":"create_new","vertical":"furniture"}`))
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}
	got, ok := b.brandMapping["ikea"]
	if !ok {
		t.Fatal("brand mapping not recorded")
	}
	if got.Action != "create_new" || got.Vertical != "furniture" {
		t.Errorf("recorded = %+v, want action=create_new vertical=furniture", got)
	}
}

func TestDispatch_ProposeBrandMapping_LinkExistingVerticalOptional(t *testing.T) {
	d, b, _ := newDispatcher(t)
	out, isErr := d.Dispatch(context.Background(), "propose_brand_mapping",
		json.RawMessage(`{"vendor":"COSRX","action":"link_existing","master_brand":"COSRX"}`))
	if isErr {
		t.Fatalf("link_existing without vertical should be accepted, got: %s", out)
	}
	if got := b.brandMapping["cosrx"].Action; got != "link_existing" {
		t.Errorf("action = %q", got)
	}
}

// Sonnet sometimes emits HTML-encoded ampersands ("Stone &amp; Steel") even
// though the source data has the raw "&". Without decoding, the lowercase key
// "stone &amp; steel" never matches a listing's vendor "Stone & Steel" and
// resolveVertical falls through to "unknown".
func TestDispatch_ProposeBrandMapping_DecodesHTMLEntitiesInVendor(t *testing.T) {
	d, b, _ := newDispatcher(t)
	out, isErr := d.Dispatch(context.Background(), "propose_brand_mapping",
		json.RawMessage(`{"vendor":"Stone &amp; Steel","action":"create_new","vertical":"furniture"}`))
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}
	// SetBrandMapping lowercases. We expect the "&amp;" to be decoded back to
	// "&" before the lowercase, so the key is "stone & steel" (the form a
	// listing's vendor would normalize to).
	if _, ok := b.brandMapping["stone & steel"]; !ok {
		t.Errorf("expected key 'stone & steel', got: %v", keysOf(b.brandMapping))
	}
	if _, bad := b.brandMapping["stone &amp; steel"]; bad {
		t.Errorf("HTML-encoded key 'stone &amp; steel' was stored — html.UnescapeString didn't run")
	}
}

func keysOf(m map[string]domain.BrandMappingTarget) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestDispatch_ProposeBrandMapping_LinkExistingVerticalAccepted(t *testing.T) {
	d, b, _ := newDispatcher(t)
	out, isErr := d.Dispatch(context.Background(), "propose_brand_mapping",
		json.RawMessage(`{"vendor":"COSRX","action":"link_existing","master_brand":"COSRX","vertical":"cosmetics"}`))
	if isErr {
		t.Fatalf("unexpected error: %s", out)
	}
	if got := b.brandMapping["cosrx"].Vertical; got != "cosmetics" {
		t.Errorf("vertical hint not stored, got %q", got)
	}
}

func TestDiscover_TokenBudgetExceeded(t *testing.T) {
	// Single response burns the entire budget.
	llm := &scriptedSender{
		responses: []anthropic.MessagesResponse{
			{
				Content: []anthropic.ContentBlock{
					{
						Type:  "tool_use",
						ID:    "toolu_1",
						Name:  "describe_field",
						Input: json.RawMessage(`{"path":"vendor"}`),
					},
				},
				StopReason: "tool_use",
				Usage:      anthropic.Usage{InputTokens: 200_000, OutputTokens: 100},
			},
		},
	}
	staging := &fakeStaging{}
	report := &domain.MetaReport{
		TenantID: "t1",
		Fields: []domain.FieldStats{
			{Name: "vendor", Path: "vendor", OwnerType: "product"},
		},
	}
	da := NewDiscoveryAgent(llm, staging, &fakeVariantsPort{}, nil, newTestLogger())
	res, _ := da.Discover(context.Background(), "t1", report, AutoMapTier1Result{})
	if res.Status != domain.MappingArtifactStatusNeedsHumanReview {
		t.Errorf("status = %s", res.Status)
	}
	if res.StopReason != "token_budget_exceeded" {
		t.Errorf("stop = %q", res.StopReason)
	}
}
