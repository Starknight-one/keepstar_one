package operations

import (
	"context"
	"fmt"
	"strings"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/tools"
)

// QueryExecutor is the config-driven read operation (RUNTIME_SPEC.md §4.2):
//
//   - config {"source":"catalog"} — the canonical-data path: reuses the
//     proven catalog search flow verbatim (hybrid retrieval → Products
//     zone), schema = CatalogSearchSchemaForDigest.
//   - config {"source":"entity","entity":"lead"} — the entity plane:
//     flat filtered QueryRecords → Entities zone via StatePort.UpdateData,
//     delta ENTITY_QUERY with Path "data" (R28), schema derived from the
//     entity definition + value sets.
//
// Demo instances: lead_search (entity). Catalog canonical search stays on
// the auto-enabled legacy catalog_search wrap until its M-later move; the
// catalog branch here serves catalog-sourced instances.
type QueryExecutor struct {
	state       ports.StatePort
	catalog     ports.CatalogPort
	entities    ports.EntityPort
	catalogTool tools.Tool // the existing catalog search flow, reused verbatim
	tmpl        domain.OperationTemplate
}

var _ ports.Executor = (*QueryExecutor)(nil)

// NewQueryExecutor wires the executor. embedding may be nil (catalog branch
// degrades to FTS-only, mirroring catalog_search).
func NewQueryExecutor(state ports.StatePort, catalog ports.CatalogPort, entities ports.EntityPort, embedding ports.EmbeddingPort) *QueryExecutor {
	return &QueryExecutor{
		state:       state,
		catalog:     catalog,
		entities:    entities,
		catalogTool: tools.NewCatalogSearchTool(state, catalog, embedding),
		tmpl:        seedRow("query"),
	}
}

func (e *QueryExecutor) Template() domain.OperationSpec        { return e.tmpl.Spec() }
func (e *QueryExecutor) TemplateRow() domain.OperationTemplate { return e.tmpl }

// SpecForTenant derives the instance schema from the configured source:
// catalog → CatalogSearchSchemaForDigest verbatim; entity → the definition
// + value-set derived filter schema. No config (template surface) or a
// failed derivation keeps the static template schema (fail-open, same as
// the legacy wrap).
func (e *QueryExecutor) SpecForTenant(ctx context.Context, t domain.Tenant, cfg map[string]any) (domain.OperationSpec, error) {
	spec := e.tmpl.Spec()
	switch cfgString(cfg, "source") {
	case "catalog":
		digest, err := e.catalog.BuildCatalogDigest(ctx, t.ID)
		if err != nil {
			return spec, nil
		}
		if schema := tools.CatalogSearchSchemaForDigest(digest); schema != nil {
			spec.InputSchema = schema
		}
	case "entity":
		def, sets, err := e.loadDefinition(ctx, t.ID, cfgString(cfg, "entity"))
		if err != nil {
			return spec, nil
		}
		if schema := entityQuerySchema(def, sets); schema != nil {
			spec.InputSchema = schema
			spec.Description = fmt.Sprintf("Search and filter %s. %s", entityDisplayName(def, true), spec.Description)
		}
	}
	return spec, nil
}

// Execute routes on the instance config. Input arrives validated+coerced
// against the SpecForTenant schema by the registry choke point.
func (e *QueryExecutor) Execute(ctx context.Context, octx domain.OperationContext, input map[string]any) (*domain.OperationResult, error) {
	if cfgString(octx.Config, "source") == "entity" {
		return e.executeEntity(ctx, octx, input)
	}
	return e.executeCatalog(ctx, octx, input)
}

// ─── catalog branch ──────────────────────────────────────────────────────

