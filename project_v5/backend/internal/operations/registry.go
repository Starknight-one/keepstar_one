// Package operations implements the operation registry — THE single
// execution choke point of the Keepstar interface runtime (RUNTIME_SPEC.md
// §4.1, rulings R1/R8/R14/R15/R16).
//
// The registry replaces tools.Registry: executors register per
// OperationKind at boot, templates seed into public.v5_operations, tenants
// enable instances in public.v5_tenant_operations, and every execution —
// including denied and invalid — is audited in public.v5_operation_runs
// with x-sensitive keys redacted (R6).
//
// A form (storage key: mode string) selects which operations an agent
// sees; nothing here assumes the form set is closed at storefront/crm/
// onboarding — forms are data.
package operations

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// defaultSpecTTL is the §6.1 TTL safety net default (CACHE_TTL_SECONDS 600).
const defaultSpecTTL = 600 * time.Second

// Store is the persistence surface the registry needs. It extends the
// shared ports.OperationStorePort with the template read/seed methods the
// boot seeder and instance resolution require. Implemented by the postgres
// operations adapter (postgres_operations.go).
type Store interface {
	ports.OperationStorePort
	// UpsertTemplate idempotently upserts one v5_operations row on name
	// (§3.1 boot seed) and returns the row id. embedding may be nil
	// (embedder unset → embedding NULL → SearchLibrary degrades to FTS).
	UpsertTemplate(ctx context.Context, t domain.OperationTemplate, embedding []float32) (string, error)
	// ListTemplates returns every v5_operations row (system library, R30).
	ListTemplates(ctx context.Context) ([]domain.OperationTemplate, error)
}

// TenantResolver is the slice of CatalogPort the registry needs to resolve
// a tenant slug to its row (satisfied by every ports.CatalogPort).
type TenantResolver interface {
	GetTenantBySlug(ctx context.Context, slug string) (*domain.Tenant, error)
}

// passthroughExecutor marks executors whose input/output bypass registry
// validation — the legacy wraps, which must stay byte-identical to the
// pre-registry tools (incl. the retry-with-empty-input path). Native
// executors do NOT implement it; their input is validated+coerced.
type passthroughExecutor interface {
	Passthrough() bool
}

// templateRowProvider lets an executor supply its full v5_operations seed
// row (metadata beyond OperationSpec: title, card, auto_enabled). The
// legacy wraps implement it; executors without it get a synthesized
// non-auto-enabled row from Template().
type templateRowProvider interface {
	TemplateRow() domain.OperationTemplate
}

// RegistryConfig wires the registry's dependencies.
type RegistryConfig struct {
	// Store persists templates + instances. Required in production wiring
	// (seed + per-tenant instances); nil is tolerated for unit tests and
	// disables instance resolution and seeding.
	Store Store
	// Runs is the v5_operation_runs audit sink. Best-effort: nil (tests)
	// or append failure never fails an operation.
	Runs ports.OperationRunPort
	// Tenants resolves tenant slugs (required).
	Tenants TenantResolver
	// Embedder embeds template descriptions at seed time. Optional.
	Embedder ports.EmbeddingPort
	// SpecTTL is the per-tenant spec cache TTL (§6.1). 0 → 600s.
	SpecTTL time.Duration
	Log     *slog.Logger
}

// Registry implements ports.OperationRegistry.
type Registry struct {
	store    Store
	runs     ports.OperationRunPort
	tenants  TenantResolver
	embedder ports.EmbeddingPort
	log      *slog.Logger

	mu               sync.RWMutex
	execs            map[string]ports.Executor        // by template wire name
	templates        map[string]domain.OperationTemplate // by name — gate metadata (modes/min_role/auto_enabled)
	templateNameByID map[string]string                // v5_operations.id → name (instance resolution)

	specs *specCache
}

var _ ports.OperationRegistry = (*Registry)(nil)

