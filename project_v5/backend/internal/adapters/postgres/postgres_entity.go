package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine"
	"keepstar_v5/internal/ports"
)

// EntityAdapter implements ports.EntityPort — the SOLE write surface for
// public.v5_entity_* / v5_value_sets / v5_automations / v5_events
// (RUNTIME_SPEC.md §4.4, rulings R10/R12/R27/R28/R29). Record mutations run
// in ONE tx with their v5_events outbox row (the zoneWriteWithDelta
// hygiene, postgres_state.go); the created event is returned for
// post-commit inline dispatch — there is no sweeper in v1 (R12).
//
// Guardrails enforced here, not in prose (§4.4): closed 10-type field set;
// camelCase field keys through the engine.SnakeToCamel vocabulary (§6.2);
// definitions additive-only once records exist (type change / field removal
// / slug rename rejected); no delete API; soft caps 20 defs/tenant,
// 50 fields/def, 50 values/set; automations = 2 event types, 1 flat
// predicate, notify only.
type EntityAdapter struct {
	client *Client
}

// NewEntityAdapter constructs the entity-plane adapter.
func NewEntityAdapter(client *Client) *EntityAdapter {
	return &EntityAdapter{client: client}
}

var _ ports.EntityPort = (*EntityAdapter)(nil)

// Not-found / invalid sentinels. Instances of domain.Error so usecases can
// match by errors.Is against these exported values or by Code via
// errors.As(&domain.Error) without importing this package.
// TODO(integrator): consider relocating to domain/errors.go (contracts file,
// owned by the contracts phase) alongside ErrPresetNotFound.
var (
	ErrEntityDefinitionNotFound = &domain.Error{Code: "ENTITY_NOT_FOUND", Message: "entity definition not found"}
	ErrValueSetNotFound         = &domain.Error{Code: "VALUE_SET_NOT_FOUND", Message: "value set not found"}
	ErrRecordNotFound           = &domain.Error{Code: "RECORD_NOT_FOUND", Message: "entity record not found"}
)

// fieldKeyRe is the §6.2 binding-vocabulary gate: camelCase, letter-first.
// Paired with the SnakeToCamel no-op check (SnakeToCamel is THE normalizer).
var fieldKeyRe = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`)

// entitySlugRe gates entity / value-set slugs ("lead", "lead_pipeline") —
// interpolation-safe lowercase identifiers, ≤100 chars (VARCHAR(100)).
var entitySlugRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,99}$`)

var validFieldTypes = map[domain.FieldType]bool{
	domain.FieldText: true, domain.FieldNumber: true, domain.FieldMoney: true,
	domain.FieldBool: true, domain.FieldDate: true, domain.FieldDatetime: true,
	domain.FieldEnum: true, domain.FieldPhone: true, domain.FieldEmail: true,
	domain.FieldRef: true,
}

// automationEventTypes is the closed v1 automation trigger set (§4.4
// guardrails — record.updated exists as an event but does not trigger).
var automationEventTypes = map[string]bool{
	domain.EventRecordCreated:       true,
	domain.EventRecordStatusChanged: true,
}

// ─── entity definitions ──────────────────────────────────────────────────

const entityDefinitionSelect = `
	SELECT id::text, tenant_id::text, slug, name, COALESCE(name_plural, ''),
	       fields, display, COALESCE(status_field, ''), created_at, updated_at
	FROM v5_entity_definitions
`