// executeCatalog reuses the existing catalog search flow verbatim: same
// retrieval, same Products-zone write, same Summary bytes ("ok: found N
// products" / "empty: 0 results, previous data preserved").
func (e *QueryExecutor) executeCatalog(ctx context.Context, octx domain.OperationContext, input map[string]any) (*domain.OperationResult, error) {
	res, err := e.catalogTool.Execute(ctx, domain.ToolContext{
		SessionID:  octx.SessionID,
		TenantSlug: octx.TenantSlug,
		TurnID:     octx.TurnID,
	}, catalogToolInput(input))
	if err != nil {
		return nil, err
	}
	if res == nil {
		return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeError, "error: catalog search returned no result"), nil
	}
	result := &domain.OperationResult{
		Kind:    e.tmpl.Kind,
		Outcome: legacyOutcome(res),
		Summary: res.Content, // byte-exact — prompt-cache duty C3
		// EntityKind deliberately empty: catalog query results speak the
		// legacy "new_search: N items found" microcontext phrase and get
		// their Count stamped from the reloaded state (stampStateCounts).
		Metadata: res.Metadata,
	}
	if n, ok := res.Metadata["count"].(int); ok {
		result.Count = n
		result.Output = map[string]any{"count": n}
	}
	return result, nil
}

// catalogToolInput bridges the registry-cleaned input back to the legacy
// tool's decoding expectations: the validator returns Go ints for integer
// properties, the tool asserts float64 (raw-JSON heritage).
func catalogToolInput(input map[string]any) map[string]any {
	v, isInt := input["limit"].(int)
	if !isInt {
		return input
	}
	out := make(map[string]any, len(input))
	for k, val := range input {
		out[k] = val
	}
	out["limit"] = float64(v)
	return out
}

// ─── entity branch ───────────────────────────────────────────────────────

func (e *QueryExecutor) executeEntity(ctx context.Context, octx domain.OperationContext, input map[string]any) (*domain.OperationResult, error) {
	slug := cfgString(octx.Config, "entity")
	if slug == "" {
		return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeError, "error: query instance config has no entity"), nil
	}
	def, sets, err := e.loadDefinition(ctx, octx.TenantID, slug)
	if err != nil {
		return entityFailure(e.tmpl.Name, e.tmpl.Kind, err)
	}

	filter := entityRecordFilter(input)
	records, total, err := e.entities.QueryRecords(ctx, octx.TenantID, slug, filter)
	if err != nil {
		return nil, fmt.Errorf("query %s records: %w", slug, err)
	}

	set := domain.EntitySet{
		Slug:    def.Slug,
		Name:    entityDisplayName(def, true),
		Fields:  def.Fields,
		Labels:  valueSetLabels(def, sets),
		Records: records,
	}
	if err := e.writeEntitiesZone(ctx, octx, def, set, input); err != nil {
		return nil, err
	}

	plural := entityDisplayName(def, true)
	result := &domain.OperationResult{
		Kind:       e.tmpl.Kind,
		Count:      len(records),
		EntityKind: def.Slug,
		Output:     map[string]any{"count": len(records)},
		Metadata:   map[string]any{"entity": def.Slug, "count": len(records), "total": total},
	}
	if len(records) == 0 {
		result.Outcome = domain.OutcomeEmpty
		result.Summary = fmt.Sprintf("empty: no %s found", strings.ToLower(plural))
	} else {
		result.Outcome = domain.OutcomeOK
		result.Summary = fmt.Sprintf("ok: found %d %s", len(records), strings.ToLower(plural))
	}
	return result, nil
}

// writeEntitiesZone upserts the loaded set into StateData.Entities (other
// sets and the Products/Services zones are preserved — an entity query
// must not blank the catalog context) and appends the ENTITY_QUERY delta,
// Path "data" (R28), in the same zone write.
func (e *QueryExecutor) writeEntitiesZone(ctx context.Context, octx domain.OperationContext, def *domain.EntityDefinition, set domain.EntitySet, input map[string]any) error {
	st, err := e.state.GetState(ctx, octx.SessionID)
	if err != nil {
		return fmt.Errorf("get state: %w", err)
	}

	data := st.Current.Data
	replaced := false
	for i := range data.Entities {
		if data.Entities[i].Slug == set.Slug {
			data.Entities[i] = set
			replaced = true
			break
		}
	}
	if !replaced {
		data.Entities = append(data.Entities, set)
	}

	meta := st.Current.Meta
	if meta.EntityCounts == nil {
		meta.EntityCounts = map[string]int{}
	}
	meta.EntityCounts[set.Slug] = len(set.Records)

	fields := make([]string, 0, len(def.Fields))
	for _, f := range def.Fields {
		fields = append(fields, f.Key)
	}
	actor := octx.ActorID
	if actor == "" {
		actor = "agent1"
	}
	info := domain.DeltaInfo{
		TurnID:    octx.TurnID,
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   actor,
		DeltaType: domain.DeltaTypeAdd,
		Path:      "data",
		Action: domain.Action{
			Type:   domain.ActionEntityQuery,
			Tool:   e.tmpl.Name,
			Params: map[string]any{"entity": set.Slug, "input": input},
		},
		Result: domain.ResultMeta{Count: len(set.Records), Fields: fields},
	}
	if _, err := e.state.UpdateData(ctx, octx.SessionID, data, meta, info); err != nil {
		return fmt.Errorf("update entities zone: %w", err)
	}
	return nil
}

