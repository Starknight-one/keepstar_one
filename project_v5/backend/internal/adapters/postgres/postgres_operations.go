package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/operations"
	"keepstar_v5/internal/ports"
)

// OperationAdapter implements operations.Store (template seed/read, library
// search, tenant-instance CRUD) and ports.OperationRunPort (the append-only
// v5_operation_runs audit sink) against the public.v5_* operation plane
// (RUNTIME_SPEC.md §3.1, rulings R14/R29/R30).
//
// Cache duty (§6.1): the adapter is pure persistence — callers of the
// mutators (ManifestApplier / future admin surface) invalidate
// registry.InvalidateTenant + Agent1PromptCache post-commit; the registry's
// TTL net heals anything missed.
type OperationAdapter struct {
	client *Client
}

// NewOperationAdapter constructs the operation-plane adapter.
func NewOperationAdapter(client *Client) *OperationAdapter {
	return &OperationAdapter{client: client}
}

var (
	_ operations.Store       = (*OperationAdapter)(nil)
	_ ports.OperationRunPort = (*OperationAdapter)(nil)
)

// operationTemplateSelect is the shared v5_operations column list scanned by
// scanOperationTemplate.
const operationTemplateSelect = `
	SELECT id::text, name, kind, title, description, card, input_schema,
	       output_schema, effects, config_schema, modes, agent, min_role,
	       auto_enabled, created_at, updated_at
	FROM v5_operations
`

// operationLibraryFTS is the on-the-fly FTS document for SearchLibrary —
// the library is tens of rows (no ivfflat, no stored tsv column v1; §3.1).
const operationLibraryFTS = `to_tsvector('simple', name || ' ' || title || ' ' || description)`

// UpsertTemplate idempotently upserts one v5_operations row on name (§3.1
// boot seed) and returns the row id. embedding may be nil — the row keeps
// any previously written embedding (a keyless boot never erases vectors;
// fresh rows seed with embedding NULL → SearchLibrary degrades to FTS).
func (a *OperationAdapter) UpsertTemplate(ctx context.Context, t domain.OperationTemplate, embedding []float32) (string, error) {
	card, err := jsonbMap(t.Card)
	if err != nil {
		return "", fmt.Errorf("upsert template %s: card: %w", t.Name, err)
	}
	inputSchema, err := jsonbMap(t.InputSchema)
	if err != nil {
		return "", fmt.Errorf("upsert template %s: input_schema: %w", t.Name, err)
	}
	outputSchema, err := jsonbMap(t.OutputSchema)
	if err != nil {
		return "", fmt.Errorf("upsert template %s: output_schema: %w", t.Name, err)
	}
	configSchema, err := jsonbMap(t.ConfigSchema)
	if err != nil {
		return "", fmt.Errorf("upsert template %s: config_schema: %w", t.Name, err)
	}
	effects, err := json.Marshal(normalizeEffects(t.Effects))
	if err != nil {
		return "", fmt.Errorf("upsert template %s: effects: %w", t.Name, err)
	}

	title := t.Title
	if title == "" {
		title = t.Name // title is NOT NULL; synthesized rows fall back to the wire name
	}
	agent := t.Agent
	if agent == "" {
		agent = domain.AgentData
	}
	minRole := t.MinRole
	if minRole == "" {
		minRole = domain.RoleVisitor
	}

	var emb interface{}
	if len(embedding) > 0 {
		emb = pgvector.NewVector(embedding)
	}

	var id string
	err = a.client.pool.QueryRow(ctx, `
		INSERT INTO v5_operations
			(name, kind, title, description, card, input_schema, output_schema,
			 effects, config_schema, modes, agent, min_role, auto_enabled, embedding)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (name) DO UPDATE SET
			kind          = EXCLUDED.kind,
			title         = EXCLUDED.title,
			description   = EXCLUDED.description,
			card          = EXCLUDED.card,
			input_schema  = EXCLUDED.input_schema,
			output_schema = EXCLUDED.output_schema,
			effects       = EXCLUDED.effects,
			config_schema = EXCLUDED.config_schema,
			modes         = EXCLUDED.modes,
			agent         = EXCLUDED.agent,
			min_role      = EXCLUDED.min_role,
			auto_enabled  = EXCLUDED.auto_enabled,
			embedding     = COALESCE(EXCLUDED.embedding, v5_operations.embedding),
			updated_at    = now()
		RETURNING id::text
	`, t.Name, string(t.Kind), title, t.Description, card, inputSchema, outputSchema,
		effects, configSchema, modesToStrings(t.Modes), string(agent), string(minRole),
		t.AutoEnabled, emb).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert template %s: %w", t.Name, err)
	}
	return id, nil
}

