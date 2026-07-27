package operations

import (
	"context"
	"fmt"
	"strings"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// SearchLibraryExecutor — the one IMMEDIATE meta-operation (RUNTIME_SPEC.md
// §4.3): semantic search over the system library (v5_operations — executor
// templates + preset_pack rows, R30) whose results are usable in the SAME
// turn:
//
//   - manifest libraryContext gains the hits (what the plan proposes from),
//   - the synthetic `opCard` EntitySet (R23) lands in the Entities zone so
//     Agent2's compose_turn can replicate the operation_card preset over it
//     (beat 2: input → does → output → why),
//   - Agent1 gets a one-line digest naming the matches, so it can stage
//     enable_operations / adopt_presets against real template names.
//
// Embedder nil (or failing) degrades to the store's FTS fallback, mirroring
// catalog_search.

// opCardSetSlug is the synthetic EntitySet slug the operation_card preset
// replicates over (compose_turn `replicate: "opCard"`).
const opCardSetSlug = "opCard"

// libraryOperationKinds are the enable-able executor template kinds —
// what `kind: "operation"` searches. Meta/visual/internal templates are
// runtime plumbing, never proposals.
var libraryOperationKinds = []string{
	string(domain.KindQuery), string(domain.KindCreateRecord),
	string(domain.KindUpdateRecord), string(domain.KindTransitionStatus),
	string(domain.KindScheduleSlot), string(domain.KindNotify),
}

const (
	searchLibraryDefaultLimit = 8
	searchLibraryMaxLimit     = 20
)

// SearchLibraryExecutor implements ports.Executor for search_library.
type SearchLibraryExecutor struct {
	deps MetaExecutorDeps
	tmpl domain.OperationTemplate
}

var _ ports.Executor = (*SearchLibraryExecutor)(nil)

// NewSearchLibraryExecutor builds the executor over the meta deps.
func NewSearchLibraryExecutor(deps MetaExecutorDeps) *SearchLibraryExecutor {
	return &SearchLibraryExecutor{deps: deps, tmpl: seedRow("search_library")}
}

func (e *SearchLibraryExecutor) Template() domain.OperationSpec        { return e.tmpl.Spec() }
func (e *SearchLibraryExecutor) TemplateRow() domain.OperationTemplate { return e.tmpl }

func (e *SearchLibraryExecutor) SpecForTenant(context.Context, domain.Tenant, map[string]any) (domain.OperationSpec, error) {
	return e.tmpl.Spec(), nil
}

// Execute runs the search and lands the results in both zones.
func (e *SearchLibraryExecutor) Execute(ctx context.Context, octx domain.OperationContext, input map[string]any) (*domain.OperationResult, error) {
	query := stringInput(input, "query")
	limit := searchLibraryDefaultLimit
	if v, ok := input["limit"].(int); ok && v > 0 {
		limit = v
	}
	if limit > searchLibraryMaxLimit {
		limit = searchLibraryMaxLimit
	}

	var kinds []string
	switch stringInput(input, "kind") {
	case "operation":
		kinds = libraryOperationKinds
	case "preset":
		kinds = []string{string(domain.KindPresetPack)}
	default: // "any" or absent
		kinds = append(append([]string{}, libraryOperationKinds...), string(domain.KindPresetPack))
	}

	// Best-effort embedding: any failure degrades to FTS (nil vector), the
	// same contract the boot seeder and catalog_search follow.
	var embedding []float32
	if e.deps.Embedder != nil && query != "" {
		if vecs, err := e.deps.Embedder.Embed(ctx, []string{query}); err == nil && len(vecs) == 1 {
			embedding = vecs[0]
		} else if err != nil {
			e.deps.logger().Warn("search_library: query embedding failed — FTS fallback", "err", err)
		}
	}

	hits, err := e.deps.Store.SearchLibrary(ctx, embedding, query, kinds, limit)
	if err != nil {
		return nil, fmt.Errorf("search library: %w", err)
	}
	if len(hits) == 0 {
		return &domain.OperationResult{
			Kind:    e.tmpl.Kind,
			Outcome: domain.OutcomeEmpty,
			Summary: fmt.Sprintf("empty: no library matches for %q", query),
			Output:  map[string]any{"count": 0},
		}, nil
	}

	if octx.SessionID != "" {
		// Manifest libraryContext: merge by name (a repeat search refreshes
		// an entry instead of duplicating it), append order preserved.
		m, err := loadOrNewManifest(ctx, e.deps, octx.SessionID)
		if err != nil {
			return nil, err
		}
		for _, t := range hits {
			mergeLibraryHit(m, domain.LibraryHit{
				Name:        t.Name,
				Kind:        t.Kind,
				Title:       t.Title,
				Description: t.Description,
				Card:        t.Card,
			})
		}
		if err := persistOnboarding(ctx, e.deps, octx, m, e.tmpl.Name, map[string]any{
			"op": e.tmpl.Name, "query": query, "count": len(hits),
		}); err != nil {
			return nil, err
		}

		// Synthetic opCard EntitySet (R23): bound via EntityToMap like any
		// entity set, never persisted to v5_entity_records. Data keys are the
		// operation_card preset's binding vocabulary (§6.2 camelCase).
		if err := writeSyntheticSets(ctx, e.deps, octx, e.tmpl.Name, opCardSet(hits)); err != nil {
			return nil, err
		}
	}

	names := make([]string, len(hits))
	for i, t := range hits {
		names[i] = fmt.Sprintf("%s (%s)", t.Name, t.Kind)
	}
	return &domain.OperationResult{
		Kind:       e.tmpl.Kind,
		Outcome:    domain.OutcomeOK,
		Count:      len(hits),
		EntityKind: opCardSetSlug,
		Summary:    fmt.Sprintf("ok: %d library matches: %s", len(hits), strings.Join(names, ", ")),
		Output:     map[string]any{"count": len(hits)},
		Metadata:   map[string]any{"entity": opCardSetSlug, "count": len(hits)},
	}, nil
}

// mergeLibraryHit upserts one hit into the manifest's libraryContext by
// template name.
func mergeLibraryHit(m *domain.OnboardingManifest, hit domain.LibraryHit) {
	for i := range m.LibraryContext {
		if m.LibraryContext[i].Name == hit.Name {
			m.LibraryContext[i] = hit
			return
		}
	}
	m.LibraryContext = append(m.LibraryContext, hit)
}

// opCardSet builds the synthetic opCard EntitySet from library hits. Record
// id = template name (the EntityToMap `id` contract); Data keys match the
// operation_card seed's fieldBindings: title, kind, description,
// inputSummary, does, outputSummary, why.
func opCardSet(hits []domain.OperationTemplate) domain.EntitySet {
	fields := make([]domain.FieldDef, 0, 7)
	for _, key := range []string{"title", "kind", "description", "inputSummary", "does", "outputSummary", "why"} {
		fields = append(fields, domain.FieldDef{Key: key, Label: key, Type: domain.FieldText})
	}
	records := make([]domain.EntityRecord, 0, len(hits))
	for _, t := range hits {
		data := map[string]any{
			"title":       t.Title,
			"kind":        string(t.Kind),
			"description": t.Description,
		}
		for cardKey, dataKey := range map[string]string{
			"input_summary":  "inputSummary",
			"does":           "does",
			"output_summary": "outputSummary",
			"why":            "why",
		} {
			if s, ok := t.Card[cardKey].(string); ok && s != "" {
				data[dataKey] = s
			}
		}
		records = append(records, domain.EntityRecord{
			ID:         t.Name,
			EntitySlug: opCardSetSlug,
			Data:       data,
		})
	}
	return domain.EntitySet{
		Slug:      opCardSetSlug,
		Name:      "Operation cards",
		Fields:    fields,
		Records:   records,
		Synthetic: true,
	}
}