// UpsertEntityDefinition creates or updates one definition. Natural key =
// (tenant_id, slug); a def with ID set updates that row (the only path a
// slug rename can travel). Additive-only once records exist: field removal,
// type change and slug rename are rejected. Idempotent — the manifest
// applier's retry contract (§4.3).
func (a *EntityAdapter) UpsertEntityDefinition(ctx context.Context, def *domain.EntityDefinition) error {
	if err := validateDefinition(def); err != nil {
		return err
	}
	fieldsJSON, err := json.Marshal(def.Fields)
	if err != nil {
		return fmt.Errorf("upsert entity %s: fields: %w", def.Slug, err)
	}
	displayJSON, err := json.Marshal(def.Display)
	if err != nil {
		return fmt.Errorf("upsert entity %s: display: %w", def.Slug, err)
	}

	var (
		existingID   string
		existingSlug string
		existingRaw  []byte
	)
	switch {
	case def.ID != "":
		err = a.client.pool.QueryRow(ctx, `
			SELECT id::text, slug, fields FROM v5_entity_definitions
			WHERE id = $1::uuid AND tenant_id = $2::uuid
		`, def.ID, def.TenantID).Scan(&existingID, &existingSlug, &existingRaw)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrEntityDefinitionNotFound
		}
	default:
		err = a.client.pool.QueryRow(ctx, `
			SELECT id::text, slug, fields FROM v5_entity_definitions
			WHERE tenant_id = $1::uuid AND slug = $2
		`, def.TenantID, def.Slug).Scan(&existingID, &existingSlug, &existingRaw)
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil // fresh insert below
		}
	}
	if err != nil {
		return fmt.Errorf("upsert entity %s: lookup: %w", def.Slug, err)
	}

	if existingID == "" {
		var count int
		if err := a.client.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM v5_entity_definitions WHERE tenant_id = $1::uuid
		`, def.TenantID).Scan(&count); err != nil {
			return fmt.Errorf("upsert entity %s: count: %w", def.Slug, err)
		}
		if count >= domain.MaxEntityDefinitionsPerTenant {
			return &domain.Error{Code: "ENTITY_CAP", Message: fmt.Sprintf(
				"tenant already has %d entity definitions (cap %d)", count, domain.MaxEntityDefinitionsPerTenant)}
		}
		err := a.client.pool.QueryRow(ctx, `
			INSERT INTO v5_entity_definitions
				(tenant_id, slug, name, name_plural, fields, display, status_field)
			VALUES ($1::uuid, $2, $3, NULLIF($4, ''), $5, $6, NULLIF($7, ''))
			RETURNING id::text, created_at, updated_at
		`, def.TenantID, def.Slug, def.Name, def.NamePlural, fieldsJSON, displayJSON,
			def.StatusField).Scan(&def.ID, &def.CreatedAt, &def.UpdatedAt)
		if err != nil {
			return fmt.Errorf("upsert entity %s: insert: %w", def.Slug, err)
		}
		return nil
	}

	var recordsExist bool
	if err := a.client.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM v5_entity_records WHERE entity_definition_id = $1::uuid)
	`, existingID).Scan(&recordsExist); err != nil {
		return fmt.Errorf("upsert entity %s: records check: %w", def.Slug, err)
	}
	if recordsExist {
		if existingSlug != def.Slug {
			return &domain.Error{Code: "ENTITY_NOT_ADDITIVE", Message: fmt.Sprintf(
				"entity %q: slug rename to %q rejected once records exist", existingSlug, def.Slug)}
		}
		var existingFields []domain.FieldDef
		if err := json.Unmarshal(existingRaw, &existingFields); err != nil {
			return fmt.Errorf("upsert entity %s: existing fields: %w", def.Slug, err)
		}
		if err := checkAdditiveFields(existingSlug, existingFields, def.Fields); err != nil {
			return err
		}
	}
	err = a.client.pool.QueryRow(ctx, `
		UPDATE v5_entity_definitions
		SET slug = $2, name = $3, name_plural = NULLIF($4, ''), fields = $5,
		    display = $6, status_field = NULLIF($7, ''), updated_at = NOW()
		WHERE id = $1::uuid
		RETURNING created_at, updated_at
	`, existingID, def.Slug, def.Name, def.NamePlural, fieldsJSON, displayJSON,
		def.StatusField).Scan(&def.CreatedAt, &def.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert entity %s: update: %w", def.Slug, err)
	}
	def.ID = existingID
	return nil
}

func (a *EntityAdapter) GetEntityDefinition(ctx context.Context, tenantID, slug string) (*domain.EntityDefinition, error) {
	row := a.client.pool.QueryRow(ctx, entityDefinitionSelect+`
		WHERE tenant_id = $1::uuid AND slug = $2
	`, tenantID, slug)
	def, err := scanEntityDefinition(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEntityDefinitionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get entity %s: %w", slug, err)
	}
	return def, nil
}