// ListTemplates returns every v5_operations row — the whole system library
// (R30: templates + preset_pack rows only, no tenant rows in this table).
func (a *OperationAdapter) ListTemplates(ctx context.Context) ([]domain.OperationTemplate, error) {
	rows, err := a.client.pool.Query(ctx, operationTemplateSelect+` ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()
	return scanOperationTemplates(rows)
}

// SearchLibrary ranks the system library by fused score — pgvector cosine
// (when an embedding is supplied) plus on-the-fly ts_rank — mirroring the
// catalog SearchProjection read. embedding nil + query "" returns the
// newest-name-first slice head (browse). kinds narrows on v5_operations.kind
// when non-empty.
func (a *OperationAdapter) SearchLibrary(ctx context.Context, embedding []float32, query string, kinds []string, k int) ([]domain.OperationTemplate, error) {
	if k <= 0 {
		k = 10
	}

	var embArg interface{}
	if len(embedding) > 0 {
		embArg = pgvector.NewVector(embedding)
	}
	var kindsArg interface{}
	if len(kinds) > 0 {
		kindsArg = kinds
	}

	rows, err := a.client.pool.Query(ctx, operationTemplateSelect+`
		WHERE ($3::text[] IS NULL OR kind = ANY($3::text[]))
		  AND ($1::vector IS NOT NULL OR $2 = '' OR `+operationLibraryFTS+` @@ plainto_tsquery('simple', $2))
		ORDER BY
			COALESCE(CASE WHEN $1::vector IS NOT NULL AND embedding IS NOT NULL
			              THEN 1 - (embedding <=> $1::vector) END, 0)
			+ COALESCE(CASE WHEN $2 <> ''
			                THEN ts_rank(`+operationLibraryFTS+`, plainto_tsquery('simple', $2)) END, 0) DESC,
			name
		LIMIT $4
	`, embArg, query, kindsArg, k)
	if err != nil {
		return nil, fmt.Errorf("search library: %w", err)
	}
	defer rows.Close()
	return scanOperationTemplates(rows)
}

// EnableOperation creates (or re-enables) a tenant instance of a template
// under an instance wire name. cfg is validated against the template's
// config_schema (violations → error, §3.1). Idempotent upsert on
// (tenant_id, name) — the manifest applier's retry contract (§4.3).
func (a *OperationAdapter) EnableOperation(ctx context.Context, tenantID, templateName, instanceName string, cfg map[string]any) (*domain.TenantOperation, error) {
	var templateID string
	var configSchemaRaw []byte
	err := a.client.pool.QueryRow(ctx, `
		SELECT id::text, config_schema FROM v5_operations WHERE name = $1
	`, templateName).Scan(&templateID, &configSchemaRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("enable operation: unknown template %q", templateName)
	}
	if err != nil {
		return nil, fmt.Errorf("enable operation: resolve template %s: %w", templateName, err)
	}

	cleaned, err := validateInstanceConfig(configSchemaRaw, cfg)
	if err != nil {
		return nil, fmt.Errorf("enable operation %s: %w", instanceName, err)
	}
	cfgJSON, err := jsonbMap(cleaned)
	if err != nil {
		return nil, fmt.Errorf("enable operation %s: config: %w", instanceName, err)
	}

	row := a.client.pool.QueryRow(ctx, `
		INSERT INTO v5_tenant_operations (tenant_id, operation_id, name, config)
		VALUES ($1::uuid, $2::uuid, $3, $4)
		ON CONFLICT (tenant_id, name) DO UPDATE SET
			operation_id = EXCLUDED.operation_id,
			config       = EXCLUDED.config,
			enabled      = true,
			updated_at   = now()
		RETURNING id::text, tenant_id::text, operation_id::text, name, enabled,
		          config, COALESCE(description, ''), created_at, updated_at
	`, tenantID, templateID, instanceName, cfgJSON)
	inst, err := scanOperationInstance(row)
	if err != nil {
		return nil, fmt.Errorf("enable operation %s: %w", instanceName, err)
	}
	return inst, nil
}

// UpdateInstanceConfig replaces one instance's config after validating it
// against the template's config_schema.
func (a *OperationAdapter) UpdateInstanceConfig(ctx context.Context, tenantID, instanceName string, cfg map[string]any) error {
	var configSchemaRaw []byte
	err := a.client.pool.QueryRow(ctx, `
		SELECT o.config_schema
		FROM v5_tenant_operations t
		JOIN v5_operations o ON o.id = t.operation_id
		WHERE t.tenant_id = $1::uuid AND t.name = $2
	`, tenantID, instanceName).Scan(&configSchemaRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("update instance config: unknown instance %q", instanceName)
	}
	if err != nil {
		return fmt.Errorf("update instance config %s: %w", instanceName, err)
	}

	cleaned, err := validateInstanceConfig(configSchemaRaw, cfg)
	if err != nil {
		return fmt.Errorf("update instance config %s: %w", instanceName, err)
	}
	cfgJSON, err := jsonbMap(cleaned)
	if err != nil {
		return fmt.Errorf("update instance config %s: config: %w", instanceName, err)
	}

	_, err = a.client.pool.Exec(ctx, `
		UPDATE v5_tenant_operations SET config = $3, updated_at = now()
		WHERE tenant_id = $1::uuid AND name = $2
	`, tenantID, instanceName, cfgJSON)
	if err != nil {
		return fmt.Errorf("update instance config %s: %w", instanceName, err)
	}
	return nil
}

// DisableOperation flips one instance off. Lifecycle is enable/disable —
// there is no delete API (guardrails §4.4).
func (a *OperationAdapter) DisableOperation(ctx context.Context, tenantID, instanceName string) error {
	tag, err := a.client.pool.Exec(ctx, `
		UPDATE v5_tenant_operations SET enabled = false, updated_at = now()
		WHERE tenant_id = $1::uuid AND name = $2
	`, tenantID, instanceName)
	if err != nil {
		return fmt.Errorf("disable operation %s: %w", instanceName, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("disable operation: unknown instance %q", instanceName)
	}
	return nil
}

// ListEnabled returns the tenant's enabled instances, name-sorted (byte-
// stable prompt-cache duty C3 flows from here through DefinitionsFor).
func (a *OperationAdapter) ListEnabled(ctx context.Context, tenantID string) ([]domain.TenantOperation, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT id::text, tenant_id::text, operation_id::text, name, enabled,
		       config, COALESCE(description, ''), created_at, updated_at
		FROM v5_tenant_operations
		WHERE tenant_id = $1::uuid AND enabled
		ORDER BY name
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list enabled operations: %w", err)
	}
	defer rows.Close()

	var out []domain.TenantOperation
	for rows.Next() {
		inst, err := scanOperationInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("scan enabled operation: %w", err)
		}
		out = append(out, *inst)
	}
	return out, rows.Err()
}

// Append writes one v5_operation_runs audit row. x-sensitive keys are
// redacted BEFORE this call (registry choke point, R6). Best-effort by
// contract — the registry logs and swallows any error returned here.
func (a *OperationAdapter) Append(ctx context.Context, run domain.OperationRun) error {
	if run.TenantID == "" {
		// tenant_id is NOT NULL UUID; a run without tenant identity (failed
		// tenant resolution) cannot be stored — surface it to the warn log.
		return fmt.Errorf("append operation run %s: missing tenant id", run.Operation)
	}
	input, err := jsonbMap(run.Input)
	if err != nil {
		return fmt.Errorf("append operation run %s: input: %w", run.Operation, err)
	}
	var output interface{}
	if run.Output != nil {
		outJSON, err := jsonbMap(run.Output)
		if err != nil {
			return fmt.Errorf("append operation run %s: output: %w", run.Operation, err)
		}
		output = outJSON
	}

	_, err = a.client.pool.Exec(ctx, `
		INSERT INTO v5_operation_runs
			(tenant_id, session_id, turn_id, operation, kind, mode, actor_role,
			 actor_id, input, output, outcome, error, latency_ms)
		VALUES ($1::uuid, NULLIF($2, ''), NULLIF($3, ''), $4, $5, $6, $7,
		        NULLIF($8, ''), $9, $10, $11, NULLIF($12, ''), $13)
	`, run.TenantID, run.SessionID, run.TurnID, run.Operation, string(run.Kind),
		string(run.Mode), string(run.ActorRole), run.ActorID, input, output,
		string(run.Outcome), run.Error, run.LatencyMS)
	if err != nil {
		return fmt.Errorf("append operation run %s: %w", run.Operation, err)
	}
	return nil
}

// ─── scan + marshal helpers ──────────────────────────────────────────────

func scanOperationTemplates(rows pgx.Rows) ([]domain.OperationTemplate, error) {
	var out []domain.OperationTemplate
	for rows.Next() {
		t, err := scanOperationTemplate(rows)
		if err != nil {
			return nil, fmt.Errorf("scan operation template: %w", err)
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func scanOperationTemplate(row pgx.Row) (*domain.OperationTemplate, error) {
	var (
		t                     domain.OperationTemplate
		kind, agent, minRole  string
		card, inputSchema     []byte
		outputSchema, effects []byte
		configSchema          []byte
		modes                 []string
	)
	if err := row.Scan(&t.ID, &t.Name, &kind, &t.Title, &t.Description, &card,
		&inputSchema, &outputSchema, &effects, &configSchema, &modes, &agent,
		&minRole, &t.AutoEnabled, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	t.Kind = domain.OperationKind(kind)
	t.Agent = domain.AgentPlane(agent)
	t.MinRole = domain.Role(minRole)
	t.Modes = stringsToModes(modes)
	var err error
	if t.Card, err = unmarshalJSONMap(card); err != nil {
		return nil, fmt.Errorf("card: %w", err)
	}
	if t.InputSchema, err = unmarshalJSONMap(inputSchema); err != nil {
		return nil, fmt.Errorf("input_schema: %w", err)
	}
	if t.OutputSchema, err = unmarshalJSONMap(outputSchema); err != nil {
		return nil, fmt.Errorf("output_schema: %w", err)
	}
	if t.ConfigSchema, err = unmarshalJSONMap(configSchema); err != nil {
		return nil, fmt.Errorf("config_schema: %w", err)
	}
	if len(effects) > 0 {
		if err := json.Unmarshal(effects, &t.Effects); err != nil {
			return nil, fmt.Errorf("effects: %w", err)
		}
	}
	return &t, nil
}

func scanOperationInstance(row pgx.Row) (*domain.OperationInstance, error) {
	var (
		inst   domain.OperationInstance
		config []byte
	)
	if err := row.Scan(&inst.ID, &inst.TenantID, &inst.OperationID, &inst.Name,
		&inst.Enabled, &config, &inst.Description, &inst.CreatedAt, &inst.UpdatedAt); err != nil {
		return nil, err
	}
	var err error
	if inst.Config, err = unmarshalJSONMap(config); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &inst, nil
}

// validateInstanceConfig validates cfg against a raw config_schema JSONB via
// the registry's restricted-dialect validator (§3.1: config validated vs
// config_schema on write). Empty schema validates everything.
func validateInstanceConfig(configSchemaRaw []byte, cfg map[string]any) (map[string]any, error) {
	schema, err := unmarshalJSONMap(configSchemaRaw)
	if err != nil {
		return nil, fmt.Errorf("config_schema: %w", err)
	}
	cleaned, violations := operations.ValidateInput(schema, cfg)
	if len(violations) > 0 {
		return nil, fmt.Errorf("config invalid: %s", strings.Join(violations, "; "))
	}
	return cleaned, nil
}

// jsonbMap marshals a map for a NOT NULL JSONB column: nil → {}.
func jsonbMap(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

func unmarshalJSONMap(b []byte) (map[string]any, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// normalizeEffects pins nil slices to empty so stored rows match the DDL
// default shape {"reads":[],"writes":[],"emits":[]}.
func normalizeEffects(e domain.OperationEffects) domain.OperationEffects {
	if e.Reads == nil {
		e.Reads = []string{}
	}
	if e.Writes == nil {
		e.Writes = []string{}
	}
	if e.Emits == nil {
		e.Emits = []domain.EmitDecl{}
	}
	return e
}

func modesToStrings(modes []domain.PipelineMode) []string {
	out := make([]string, len(modes))
	for i, m := range modes {
		out[i] = string(m)
	}
	return out
}

func stringsToModes(s []string) []domain.PipelineMode {
	if s == nil {
		return nil
	}
	out := make([]domain.PipelineMode, len(s))
	for i, m := range s {
		out[i] = domain.PipelineMode(m)
	}
	return out
}