func (e *QueryExecutor) loadDefinition(ctx context.Context, tenantID, slug string) (*domain.EntityDefinition, map[string]domain.ValueSet, error) {
	def, err := e.entities.GetEntityDefinition(ctx, tenantID, slug)
	if err != nil {
		return nil, nil, err
	}
	sets, err := e.entities.ListValueSets(ctx, tenantID)
	if err != nil {
		return nil, nil, err
	}
	byKey := make(map[string]domain.ValueSet, len(sets))
	for _, vs := range sets {
		byKey[vs.Slug] = vs
	}
	return def, byKey, nil
}

// valueSetLabels builds the EntitySet.Labels lookup (valueSetRef → value →
// label) for every enum field, so binding derives `<key>Label` without a
// second read (§4.5).
func valueSetLabels(def *domain.EntityDefinition, sets map[string]domain.ValueSet) map[string]map[string]string {
	labels := map[string]map[string]string{}
	for _, f := range def.Fields {
		if f.Type != domain.FieldEnum || f.ValueSetRef == "" {
			continue
		}
		vs, ok := sets[f.ValueSetRef]
		if !ok {
			continue
		}
		m := make(map[string]string, len(vs.Values))
		for _, entry := range vs.Values {
			m[entry.Value] = entry.Label
		}
		labels[f.ValueSetRef] = m
	}
	if len(labels) == 0 {
		return nil
	}
	return labels
}

// entityRecordFilter maps the validated input onto the flat RecordFilter:
// status / since / ref_id top-level, exact matches and min_/max_ range
// bounds from the filters sub-object. Money bounds arrive as integer cents
// (registry usd coercion) and pass through as numeric cents — records
// store cents (R18).
func entityRecordFilter(input map[string]any) domain.RecordFilter {
	f := domain.RecordFilter{
		Status:      stringInput(input, "status"),
		RefEntityID: stringInput(input, "ref_id"),
		SortField:   stringInput(input, "sort_by"),
		SortOrder:   stringInput(input, "sort_order"),
		Limit:       50,
	}
	if v, ok := input["limit"].(int); ok && v > 0 {
		f.Limit = v
	}
	if s := stringInput(input, "since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			f.Since = &t
		}
	}
	filters, _ := input["filters"].(map[string]any)
	for key, raw := range filters {
		if rest, ok := strings.CutPrefix(key, "min_"); ok && rest != "" {
			if n, isNum := asFilterNumber(raw); isNum {
				if f.AttrMin == nil {
					f.AttrMin = map[string]float64{}
				}
				f.AttrMin[rest] = n
				continue
			}
		}
		if rest, ok := strings.CutPrefix(key, "max_"); ok && rest != "" {
			if n, isNum := asFilterNumber(raw); isNum {
				if f.AttrMax == nil {
					f.AttrMax = map[string]float64{}
				}
				f.AttrMax[rest] = n
				continue
			}
		}
		if s, ok := raw.(string); ok && s != "" {
			if f.Attrs == nil {
				f.Attrs = map[string]string{}
			}
			f.Attrs[key] = s
		}
	}
	return f
}

func stringInput(input map[string]any, key string) string {
	s, _ := input[key].(string)
	return s
}

// asFilterNumber accepts the shapes the validator emits: float64 for plain
// numbers, int for unit-coerced money (cents).
func asFilterNumber(raw any) (float64, bool) {
	switch v := raw.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	}
	return 0, false
}