func (a *EntityAdapter) ListEntityDefinitions(ctx context.Context, tenantID string) ([]domain.EntityDefinition, error) {
	rows, err := a.client.pool.Query(ctx, entityDefinitionSelect+`
		WHERE tenant_id = $1::uuid ORDER BY slug
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list entities: %w", err)
	}
	defer rows.Close()

	var out []domain.EntityDefinition
	for rows.Next() {
		def, err := scanEntityDefinition(rows)
		if err != nil {
			return nil, fmt.Errorf("scan entity definition: %w", err)
		}
		out = append(out, *def)
	}
	return out, rows.Err()
}

// validateDefinition enforces the §4.4 definition guardrails (closed field
// types, §6.2 camelCase keys, enum/ref pairing, caps).
func validateDefinition(def *domain.EntityDefinition) error {
	if def == nil {
		return &domain.Error{Code: "ENTITY_INVALID", Message: "nil entity definition"}
	}
	if def.TenantID == "" {
		return &domain.Error{Code: "ENTITY_INVALID", Message: "entity definition missing tenant id"}
	}
	if !entitySlugRe.MatchString(def.Slug) {
		return &domain.Error{Code: "ENTITY_INVALID", Message: fmt.Sprintf(
			"entity slug %q must match ^[a-z][a-z0-9_]*$ (max 100 chars)", def.Slug)}
	}
	if def.Name == "" {
		return &domain.Error{Code: "ENTITY_INVALID", Message: fmt.Sprintf("entity %q missing name", def.Slug)}
	}
	if len(def.Fields) == 0 {
		return &domain.Error{Code: "ENTITY_INVALID", Message: fmt.Sprintf("entity %q has no fields", def.Slug)}
	}
	if len(def.Fields) > domain.MaxFieldsPerEntity {
		return &domain.Error{Code: "ENTITY_INVALID", Message: fmt.Sprintf(
			"entity %q has %d fields (cap %d)", def.Slug, len(def.Fields), domain.MaxFieldsPerEntity)}
	}
	seen := make(map[string]bool, len(def.Fields))
	for _, f := range def.Fields {
		if !fieldKeyRe.MatchString(f.Key) || engine.SnakeToCamel(f.Key) != f.Key {
			return &domain.Error{Code: "ENTITY_INVALID", Message: fmt.Sprintf(
				"field key %q must be camelCase (^[a-z][a-zA-Z0-9]*$)", f.Key)}
		}
		if seen[f.Key] {
			return &domain.Error{Code: "ENTITY_INVALID", Message: fmt.Sprintf("duplicate field key %q", f.Key)}
		}
		seen[f.Key] = true
		if !validFieldTypes[f.Type] {
			return &domain.Error{Code: "ENTITY_INVALID", Message: fmt.Sprintf(
				"field %q: unknown type %q (closed 10-type set)", f.Key, f.Type)}
		}
		if f.Unit != "" {
			// Unit is number-only; money/date/datetime imply theirs (R18).
			if f.Type != domain.FieldNumber {
				return &domain.Error{Code: "ENTITY_INVALID", Message: fmt.Sprintf(
					"field %q: unit only allowed on number fields, got type %q", f.Key, f.Type)}
			}
			if !f.Unit.Valid() {
				return &domain.Error{Code: "ENTITY_INVALID", Message: fmt.Sprintf(
					"field %q: unknown unit %q (closed R18 vocabulary)", f.Key, f.Unit)}
			}
		}
		if (f.Type == domain.FieldEnum) != (f.ValueSetRef != "") {
			return &domain.Error{Code: "ENTITY_INVALID", Message: fmt.Sprintf(
				"field %q: valueSetRef is required for enum fields and only for them", f.Key)}
		}
		if (f.Type == domain.FieldRef) != (f.RefTarget != "") {
			return &domain.Error{Code: "ENTITY_INVALID", Message: fmt.Sprintf(
				"field %q: refTarget is required for ref fields and only for them", f.Key)}
		}
	}
	if def.StatusField != "" && !seen[def.StatusField] {
		return &domain.Error{Code: "ENTITY_INVALID", Message: fmt.Sprintf(
			"status field %q is not a declared field", def.StatusField)}
	}
	return nil
}

// checkAdditiveFields rejects field removal and type changes against the
// stored definition — the additive-only guardrail once records exist.
func checkAdditiveFields(slug string, old, new []domain.FieldDef) error {
	byKey := make(map[string]domain.FieldType, len(new))
	for _, f := range new {
		byKey[f.Key] = f.Type
	}
	for _, f := range old {
		newType, ok := byKey[f.Key]
		if !ok {
			return &domain.Error{Code: "ENTITY_NOT_ADDITIVE", Message: fmt.Sprintf(
				"entity %q: field %q removed — additive-only once records exist", slug, f.Key)}
		}
		if newType != f.Type {
			return &domain.Error{Code: "ENTITY_NOT_ADDITIVE", Message: fmt.Sprintf(
				"entity %q: field %q type change %s→%s — additive-only once records exist",
				slug, f.Key, f.Type, newType)}
		}
	}
	return nil
}

func scanEntityDefinition(row pgx.Row) (*domain.EntityDefinition, error) {
	var (
		def        domain.EntityDefinition
		fieldsRaw  []byte
		displayRaw []byte
	)
	if err := row.Scan(&def.ID, &def.TenantID, &def.Slug, &def.Name, &def.NamePlural,
		&fieldsRaw, &displayRaw, &def.StatusField, &def.CreatedAt, &def.UpdatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(fieldsRaw, &def.Fields); err != nil {
		return nil, fmt.Errorf("fields: %w", err)
	}
	if len(displayRaw) > 0 {
		if err := json.Unmarshal(displayRaw, &def.Display); err != nil {
			return nil, fmt.Errorf("display: %w", err)
		}
	}
	return &def, nil
}

// ─── value sets ──────────────────────────────────────────────────────────

// UpsertValueSet idempotently upserts one v5_value_sets row on
// (tenant_id, slug). Entries stay in array order (R27).
func (a *EntityAdapter) UpsertValueSet(ctx context.Context, vs *domain.ValueSet) error {
	if err := validateValueSet(vs); err != nil {
		return err
	}
	valuesJSON, err := json.Marshal(vs.Values)
	if err != nil {
		return fmt.Errorf("upsert value set %s: values: %w", vs.Slug, err)
	}
	err = a.client.pool.QueryRow(ctx, `
		INSERT INTO v5_value_sets (tenant_id, slug, name, "values")
		VALUES ($1::uuid, $2, $3, $4)
		ON CONFLICT (tenant_id, slug) DO UPDATE SET
			name       = EXCLUDED.name,
			"values"   = EXCLUDED."values",
			updated_at = NOW()
		RETURNING id::text, created_at, updated_at
	`, vs.TenantID, vs.Slug, vs.Name, valuesJSON).Scan(&vs.ID, &vs.CreatedAt, &vs.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert value set %s: %w", vs.Slug, err)
	}
	return nil
}

func (a *EntityAdapter) GetValueSet(ctx context.Context, tenantID, slug string) (*domain.ValueSet, error) {
	row := a.client.pool.QueryRow(ctx, `
		SELECT id::text, tenant_id::text, slug, name, "values", created_at, updated_at
		FROM v5_value_sets
		WHERE tenant_id = $1::uuid AND slug = $2
	`, tenantID, slug)
	vs, err := scanValueSet(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrValueSetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get value set %s: %w", slug, err)
	}
	return vs, nil
}

func (a *EntityAdapter) ListValueSets(ctx context.Context, tenantID string) ([]domain.ValueSet, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT id::text, tenant_id::text, slug, name, "values", created_at, updated_at
		FROM v5_value_sets
		WHERE tenant_id = $1::uuid ORDER BY slug
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list value sets: %w", err)
	}
	defer rows.Close()

	var out []domain.ValueSet
	for rows.Next() {
		vs, err := scanValueSet(rows)
		if err != nil {
			return nil, fmt.Errorf("scan value set: %w", err)
		}
		out = append(out, *vs)
	}
	return out, rows.Err()
}

func validateValueSet(vs *domain.ValueSet) error {
	if vs == nil {
		return &domain.Error{Code: "VALUE_SET_INVALID", Message: "nil value set"}
	}
	if vs.TenantID == "" {
		return &domain.Error{Code: "VALUE_SET_INVALID", Message: "value set missing tenant id"}
	}
	if !entitySlugRe.MatchString(vs.Slug) {
		return &domain.Error{Code: "VALUE_SET_INVALID", Message: fmt.Sprintf(
			"value set slug %q must match ^[a-z][a-z0-9_]*$ (max 100 chars)", vs.Slug)}
	}
	if vs.Name == "" {
		return &domain.Error{Code: "VALUE_SET_INVALID", Message: fmt.Sprintf("value set %q missing name", vs.Slug)}
	}
	if len(vs.Values) == 0 {
		return &domain.Error{Code: "VALUE_SET_INVALID", Message: fmt.Sprintf("value set %q has no values", vs.Slug)}
	}
	if len(vs.Values) > domain.MaxValuesPerValueSet {
		return &domain.Error{Code: "VALUE_SET_INVALID", Message: fmt.Sprintf(
			"value set %q has %d values (cap %d)", vs.Slug, len(vs.Values), domain.MaxValuesPerValueSet)}
	}
	seen := make(map[string]bool, len(vs.Values))
	for _, v := range vs.Values {
		if v.Value == "" {
			return &domain.Error{Code: "VALUE_SET_INVALID", Message: fmt.Sprintf("value set %q has an empty value", vs.Slug)}
		}
		if seen[v.Value] {
			return &domain.Error{Code: "VALUE_SET_INVALID", Message: fmt.Sprintf(
				"value set %q has duplicate value %q", vs.Slug, v.Value)}
		}
		seen[v.Value] = true
	}
	return nil
}

func scanValueSet(row pgx.Row) (*domain.ValueSet, error) {
	var (
		vs        domain.ValueSet
		valuesRaw []byte
	)
	if err := row.Scan(&vs.ID, &vs.TenantID, &vs.Slug, &vs.Name, &valuesRaw,
		&vs.CreatedAt, &vs.UpdatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(valuesRaw, &vs.Values); err != nil {
		return nil, fmt.Errorf("values: %w", err)
	}
	return &vs, nil
}

// ─── records (tx: record write + v5_events outbox row) ──────────────────

const entityRecordSelect = `
	SELECT id::text, tenant_id::text, entity_definition_id::text, entity_slug, data,
	       COALESCE(status, ''), COALESCE(ref_entity_type, ''), COALESCE(ref_entity_id::text, ''),
	       COALESCE(created_by, ''), created_at, updated_at
	FROM v5_entity_records
`

// CreateRecord inserts one record and its record.created outbox event in a
// single READ COMMITTED tx. Data is validated UPSTREAM by
// usecases.EntityWrite (ValidateRecordData) — here we resolve the
// definition, keep the status mirror true and guarantee record+event
// atomicity. The returned event is dispatched inline post-commit by the
// caller (R12).
func (a *EntityAdapter) CreateRecord(ctx context.Context, rec *domain.EntityRecord) (*domain.EntityRecord, *domain.RuntimeEvent, error) {
	if rec == nil || rec.TenantID == "" || rec.EntitySlug == "" {
		return nil, nil, &domain.Error{Code: "RECORD_INVALID", Message: "record missing tenant id or entity slug"}
	}
	if rec.Data == nil {
		rec.Data = map[string]any{}
	}
	def, err := a.GetEntityDefinition(ctx, rec.TenantID, rec.EntitySlug)
	if err != nil {
		return nil, nil, err
	}
	if rec.Status == "" && def.StatusField != "" {
		if s, ok := rec.Data[def.StatusField].(string); ok {
			rec.Status = s // keep the hot mirror true regardless of caller discipline
		}
	}
	dataJSON, err := jsonbMap(rec.Data)
	if err != nil {
		return nil, nil, fmt.Errorf("create record %s: data: %w", rec.EntitySlug, err)
	}

	tx, err := a.client.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, nil, fmt.Errorf("create record %s: begin tx: %w", rec.EntitySlug, err)
	}
	defer tx.Rollback(ctx) // no-op after Commit

	err = tx.QueryRow(ctx, `
		INSERT INTO v5_entity_records
			(tenant_id, entity_definition_id, entity_slug, data, status,
			 ref_entity_type, ref_entity_id, created_by)
		VALUES ($1::uuid, $2::uuid, $3, $4, NULLIF($5, ''),
		        NULLIF($6, ''), NULLIF($7, '')::uuid, NULLIF($8, ''))
		RETURNING id::text, created_at, updated_at
	`, rec.TenantID, def.ID, rec.EntitySlug, dataJSON, rec.Status,
		rec.RefEntityType, rec.RefEntityID, rec.CreatedBy).Scan(&rec.ID, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		return nil, nil, fmt.Errorf("create record %s: insert: %w", rec.EntitySlug, err)
	}
	rec.EntityDefinitionID = def.ID

	ev, err := insertEvent(ctx, tx, rec.TenantID, domain.EventRecordCreated, rec.EntitySlug, rec.ID,
		map[string]any{"actor": rec.CreatedBy, "snapshot": rec.Data})
	if err != nil {
		return nil, nil, fmt.Errorf("create record %s: event: %w", rec.EntitySlug, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("create record %s: commit: %w", rec.EntitySlug, err)
	}
	return rec, ev, nil
}

// UpdateRecord shallow-merges patch into data (last-write-wins guardrail),
// re-mirrors the status column from data[status_field], and writes the
// record.updated outbox event in the same tx. Payload carries
// {actor, diff:{before,after}} — full data snapshots, so automation
// predicates and notify templates can read any field.
func (a *EntityAdapter) UpdateRecord(ctx context.Context, tenantID, recordID string, patch map[string]any) (*domain.EntityRecord, *domain.RuntimeEvent, error) {
	if len(patch) == 0 {
		return nil, nil, &domain.Error{Code: "RECORD_INVALID", Message: "empty patch"}
	}
	if !isUUID(recordID) {
		return nil, nil, ErrRecordNotFound
	}

	tx, err := a.client.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, nil, fmt.Errorf("update record: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	rec, statusField, _, err := lockRecord(ctx, tx, tenantID, recordID)
	if err != nil {
		return nil, nil, err
	}

	before := copyDataMap(rec.Data)
	merged := copyDataMap(rec.Data)
	for k, v := range patch {
		merged[k] = v
	}
	newStatus := rec.Status
	if statusField != "" {
		if s, ok := merged[statusField].(string); ok {
			newStatus = s
		}
	}
	mergedJSON, err := jsonbMap(merged)
	if err != nil {
		return nil, nil, fmt.Errorf("update record %s: data: %w", recordID, err)
	}
	if err := tx.QueryRow(ctx, `
		UPDATE v5_entity_records SET data = $3, status = NULLIF($4, ''), updated_at = NOW()
		WHERE tenant_id = $1::uuid AND id = $2::uuid
		RETURNING updated_at
	`, tenantID, recordID, mergedJSON, newStatus).Scan(&rec.UpdatedAt); err != nil {
		return nil, nil, fmt.Errorf("update record %s: %w", recordID, err)
	}
	rec.Data = merged
	rec.Status = newStatus

	ev, err := insertEvent(ctx, tx, tenantID, domain.EventRecordUpdated, rec.EntitySlug, rec.ID,
		map[string]any{"actor": rec.CreatedBy, "diff": map[string]any{"before": before, "after": merged}})
	if err != nil {
		return nil, nil, fmt.Errorf("update record %s: event: %w", recordID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("update record %s: commit: %w", recordID, err)
	}
	return rec, ev, nil
}

// TransitionStatus moves a record to toStatus with value-set validation:
// when the definition's status field is enum-backed, toStatus must be a
// member of its value set (violation → typed INVALID_STATUS error carrying
// the allowed values, so the LLM can self-correct). Status column and
// data[status_field] move together with the record.status_changed outbox
// event, all in one tx.
func (a *EntityAdapter) TransitionStatus(ctx context.Context, tenantID, recordID, toStatus string) (*domain.EntityRecord, *domain.RuntimeEvent, error) {
	if toStatus == "" {
		return nil, nil, &domain.Error{Code: "INVALID_STATUS", Message: "empty target status"}
	}
	if !isUUID(recordID) {
		return nil, nil, ErrRecordNotFound
	}

	tx, err := a.client.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, nil, fmt.Errorf("transition record: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	rec, statusField, fieldsRaw, err := lockRecord(ctx, tx, tenantID, recordID)
	if err != nil {
		return nil, nil, err
	}
	if statusField == "" {
		return nil, nil, &domain.Error{Code: "NO_STATUS_FIELD", Message: fmt.Sprintf(
			"entity %q has no status field — nothing to transition", rec.EntitySlug)}
	}
	var fields []domain.FieldDef
	if err := json.Unmarshal(fieldsRaw, &fields); err != nil {
		return nil, nil, fmt.Errorf("transition record %s: fields: %w", recordID, err)
	}
	valueSetRef := ""
	for _, f := range fields {
		if f.Key == statusField {
			valueSetRef = f.ValueSetRef
			break
		}
	}
	if valueSetRef != "" {
		var valuesRaw []byte
		err := tx.QueryRow(ctx, `
			SELECT "values" FROM v5_value_sets WHERE tenant_id = $1::uuid AND slug = $2
		`, tenantID, valueSetRef).Scan(&valuesRaw)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrValueSetNotFound
		}
		if err != nil {
			return nil, nil, fmt.Errorf("transition record %s: value set %s: %w", recordID, valueSetRef, err)
		}
		var entries []domain.ValueSetEntry
		if err := json.Unmarshal(valuesRaw, &entries); err != nil {
			return nil, nil, fmt.Errorf("transition record %s: value set %s: %w", recordID, valueSetRef, err)
		}
		allowed := make([]string, len(entries))
		member := false
		for i, e := range entries {
			allowed[i] = e.Value
			if e.Value == toStatus {
				member = true
			}
		}
		if !member {
			return nil, nil, &domain.Error{Code: "INVALID_STATUS", Message: fmt.Sprintf(
				"status %q is not in value set %s (allowed: %s)", toStatus, valueSetRef, strings.Join(allowed, ", "))}
		}
	}

	before := copyDataMap(rec.Data)
	merged := copyDataMap(rec.Data)
	merged[statusField] = toStatus
	mergedJSON, err := jsonbMap(merged)
	if err != nil {
		return nil, nil, fmt.Errorf("transition record %s: data: %w", recordID, err)
	}
	if err := tx.QueryRow(ctx, `
		UPDATE v5_entity_records SET data = $3, status = $4, updated_at = NOW()
		WHERE tenant_id = $1::uuid AND id = $2::uuid
		RETURNING updated_at
	`, tenantID, recordID, mergedJSON, toStatus).Scan(&rec.UpdatedAt); err != nil {
		return nil, nil, fmt.Errorf("transition record %s: %w", recordID, err)
	}
	rec.Data = merged
	rec.Status = toStatus

	ev, err := insertEvent(ctx, tx, tenantID, domain.EventRecordStatusChanged, rec.EntitySlug, rec.ID,
		map[string]any{"actor": rec.CreatedBy, "diff": map[string]any{"before": before, "after": merged}})
	if err != nil {
		return nil, nil, fmt.Errorf("transition record %s: event: %w", recordID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("transition record %s: commit: %w", recordID, err)
	}
	return rec, ev, nil
}

func (a *EntityAdapter) GetRecord(ctx context.Context, tenantID, recordID string) (*domain.EntityRecord, error) {
	if !isUUID(recordID) {
		return nil, ErrRecordNotFound
	}
	row := a.client.pool.QueryRow(ctx, entityRecordSelect+`
		WHERE tenant_id = $1::uuid AND id = $2::uuid
	`, tenantID, recordID)
	rec, err := scanEntityRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get record %s: %w", recordID, err)
	}
	return rec, nil
}

// QueryRecords is the flat filtered read (no aggregation/group-by/FTS/
// vector — guardrail §4.4). Generic attr predicates follow the
// catalogFilterConditions discipline: keys are normalized through
// engine.SnakeToCamel, identifier-gated before interpolation, values always
// travel as parameters, numeric bounds are jsonb_typeof-guarded.
func (a *EntityAdapter) QueryRecords(ctx context.Context, tenantID, entitySlug string, f domain.RecordFilter) (records []domain.EntityRecord, total int, err error) {
	if f.RefEntityID != "" && !isUUID(f.RefEntityID) {
		return nil, 0, &domain.Error{Code: "INVALID_FILTER", Message: fmt.Sprintf(
			"refEntityId %q must be a UUID", f.RefEntityID)}
	}
	args := []interface{}{tenantID, entitySlug}
	argNum := 3
	conditions, args, argNum := entityFilterConditions(f, args, argNum)

	where := ` WHERE tenant_id = $1::uuid AND entity_slug = $2`
	if len(conditions) > 0 {
		where += " AND " + strings.Join(conditions, " AND ")
	}

	countQuery := `SELECT COUNT(*) FROM v5_entity_records` + where
	if err := a.client.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("query records %s: count: %w", entitySlug, err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	query := entityRecordSelect + where +
		` ORDER BY ` + entitySortClause(f.SortField, f.SortOrder) +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, argNum, argNum+1)
	args = append(args, limit, offset)

	rows, err := a.client.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query records %s: %w", entitySlug, err)
	}
	defer rows.Close()

	for rows.Next() {
		rec, err := scanEntityRecord(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan record: %w", err)
		}
		records = append(records, *rec)
	}
	return records, total, rows.Err()
}

// entityFilterConditions translates a RecordFilter into SQL predicates over
// v5_entity_records — the catalogFilterConditions discipline applied to the
// entity plane (data keys are camelCase by construction, so incoming keys
// are normalized once via engine.SnakeToCamel instead of probing twins).
func entityFilterConditions(f domain.RecordFilter, args []interface{}, argNum int) ([]string, []interface{}, int) {
	var conditions []string
	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argNum))
		args = append(args, f.Status)
		argNum++
	}
	if f.Since != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argNum))
		args = append(args, *f.Since)
		argNum++
	}
	if f.RefEntityID != "" {
		conditions = append(conditions, fmt.Sprintf("ref_entity_id = $%d::uuid", argNum))
		args = append(args, f.RefEntityID)
		argNum++
	}
	for _, k := range sortedAttrKeys(f.Attrs) {
		v := f.Attrs[k]
		key := engine.SnakeToCamel(k)
		if v == "" || !safeTier2Key(key) {
			continue
		}
		conditions = append(conditions, fmt.Sprintf(
			"(data->>'%[1]s' = $%[2]d OR (jsonb_typeof(data->'%[1]s') = 'array' AND data->'%[1]s' @> to_jsonb($%[2]d::text)))",
			key, argNum))
		args = append(args, v)
		argNum++
	}
	for _, k := range sortedNumKeys(f.AttrMin) {
		key := engine.SnakeToCamel(k)
		if !safeTier2Key(key) {
			continue
		}
		conditions = append(conditions, fmt.Sprintf(
			"(jsonb_typeof(data->'%[1]s') = 'number' AND (data->>'%[1]s')::numeric >= $%[2]d)", key, argNum))
		args = append(args, f.AttrMin[k])
		argNum++
	}
	for _, k := range sortedNumKeys(f.AttrMax) {
		key := engine.SnakeToCamel(k)
		if !safeTier2Key(key) {
			continue
		}
		conditions = append(conditions, fmt.Sprintf(
			"(jsonb_typeof(data->'%[1]s') = 'number' AND (data->>'%[1]s')::numeric <= $%[2]d)", key, argNum))
		args = append(args, f.AttrMax[k])
		argNum++
	}
	return conditions, args, argNum
}

// entitySortClause maps a RecordFilter sort onto a safe ORDER BY: known
// columns by name, any other identifier-gated key sorts on data->>'key'
// (text order). id is the deterministic tiebreak. Unknown/unsafe input
// falls back to the created_at DESC index order. The field normalizes
// through SnakeToCamel first so the query schema's snake_case sort enum
// ("created_at" — entityQuerySchema sortFields) and camelCase data keys
// address the same columns.
func entitySortClause(field, order string) string {
	dir := "DESC"
	if strings.EqualFold(order, "asc") {
		dir = "ASC"
	}
	col := "created_at"
	key := engine.SnakeToCamel(field)
	switch key {
	case "", "createdAt":
		// default
	case "updatedAt":
		col = "updated_at"
	case "status":
		col = "status"
	default:
		if safeTier2Key(key) {
			col = fmt.Sprintf("data->>'%s'", key)
		}
	}
	return fmt.Sprintf("%s %s, id", col, dir)
}

// lockRecord loads one record FOR UPDATE together with its definition's
// status_field and fields — the shared head of the update/transition txs.
func lockRecord(ctx context.Context, tx pgx.Tx, tenantID, recordID string) (*domain.EntityRecord, string, []byte, error) {
	var (
		rec         domain.EntityRecord
		dataRaw     []byte
		statusField string
		fieldsRaw   []byte
	)
	err := tx.QueryRow(ctx, `
		SELECT r.id::text, r.tenant_id::text, r.entity_definition_id::text, r.entity_slug,
		       r.data, COALESCE(r.status, ''), COALESCE(r.ref_entity_type, ''),
		       COALESCE(r.ref_entity_id::text, ''), COALESCE(r.created_by, ''),
		       r.created_at, r.updated_at, COALESCE(d.status_field, ''), d.fields
		FROM v5_entity_records r
		JOIN v5_entity_definitions d ON d.id = r.entity_definition_id
		WHERE r.tenant_id = $1::uuid AND r.id = $2::uuid
		FOR UPDATE OF r
	`, tenantID, recordID).Scan(&rec.ID, &rec.TenantID, &rec.EntityDefinitionID, &rec.EntitySlug,
		&dataRaw, &rec.Status, &rec.RefEntityType, &rec.RefEntityID, &rec.CreatedBy,
		&rec.CreatedAt, &rec.UpdatedAt, &statusField, &fieldsRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", nil, ErrRecordNotFound
	}
	if err != nil {
		return nil, "", nil, fmt.Errorf("lock record %s: %w", recordID, err)
	}
	rec.Data, err = unmarshalJSONMap(dataRaw)
	if err != nil {
		return nil, "", nil, fmt.Errorf("lock record %s: data: %w", recordID, err)
	}
	if rec.Data == nil {
		rec.Data = map[string]any{}
	}
	return &rec, statusField, fieldsRaw, nil
}

// insertEvent writes one v5_events outbox row inside the caller's tx.
// The event payload actor is the record's created_by — the port contract
// carries no per-call actor in v1; who ran the operation is audited in
// v5_operation_runs at the registry choke point.
func insertEvent(ctx context.Context, tx pgx.Tx, tenantID, eventType, entitySlug, recordID string, payload map[string]any) (*domain.RuntimeEvent, error) {
	payloadJSON, err := jsonbMap(payload)
	if err != nil {
		return nil, fmt.Errorf("event payload: %w", err)
	}
	ev := &domain.RuntimeEvent{
		TenantID:   tenantID,
		EventType:  eventType,
		EntitySlug: entitySlug,
		RecordID:   recordID,
		Payload:    payload,
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO v5_events (tenant_id, event_type, entity_slug, record_id, payload)
		VALUES ($1::uuid, $2, NULLIF($3, ''), NULLIF($4, '')::uuid, $5)
		RETURNING id, occurred_at
	`, tenantID, eventType, entitySlug, recordID, payloadJSON).Scan(&ev.ID, &ev.OccurredAt)
	if err != nil {
		return nil, err
	}
	return ev, nil
}

