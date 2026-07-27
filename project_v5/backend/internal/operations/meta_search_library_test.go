package operations

import (
	"context"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
)

func libraryHits() []domain.OperationTemplate {
	return []domain.OperationTemplate{
		{
			Name: "schedule_slot", Kind: domain.KindScheduleSlot,
			Title:       "Book a time slot",
			Description: "Book a date and time against a catalog item.",
			Card: map[string]any{
				"input_summary":  "Contact details plus a preferred date and time",
				"does":           "Checks the datetime and creates the scheduling record",
				"output_summary": "A confirmed booking record",
				"why":            "Converts interest into a trackable appointment",
			},
		},
		{
			Name: "crm_starter_pack", Kind: domain.KindPresetPack,
			Title:       "CRM starter pack",
			Description: "Record table and detail presets for a staff CRM.",
		},
	}
}

// search_library is IMMEDIATE: hits land in the manifest's libraryContext
// AND as the synthetic opCard EntitySet (R23) whose Data keys are the
// operation_card preset's binding vocabulary; Agent1 gets a one-line digest
// naming the matches.
func TestSearchLibraryLandsBothZones(t *testing.T) {
	ob := &fakeOnboardingState{}
	state := newOpsStatePort()
	store := &fakeLibraryStore{hits: libraryHits()}
	deps := metaDeps(ob, state)
	deps.Store = store
	ex := NewSearchLibraryExecutor(deps)

	res, err := ex.Execute(context.Background(), onboardingOctx(), map[string]any{
		"query": "let visitors book appointments", "kind": "any", "limit": 5,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeOK || res.Count != 2 {
		t.Fatalf("result = %+v", res)
	}
	if want := "ok: 2 library matches: schedule_slot (schedule_slot), crm_starter_pack (preset_pack)"; res.Summary != want {
		t.Errorf("summary = %q", res.Summary)
	}

	// kind "any" searches executable templates + preset packs — never
	// meta/visual/internal plumbing.
	wantKinds := []string{"query", "create_record", "update_record", "transition_status", "schedule_slot", "notify", "preset_pack"}
	if len(store.lastKinds) != len(wantKinds) {
		t.Fatalf("kinds = %v", store.lastKinds)
	}
	for i, k := range wantKinds {
		if store.lastKinds[i] != k {
			t.Errorf("kinds[%d] = %q, want %q", i, store.lastKinds[i], k)
		}
	}
	if store.lastK != 5 || store.lastQuery != "let visitors book appointments" {
		t.Errorf("store call = k %d query %q", store.lastK, store.lastQuery)
	}

	// Manifest libraryContext.
	if ob.manifest == nil || len(ob.manifest.LibraryContext) != 2 {
		t.Fatalf("libraryContext = %+v", ob.manifest)
	}
	hit := ob.manifest.LibraryContext[0]
	if hit.Name != "schedule_slot" || hit.Card["why"] == nil {
		t.Errorf("hit = %+v", hit)
	}

	// Synthetic opCard set: binding vocabulary of operation_card.json.
	var set *domain.EntitySet
	for i := range state.state.Current.Data.Entities {
		if state.state.Current.Data.Entities[i].Slug == "opCard" {
			set = &state.state.Current.Data.Entities[i]
		}
	}
	if set == nil {
		t.Fatal("opCard set not written")
	}
	if !set.Synthetic || len(set.Records) != 2 {
		t.Fatalf("opCard set = %+v", set)
	}
	rec := set.Records[0]
	if rec.ID != "schedule_slot" {
		t.Errorf("record id = %q", rec.ID)
	}
	for _, key := range []string{"title", "kind", "description", "inputSummary", "does", "outputSummary", "why"} {
		if _, ok := rec.Data[key]; !ok {
			t.Errorf("opCard record missing binding key %q: %+v", key, rec.Data)
		}
	}
	if state.updatedMeta.EntityCounts["opCard"] != 2 {
		t.Errorf("EntityCounts = %+v", state.updatedMeta.EntityCounts)
	}
	if state.updatedInfo.Path != "data" || state.updatedInfo.Action.Type != domain.ActionEntityQuery {
		t.Errorf("data delta = %+v", state.updatedInfo)
	}
}

// kind "preset" narrows the search to preset packs; a repeat search merges
// by name instead of duplicating libraryContext entries.
func TestSearchLibraryKindFilterAndDedupe(t *testing.T) {
	ob := &fakeOnboardingState{}
	store := &fakeLibraryStore{hits: libraryHits()[1:]}
	deps := metaDeps(ob, newOpsStatePort())
	deps.Store = store
	ex := NewSearchLibraryExecutor(deps)
	octx := onboardingOctx()

	for range 2 {
		if _, err := ex.Execute(context.Background(), octx, map[string]any{
			"query": "interface kit", "kind": "preset",
		}); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	}
	if len(store.lastKinds) != 1 || store.lastKinds[0] != "preset_pack" {
		t.Errorf("kinds = %v", store.lastKinds)
	}
	if len(ob.manifest.LibraryContext) != 1 {
		t.Errorf("dedupe failed: %+v", ob.manifest.LibraryContext)
	}
}

// No hits → structured empty, and NOTHING persists (no manifest write, no
// state write) — an empty search must not blank an earlier opCard set.
func TestSearchLibraryEmpty(t *testing.T) {
	ob := &fakeOnboardingState{}
	state := newOpsStatePort()
	deps := metaDeps(ob, state)
	ex := NewSearchLibraryExecutor(deps)

	res, err := ex.Execute(context.Background(), onboardingOctx(), map[string]any{"query": "quantum accounting"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeEmpty || !strings.Contains(res.Summary, "empty:") {
		t.Errorf("result = %+v", res)
	}
	if len(ob.infos) != 0 || state.updatedData != nil {
		t.Errorf("empty search persisted: infos=%d data=%v", len(ob.infos), state.updatedData)
	}
}