// entityQuerySchema derives the restricted-dialect query schema from an
// entity definition (§4.4 generators; the sibling of EntityRecordSchema):
// status enum from the pipeline value set, since as RFC3339, ref_id when
// the entity links the catalog, per-field exact matches and min_/max_
// numeric pairs (x-unit annotated) in a filters sub-object, plus sort and
// limit. All properties optional — "any new leads?" is a bare call.
func entityQuerySchema(def *domain.EntityDefinition, sets map[string]domain.ValueSet) map[string]any {
	if def == nil {
		return nil
	}
	props := map[string]any{
		"since": map[string]any{
			"type":               "string",
			"description":        "Only records created at or after this ISO-8601 datetime.",
			domain.SchemaKeyUnit: string(domain.UnitDatetimeISO8601),
		},
		"limit": map[string]any{
			"type":               "integer",
			"description":        "Maximum records to return (default 50).",
			domain.SchemaKeyUnit: string(domain.UnitCount),
		},
		"sort_order": map[string]any{"type": "string", "enum": []string{"asc", "desc"}},
	}

	sortFields := []string{"created_at"}
	filterProps := map[string]any{}
	for _, f := range def.Fields {
		label := f.Label
		if label == "" {
			label = f.Key
		}
		switch f.Type {
		case domain.FieldText, domain.FieldPhone, domain.FieldEmail:
			filterProps[f.Key] = map[string]any{"type": "string", "description": label + " (exact match)"}
		case domain.FieldEnum:
			if f.Key == def.StatusField {
				continue // covered by the top-level status property
			}
			p := map[string]any{"type": "string", "description": label, domain.SchemaKeyUnit: string(domain.UnitEnumValueSet)}
			if vs, ok := sets[f.ValueSetRef]; ok {
				values := make([]string, len(vs.Values))
				for i, entry := range vs.Values {
					values[i] = entry.Value
				}
				p["enum"] = values
			}
			filterProps[f.Key] = p
		case domain.FieldNumber:
			minP := map[string]any{"type": "number", "description": "Minimum " + label}
			maxP := map[string]any{"type": "number", "description": "Maximum " + label}
			if f.Unit != "" {
				minP[domain.SchemaKeyUnit] = string(f.Unit)
				maxP[domain.SchemaKeyUnit] = string(f.Unit)
			}
			filterProps["min_"+f.Key] = minP
			filterProps["max_"+f.Key] = maxP
			sortFields = append(sortFields, f.Key)
		case domain.FieldMoney:
			filterProps["min_"+f.Key] = map[string]any{
				"type": "number", "description": "Minimum " + label + " in USD (dollars)",
				domain.SchemaKeyUnit: string(domain.UnitUSD),
			}
			filterProps["max_"+f.Key] = map[string]any{
				"type": "number", "description": "Maximum " + label + " in USD (dollars)",
				domain.SchemaKeyUnit: string(domain.UnitUSD),
			}
			sortFields = append(sortFields, f.Key)
		case domain.FieldRef:
			if f.RefTarget == string(domain.EntityTypeProduct) {
				props["ref_id"] = map[string]any{
					"type":               "string",
					"description":        fmt.Sprintf("Only records linked to this %s id (%s).", f.RefTarget, label),
					domain.SchemaKeyUnit: string(domain.UnitIDRef),
				}
			}
		}
	}
	if len(filterProps) > 0 {
		props["filters"] = map[string]any{
			"type":        "object",
			"description": "Exact and range filters over the entity's fields. Only include filters you're confident about.",
			"properties":  filterProps,
		}
	}
	props["sort_by"] = map[string]any{"type": "string", "enum": sortFields}

	if def.StatusField != "" {
		statusProp := map[string]any{
			"type":               "string",
			"description":        "Filter by status.",
			domain.SchemaKeyUnit: string(domain.UnitEnumValueSet),
		}
		for _, f := range def.Fields {
			if f.Key == def.StatusField && f.ValueSetRef != "" {
				if vs, ok := sets[f.ValueSetRef]; ok {
					values := make([]string, len(vs.Values))
					for i, entry := range vs.Values {
						values[i] = entry.Value
					}
					statusProp["enum"] = values
				}
			}
		}
		props["status"] = statusProp
	}

	return map[string]any{"type": "object", "properties": props}
}