// NewRegistry builds an empty registry. Executors plug in via
// RegisterExecutor; templates seed via SeedTemplates.
func NewRegistry(cfg RegistryConfig) *Registry {
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	return &Registry{
		store:            cfg.Store,
		runs:             cfg.Runs,
		tenants:          cfg.Tenants,
		embedder:         cfg.Embedder,
		log:              log,
		execs:            make(map[string]ports.Executor),
		templates:        make(map[string]domain.OperationTemplate),
		templateNameByID: make(map[string]string),
		specs:            newSpecCache(cfg.SpecTTL),
	}
}

// RegisterExecutor plugs an executor in at boot time (R8: meta-ops arrive
// via RegisterExecutor(KindMeta, …)). The wire name is the executor's
// Template().Name; registering the same name twice replaces the prior
// entry (last-wins, mirrors the old tools.Registry contract for tests).
func (r *Registry) RegisterExecutor(kind domain.OperationKind, ex ports.Executor) {
	tmpl := ex.Template()
	if tmpl.Kind != "" && tmpl.Kind != kind {
		r.log.Warn("operations: executor kind mismatch", "name", tmpl.Name, "registered_as", kind, "template_kind", tmpl.Kind)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.execs[tmpl.Name] = ex
	if provider, ok := ex.(templateRowProvider); ok {
		r.templates[tmpl.Name] = provider.TemplateRow()
		return
	}
	if _, seeded := r.templates[tmpl.Name]; !seeded {
		// Synthesized fallback row: NOT auto-enabled — executors without a
		// seed row are reachable only through tenant instances.
		r.templates[tmpl.Name] = domain.OperationTemplate{
			Name:         tmpl.Name,
			Kind:         kind,
			Title:        tmpl.Name,
			Description:  tmpl.Description,
			Card:         tmpl.Card,
			InputSchema:  tmpl.InputSchema,
			OutputSchema: tmpl.OutputSchema,
			Effects:      tmpl.Effects,
			Modes:        tmpl.Modes,
			Agent:        tmpl.Agent,
			MinRole:      tmpl.MinRole,
		}
	}
}

// SeedTemplates runs the §3.1 boot seed: the registered executors' rows
// plus extra library rows (seed.Templates(), preset_pack content) are
// upserted into v5_operations on name, descriptions embedded in one batch
// via the embedder (nil → embedding NULL → SearchLibrary degrades to FTS).
// Also (re)binds the in-memory gate metadata and the id→name map used for
// instance resolution, then drops the whole spec cache.
func (r *Registry) SeedTemplates(ctx context.Context, extra []domain.OperationTemplate) error {
	if r.store == nil {
		return fmt.Errorf("operations: seed requires a store")
	}

	r.mu.Lock()
	rows := make([]domain.OperationTemplate, 0, len(r.execs)+len(extra))
	names := make([]string, 0, len(r.execs))
	for name := range r.execs {
		names = append(names, name)
	}
	sort.Strings(names) // deterministic seed order
	for _, name := range names {
		rows = append(rows, r.templates[name])
	}
	r.mu.Unlock()
	for _, t := range extra {
		rows = append(rows, t)
	}

	embeddings := r.embedRows(ctx, rows)

	r.mu.Lock()
	defer r.mu.Unlock()
	for i, row := range rows {
		var emb []float32
		if embeddings != nil {
			emb = embeddings[i]
		}
		id, err := r.store.UpsertTemplate(ctx, row, emb)
		if err != nil {
			return fmt.Errorf("operations: seed %s: %w", row.Name, err)
		}
		row.ID = id
		r.templates[row.Name] = row
		r.templateNameByID[id] = row.Name
	}
	r.specs.invalidateAll()
	return nil
}

// embedRows embeds every row description in one batch. Best-effort: any
// error degrades to nil embeddings (rows seed with embedding NULL).
func (r *Registry) embedRows(ctx context.Context, rows []domain.OperationTemplate) [][]float32 {
	if r.embedder == nil || len(rows) == 0 {
		return nil
	}
	texts := make([]string, len(rows))
	for i, row := range rows {
		texts[i] = row.Description
	}
	embeddings, err := r.embedder.Embed(ctx, texts)
	if err != nil || len(embeddings) != len(rows) {
		r.log.Warn("operations: seed embedding failed — templates seed unembedded", "err", err)
		return nil
	}
	return embeddings
}

// DefinitionsFor returns the LLM tool definitions one agent plane sees for
// a tenant, form (mode) and role: auto-enabled templates plus the tenant's
// enabled instances, per-tenant schemas materialized, sorted by name for
// byte-stable prompt-cache hashes (duty C3). Replaces the agent-side
// name-prefix visibility hack.
func (r *Registry) DefinitionsFor(ctx context.Context, tenantSlug string, mode domain.PipelineMode, agent domain.AgentPlane, role domain.Role) []domain.ToolDefinition {
	mode, role = normalizeMode(mode), normalizeRole(role)
	entry := r.entryFor(ctx, tenantSlug)

	visible := map[string]*domain.OperationSpec{}
	r.mu.RLock()
	names := make([]string, 0, len(r.execs))
	for name := range r.execs {
		names = append(names, name)
	}
	r.mu.RUnlock()

	for _, name := range names {
		ex, tmpl, ok := r.executorAndTemplate(name)
		if !ok || !tmpl.AutoEnabled {
			continue
		}
		if !r.gateOK(tmpl, mode, agent, role) {
			continue
		}
		visible[name] = entry.specFor(ctx, name, ex, nil)
	}
	for instName := range entry.instances {
		inst := entry.instances[instName]
		if !inst.Enabled {
			continue
		}
		tmplName, ok := r.templateName(inst.OperationID)
		if !ok {
			continue
		}
		ex, tmpl, ok := r.executorAndTemplate(tmplName)
		if !ok || !r.gateOK(tmpl, mode, agent, role) {
			continue
		}
		visible[instName] = entry.specFor(ctx, instName, ex, &inst)
	}

	defs := make([]domain.ToolDefinition, 0, len(visible))
	for _, spec := range visible {
		defs = append(defs, spec.ToolDefinition())
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	return defs
}

// Get resolves one operation (tenant instance first, else auto-enabled
// template) to its tenant-materialized spec.
func (r *Registry) Get(ctx context.Context, tenantSlug, name string) (*domain.OperationSpec, error) {
	entry := r.entryFor(ctx, tenantSlug)
	res, ok := r.resolve(ctx, entry, name)
	if !ok {
		return nil, fmt.Errorf("operations: unknown operation %q for tenant %q", name, tenantSlug)
	}
	spec := *entry.specFor(ctx, name, res.ex, res.inst)
	return &spec, nil
}

// resolvedOp is one resolved wire name: executor + gate metadata (+ the
// tenant instance when the name is an instance).
type resolvedOp struct {
	ex   ports.Executor
	tmpl domain.OperationTemplate
	inst *domain.OperationInstance
}

// resolve maps a wire name to its executor: tenant instance first, else
// auto-enabled template. Names without a registered executor (preset_pack
// rows, seeded-but-unbuilt templates) do not resolve — Execute → denied.
func (r *Registry) resolve(_ context.Context, entry *tenantEntry, name string) (resolvedOp, bool) {
	if inst, ok := entry.instances[name]; ok && inst.Enabled {
		if tmplName, ok := r.templateName(inst.OperationID); ok {
			if ex, tmpl, ok := r.executorAndTemplate(tmplName); ok {
				return resolvedOp{ex: ex, tmpl: tmpl, inst: &inst}, true
			}
		}
	}
	ex, tmpl, ok := r.executorAndTemplate(name)
	if !ok || !tmpl.AutoEnabled {
		return resolvedOp{}, false
	}
	return resolvedOp{ex: ex, tmpl: tmpl}, true
}

// Execute is THE choke point. Boundary order (deterministic, §4.1):
// resolve instance (miss → denied) → gate mode ∈ modes && role ≥ min_role
// → validate+coerce input (violations → OutcomeInvalid as IsError
// tool_result so the LLM self-corrects) → executor.Execute → validate
// output (log-only) → redact x-sensitive keys → append v5_operation_runs
// (always, incl. denied/invalid; best-effort) → return.
func (r *Registry) Execute(ctx context.Context, octx domain.OperationContext, call domain.ToolCall) (*domain.OperationResult, error) {
	start := time.Now()
	octx.Mode, octx.Role = normalizeMode(octx.Mode), normalizeRole(octx.Role)
	entry := r.entryFor(ctx, octx.TenantSlug)
	if octx.TenantID == "" && entry.tenant != nil {
		octx.TenantID = entry.tenant.ID
	}

	res, ok := r.resolve(ctx, entry, call.Name)
	if !ok {
		result := failure(call.Name, "", domain.OutcomeDenied,
			fmt.Sprintf("denied: unknown operation %q", call.Name))
		r.audit(ctx, octx, nil, nil, call.Input, result, "", start)
		return result, nil
	}
	if res.inst != nil {
		octx.Config = res.inst.Config
	}
	// Redaction schemas (R6): the materialized per-tenant spec when we have
	// one, else the executor's static template — per-tenant derivations may
	// flag additional x-sensitive keys.
	tmplSpec := res.ex.Template()
	inputSchema, outputSchema := tmplSpec.InputSchema, tmplSpec.OutputSchema

	if !modeAllowed(res.tmpl.Modes, octx.Mode) {
		result := failure(call.Name, res.tmpl.Kind, domain.OutcomeDenied,
			fmt.Sprintf("denied: operation %q is not available in this context", call.Name))
		r.audit(ctx, octx, inputSchema, outputSchema, call.Input, result, "", start)
		return result, nil
	}
	if !octx.Role.AtLeast(res.tmpl.MinRole) {
		result := failure(call.Name, res.tmpl.Kind, domain.OutcomeDenied,
			fmt.Sprintf("denied: operation %q requires role %s", call.Name, res.tmpl.MinRole))
		r.audit(ctx, octx, inputSchema, outputSchema, call.Input, result, "", start)
		return result, nil
	}

	input := call.Input
	_, passthrough := res.ex.(passthroughExecutor)
	var spec *domain.OperationSpec
	if !passthrough {
		spec = entry.specFor(ctx, call.Name, res.ex, res.inst)
		inputSchema, outputSchema = spec.InputSchema, spec.OutputSchema
		cleaned, violations := ValidateInput(spec.InputSchema, input)
		if len(violations) > 0 {
			result := failure(call.Name, res.tmpl.Kind, domain.OutcomeInvalid,
				"invalid: "+strings.Join(violations, "; "))
			r.audit(ctx, octx, inputSchema, outputSchema, input, result, "", start)
			return result, nil
		}
		input = cleaned
	}

	result, err := res.ex.Execute(ctx, octx, input)
	if err != nil {
		r.audit(ctx, octx, inputSchema, outputSchema, input, &domain.OperationResult{
			Operation: call.Name, Kind: res.tmpl.Kind, Outcome: domain.OutcomeError,
		}, err.Error(), start)
		return nil, err // transport-level failure — agents keep their retry path
	}
	if result == nil {
		result = failure(call.Name, res.tmpl.Kind, domain.OutcomeError, "error: operation returned no result")
	}
	if result.Operation == "" {
		result.Operation = call.Name
	}
	if result.Kind == "" {
		result.Kind = res.tmpl.Kind
	}

	if !passthrough && spec != nil {
		if violations := ValidateOutput(spec.OutputSchema, result.Output); len(violations) > 0 {
			r.log.Warn("operations: output schema violation", "operation", call.Name, "violations", violations)
		}
	}

	r.audit(ctx, octx, inputSchema, outputSchema, input, result, "", start)
	return result, nil
}

// InvalidateTenant drops the per-tenant spec cache entry — called by the
// scoped cache-invalidate handler (R15, scope=agent1) and by
// OperationStorePort mutators post-commit.
func (r *Registry) InvalidateTenant(tenantSlug string) {
	r.specs.invalidate(tenantSlug)
}

// ─── internals ───────────────────────────────────────────────────────────

// entryFor returns the cached per-tenant entry, building it on miss.
// Fail-open: tenant or instance resolution errors degrade to a
// template-only entry (logged), mirroring Agent1PromptCache.
func (r *Registry) entryFor(ctx context.Context, tenantSlug string) *tenantEntry {
	if e := r.specs.get(tenantSlug); e != nil {
		return e
	}
	entry := &tenantEntry{
		slug:      tenantSlug,
		instances: map[string]domain.OperationInstance{},
		specs:     map[string]*domain.OperationSpec{},
		builtAt:   time.Now(),
	}
	if tenantSlug != "" && r.tenants != nil {
		tenant, err := r.tenants.GetTenantBySlug(ctx, tenantSlug)
		if err != nil {
			r.log.Warn("operations: tenant resolution failed — template-only fail-open", "tenant", tenantSlug, "err", err)
		} else {
			entry.tenant = tenant
		}
	}
	if r.store != nil && entry.tenant != nil {
		instances, err := r.store.ListEnabled(ctx, entry.tenant.ID)
		if err != nil {
			r.log.Warn("operations: ListEnabled failed — template-only fail-open", "tenant", tenantSlug, "err", err)
		} else {
			for _, inst := range instances {
				entry.instances[inst.Name] = inst
			}
		}
	}
	r.specs.put(tenantSlug, entry)
	return entry
}

func (r *Registry) executorAndTemplate(name string) (ports.Executor, domain.OperationTemplate, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ex, ok := r.execs[name]
	if !ok {
		return nil, domain.OperationTemplate{}, false
	}
	return ex, r.templates[name], true
}

func (r *Registry) templateName(id string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	name, ok := r.templateNameByID[id]
	return name, ok
}

func (r *Registry) gateOK(tmpl domain.OperationTemplate, mode domain.PipelineMode, agent domain.AgentPlane, role domain.Role) bool {
	return modeAllowed(tmpl.Modes, mode) && tmpl.Agent == agent && role.AtLeast(tmpl.MinRole)
}

// audit appends the v5_operation_runs row — always, including denied and
// invalid outcomes; best-effort (an audit failure never fails the
// operation). Two redaction layers run before the row leaves this function
// (R6): name-based credential scrub (RedactCredentialKeys — covers the
// denied/invalid paths where input is the RAW pre-validation map and a
// model-smuggled password key is invisible to schemas) + schema-driven
// x-sensitive redaction, on BOTH input and output.
func (r *Registry) audit(ctx context.Context, octx domain.OperationContext, inputSchema, outputSchema, input map[string]any, result *domain.OperationResult, errStr string, start time.Time) {
	if r.runs == nil {
		return
	}
	run := domain.OperationRun{
		TenantID:  octx.TenantID,
		SessionID: octx.SessionID,
		TurnID:    octx.TurnID,
		Operation: result.Operation,
		Kind:      result.Kind,
		Mode:      octx.Mode,
		ActorRole: octx.Role,
		ActorID:   octx.ActorID,
		Input:     RedactSensitive(inputSchema, RedactCredentialKeys(input)),
		Output:    RedactSensitive(outputSchema, RedactCredentialKeys(result.Output)),
		Outcome:   result.Outcome,
		Error:     errStr,
		LatencyMS: int(time.Since(start).Milliseconds()),
	}
	if run.Operation == "" {
		run.Operation = "unknown"
	}
	if err := r.runs.Append(ctx, run); err != nil {
		r.log.Warn("operations: run audit append failed", "operation", run.Operation, "err", err)
	}
}

func failure(name string, kind domain.OperationKind, outcome domain.OpOutcome, summary string) *domain.OperationResult {
	return &domain.OperationResult{Operation: name, Kind: kind, Outcome: outcome, Summary: summary}
}

func modeAllowed(modes []domain.PipelineMode, mode domain.PipelineMode) bool {
	for _, m := range modes {
		if m == mode {
			return true
		}
	}
	return false
}

// normalizeMode defaults an unset mode to the storefront form — the
// v5_chat_sessions.mode column default (R17).
func normalizeMode(m domain.PipelineMode) domain.PipelineMode {
	if m == "" {
		return domain.ModeStorefront
	}
	return m
}

// normalizeRole defaults an unset role to visitor (R14 ladder floor).
func normalizeRole(role domain.Role) domain.Role {
	if role == "" {
		return domain.RoleVisitor
	}
	return role
}