func scanEntityRecord(row pgx.Row) (*domain.EntityRecord, error) {
	var (
		rec     domain.EntityRecord
		dataRaw []byte
	)
	if err := row.Scan(&rec.ID, &rec.TenantID, &rec.EntityDefinitionID, &rec.EntitySlug,
		&dataRaw, &rec.Status, &rec.RefEntityType, &rec.RefEntityID, &rec.CreatedBy,
		&rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	var err error
	rec.Data, err = unmarshalJSONMap(dataRaw)
	if err != nil {
		return nil, fmt.Errorf("data: %w", err)
	}
	if rec.Data == nil {
		rec.Data = map[string]any{}
	}
	return &rec, nil
}

func copyDataMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// ─── automations ─────────────────────────────────────────────────────────

// UpsertAutomation upserts on the natural key (tenant_id, name) — manual
// UPDATE-then-INSERT because the DDL carries no unique constraint on the
// pair (spec §3.2). Write volume is onboarding-time only; the race window
// is accepted.
func (a *EntityAdapter) UpsertAutomation(ctx context.Context, au *domain.Automation) error {
	if err := validateAutomation(au); err != nil {
		return err
	}
	var predicateJSON interface{}
	if au.Predicate != nil {
		b, err := json.Marshal(au.Predicate)
		if err != nil {
			return fmt.Errorf("upsert automation %s: predicate: %w", au.Name, err)
		}
		predicateJSON = b
	}
	paramsJSON, err := jsonbMap(au.OperationParams)
	if err != nil {
		return fmt.Errorf("upsert automation %s: params: %w", au.Name, err)
	}

	err = a.client.pool.QueryRow(ctx, `
		UPDATE v5_automations
		SET event_type = $3, entity_slug = NULLIF($4, ''), predicate = $5,
		    operation_slug = $6, operation_params = $7, enabled = $8, updated_at = NOW()
		WHERE tenant_id = $1::uuid AND name = $2
		RETURNING id::text, created_at, updated_at
	`, au.TenantID, au.Name, au.EventType, au.EntitySlug, predicateJSON,
		au.OperationSlug, paramsJSON, au.Enabled).Scan(&au.ID, &au.CreatedAt, &au.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = a.client.pool.QueryRow(ctx, `
			INSERT INTO v5_automations
				(tenant_id, name, event_type, entity_slug, predicate,
				 operation_slug, operation_params, enabled)
			VALUES ($1::uuid, $2, $3, NULLIF($4, ''), $5, $6, $7, $8)
			RETURNING id::text, created_at, updated_at
		`, au.TenantID, au.Name, au.EventType, au.EntitySlug, predicateJSON,
			au.OperationSlug, paramsJSON, au.Enabled).Scan(&au.ID, &au.CreatedAt, &au.UpdatedAt)
	}
	if err != nil {
		return fmt.Errorf("upsert automation %s: %w", au.Name, err)
	}
	return nil
}

// ListAutomations returns the tenant's ENABLED automations for one event
// type (eventType "" = all), name-sorted — the deterministic dispatch order
// EntityWrite.DispatchEvent iterates.
func (a *EntityAdapter) ListAutomations(ctx context.Context, tenantID, eventType string) ([]domain.Automation, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT id::text, tenant_id::text, name, event_type, COALESCE(entity_slug, ''),
		       predicate, operation_slug, operation_params, enabled, created_at, updated_at
		FROM v5_automations
		WHERE tenant_id = $1::uuid AND enabled AND ($2 = '' OR event_type = $2)
		ORDER BY name
	`, tenantID, eventType)
	if err != nil {
		return nil, fmt.Errorf("list automations: %w", err)
	}
	defer rows.Close()

	var out []domain.Automation
	for rows.Next() {
		var (
			au           domain.Automation
			predicateRaw []byte
			paramsRaw    []byte
		)
		if err := rows.Scan(&au.ID, &au.TenantID, &au.Name, &au.EventType, &au.EntitySlug,
			&predicateRaw, &au.OperationSlug, &paramsRaw, &au.Enabled,
			&au.CreatedAt, &au.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan automation: %w", err)
		}
		if len(predicateRaw) > 0 {
			var p domain.AutomationPredicate
			if err := json.Unmarshal(predicateRaw, &p); err != nil {
				return nil, fmt.Errorf("scan automation %s: predicate: %w", au.Name, err)
			}
			au.Predicate = &p
		}
		if au.OperationParams, err = unmarshalJSONMap(paramsRaw); err != nil {
			return nil, fmt.Errorf("scan automation %s: params: %w", au.Name, err)
		}
		out = append(out, au)
	}
	return out, rows.Err()
}

func validateAutomation(au *domain.Automation) error {
	if au == nil {
		return &domain.Error{Code: "AUTOMATION_INVALID", Message: "nil automation"}
	}
	if au.TenantID == "" {
		return &domain.Error{Code: "AUTOMATION_INVALID", Message: "automation missing tenant id"}
	}
	if au.Name == "" {
		return &domain.Error{Code: "AUTOMATION_INVALID", Message: "automation missing name"}
	}
	if !automationEventTypes[au.EventType] {
		return &domain.Error{Code: "AUTOMATION_INVALID", Message: fmt.Sprintf(
			"automation %q: event type %q not allowed (record.created | record.status_changed)", au.Name, au.EventType)}
	}
	if au.OperationSlug != "notify" {
		// v1 automation-facing allowlist (§4.2) — enforced at write time too,
		// so the table never holds an operation the runner would refuse.
		return &domain.Error{Code: "AUTOMATION_INVALID", Message: fmt.Sprintf(
			"automation %q: operation %q not allowed (v1: notify only)", au.Name, au.OperationSlug)}
	}
	if p := au.Predicate; p != nil {
		if p.Field == "" {
			return &domain.Error{Code: "AUTOMATION_INVALID", Message: fmt.Sprintf(
				"automation %q: predicate missing field", au.Name)}
		}
		switch p.Op {
		case "eq", "neq", "in":
		default:
			return &domain.Error{Code: "AUTOMATION_INVALID", Message: fmt.Sprintf(
				"automation %q: predicate op %q not allowed (eq | neq | in)", au.Name, p.Op)}
		}
	}
	return nil
}

// MarkEventProcessed stamps v5_events.processed_at after inline dispatch.
// Idempotent: an already-stamped or unknown id is a no-op (dispatch retries
// must never fail on the stamp).
func (a *EntityAdapter) MarkEventProcessed(ctx context.Context, eventID int64) error {
	_, err := a.client.pool.Exec(ctx, `
		UPDATE v5_events SET processed_at = NOW() WHERE id = $1 AND processed_at IS NULL
	`, eventID)
	if err != nil {
		return fmt.Errorf("mark event %d processed: %w", eventID, err)
	}
	return nil
}
