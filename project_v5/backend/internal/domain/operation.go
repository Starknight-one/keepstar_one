package domain

import (
	"sort"
	"time"
)

// Operation plane — shared contracts for the Keepstar interface runtime v1
// (canonical spec: RUNTIME_SPEC.md §4.1, rulings R1/R8/R10/R14/R18/R30).
//
// An operation is a validated, closed-set capability the runtime executes by
// config: templates live in public.v5_operations, per-tenant instances in
// public.v5_tenant_operations, every execution is audited in
// public.v5_operation_runs. The single execution choke point is the
// operation registry (ports.OperationRegistry).

// PipelineMode is the storage key of a *form* — a runtime configuration:
// a prompt set + prompt injections + a tool/operation set + a design
// (presets/theme) + a data scope. "storefront", "crm" and "onboarding" are
// the three forms v1 ships; onboarding is simply a system form whose
// operations assemble other forms. A form is DATA: nothing may assume the
// set is closed at exactly these three (owner terminology ruling,
// RUNTIME_SPEC.md "Terminology correction"). The type name and the `mode`
// column are kept as specced — the mode string IS the form's storage key.
type PipelineMode string

const (
	ModeStorefront PipelineMode = "storefront"
	ModeCRM        PipelineMode = "crm"
	ModeOnboarding PipelineMode = "onboarding"
)

// AgentPlane says which pipeline agent an operation is exposed to.
type AgentPlane string

const (
	AgentData   AgentPlane = "data"   // Agent1
	AgentVisual AgentPlane = "visual" // Agent2
)

// Role is the closed v1 ladder: visitor < staff < owner < system (R14).
// Enforced as v5_operations.min_role at the registry Execute choke point.
// "member" does not exist; admin workspace roles map in v2.
type Role string

const (
	RoleVisitor Role = "visitor"
	RoleStaff   Role = "staff"
	RoleOwner   Role = "owner"
	RoleSystem  Role = "system"
)

// roleRank orders the closed ladder. Unknown role strings rank below
// visitor so a corrupted or foreign role can never satisfy any gate.
var roleRank = map[Role]int{RoleVisitor: 0, RoleStaff: 1, RoleOwner: 2, RoleSystem: 3}

// AtLeast reports whether r satisfies a minimum-role gate. Unknown r or
// unknown min both fail closed.
func (r Role) AtLeast(min Role) bool {
	rr, okR := roleRank[r]
	mr, okM := roleRank[min]
	return okR && okM && rr >= mr
}

// OperationKind is the closed executor set (spec §3.1). Adding a kind is a
// spec conversation, not a config change.
type OperationKind string

const (
	KindQuery            OperationKind = "query"
	KindCreateRecord     OperationKind = "create_record"
	KindUpdateRecord     OperationKind = "update_record"
	KindTransitionStatus OperationKind = "transition_status"
	KindScheduleSlot     OperationKind = "schedule_slot"
	KindNotify           OperationKind = "notify"
	KindMeta             OperationKind = "meta"        // onboarding meta-operations
	KindVisual           OperationKind = "visual"      // visual_assembly / compose_turn
	KindInternal         OperationKind = "internal"    // _internal_* wraps
	KindPresetPack       OperationKind = "preset_pack" // library content, never executable
)

// UnitName is the closed unit vocabulary (R18), trimmed to what v1 touches.
// Stated once: storage = cents; LLM-facing money = dollars; unit is implied
// by field type for money/date/datetime. `percent` and `duration_min` are
// added when an operation actually uses them.
type UnitName string

const (
	UnitUSDCents        UnitName = "usd_cents"
	UnitUSD             UnitName = "usd"
	UnitCount           UnitName = "count"
	UnitM2              UnitName = "m2"
	UnitRooms           UnitName = "rooms"
	UnitDatetimeISO8601 UnitName = "datetime_iso8601"
	UnitDateISO8601     UnitName = "date_iso8601"
	UnitPhoneE164       UnitName = "phone_e164"
	UnitEmail           UnitName = "email"
	UnitURL             UnitName = "url"
	UnitIDRef           UnitName = "id_ref"
	UnitEnumValueSet    UnitName = "enum_value_set"
)

var validUnits = map[UnitName]bool{
	UnitUSDCents: true, UnitUSD: true, UnitCount: true, UnitM2: true,
	UnitRooms: true, UnitDatetimeISO8601: true, UnitDateISO8601: true,
	UnitPhoneE164: true, UnitEmail: true, UnitURL: true, UnitIDRef: true,
	UnitEnumValueSet: true,
}

// Valid reports whether u belongs to the closed R18 vocabulary.
func (u UnitName) Valid() bool { return validUnits[u] }

// Restricted schema dialect extension keys (spec §4.1). Operation input/
// output schemas are flat JSON-Schema-style property maps; each property may
// additionally carry:
//   - "x-unit":      a UnitName — drives validator coercion (usd→cents,
//     ISO-8601 parse, E.164 shape).
//   - "x-sensitive": true — the registry redacts the value before EVERY
//     persistence (runs/traces/deltas) and it never reaches the LLM (R6).
const (
	SchemaKeyUnit      = "x-unit"
	SchemaKeySensitive = "x-sensitive"
)

// SensitiveKeys returns the sorted property paths flagged "x-sensitive": true
// in a restricted-dialect schema. The dialect allows one nesting level for a
// filters-style sub-object, so paths are either "key" or "parent.child".
// This is the shared lookup for the R6 redaction choke point.
func SensitiveKeys(schema map[string]any) []string {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	var keys []string
	for name, raw := range props {
		prop, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if flag, _ := prop[SchemaKeySensitive].(bool); flag {
			keys = append(keys, name)
		}
		sub, ok := prop["properties"].(map[string]any)
		if !ok {
			continue
		}
		for subName, subRaw := range sub {
			subProp, ok := subRaw.(map[string]any)
			if !ok {
				continue
			}
			if flag, _ := subProp[SchemaKeySensitive].(bool); flag {
				keys = append(keys, name+"."+subName)
			}
		}
	}
	sort.Strings(keys)
	return keys
}

// EmitDecl is one declared event emission: event identity is
// event_type + entity_slug columns, never colon-suffixed names (R10).
type EmitDecl struct {
	EventType  string `json:"event_type"`
	EntitySlug string `json:"entity_slug"`
}

// OperationEffects declares what an operation touches. JSON shape mirrors
// the v5_operations.effects JSONB default {"reads":[],"writes":[],"emits":[]}.
type OperationEffects struct {
	Reads  []string   `json:"reads"`
	Writes []string   `json:"writes"`
	Emits  []EmitDecl `json:"emits"`
}

// OperationSpec is the executable definition surface the registry exposes to
// the agents: template- or tenant-derived (Executor.SpecForTenant).
type OperationSpec struct {
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Kind         OperationKind    `json:"kind"`
	Card         map[string]any   `json:"card"` // {input_summary, does, output_summary, why}
	InputSchema  map[string]any   `json:"inputSchema"`
	OutputSchema map[string]any   `json:"outputSchema"`
	Effects      OperationEffects `json:"effects"`
	Modes        []PipelineMode   `json:"modes"`
	Agent        AgentPlane       `json:"agent"`
	MinRole      Role             `json:"minRole"`
}

// ToolDefinition converts the spec into the LLM-facing tool shape.
func (s OperationSpec) ToolDefinition() ToolDefinition {
	return ToolDefinition{Name: s.Name, Description: s.Description, InputSchema: s.InputSchema}
}

// OpOutcome is the structured execution outcome — kills "empty:" prefix
// sniffing (the wrapper for legacy tools is the LAST place that prefix is
// ever read).
type OpOutcome string

const (
	OutcomeOK      OpOutcome = "ok"
	OutcomeEmpty   OpOutcome = "empty"
	OutcomeInvalid OpOutcome = "invalid"
	OutcomeDenied  OpOutcome = "denied"
	OutcomeError   OpOutcome = "error"
)

// OperationResult is the structured result of one operation execution.
// Summary is the one-line LLM-facing string; legacy tool Summary strings are
// preserved byte-exact by the wrappers so conversation-prefix caches stay
// warm (spec §4.6).
type OperationResult struct {
	Operation  string         `json:"operation"`
	Kind       OperationKind  `json:"kind"`
	Outcome    OpOutcome      `json:"outcome"`
	Count      int            `json:"count"`
	EntityKind string         `json:"entityKind,omitempty"`
	RecordID   string         `json:"recordId,omitempty"`
	Summary    string         `json:"summary"`
	Output     map[string]any `json:"output,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// ToToolResult bridges the structured result to the LLM tool_result shape
// (llm_message.go). Invalid/denied/error surface as IsError so the LLM
// self-corrects; empty is a normal (non-error) result, exactly like today's
// legacy "empty:" strings.
func (r *OperationResult) ToToolResult(toolUseID string) *ToolResult {
	return &ToolResult{
		ToolUseID: toolUseID,
		Content:   r.Summary,
		IsError:   r.Outcome == OutcomeInvalid || r.Outcome == OutcomeDenied || r.Outcome == OutcomeError,
		Metadata:  r.Metadata,
	}
}

// OperationContext carries per-execution identity into the registry choke
// point. Role comes from the session row (visitor default; staff via CRM
// surface token, R13/R17). Config is the resolved
// v5_tenant_operations.config for instance executions (nil for templates).
type OperationContext struct {
	SessionID  string         `json:"sessionId"`
	TenantSlug string         `json:"tenantSlug"`
	TenantID   string         `json:"tenantId"`
	TurnID     string         `json:"turnId"`
	Mode       PipelineMode   `json:"mode"`
	Role       Role           `json:"role"`
	ActorID    string         `json:"actorId,omitempty"`
	Config     map[string]any `json:"config,omitempty"`
}

// OperationTemplate is a public.v5_operations row (system library; R30: no
// version column, UNIQUE(name)). The embedding column stays DB-side — it is
// written at seed/write time via ports.EmbeddingPort and searched live,
// never carried through domain.
type OperationTemplate struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Kind         OperationKind    `json:"kind"`
	Title        string           `json:"title"`
	Description  string           `json:"description"`
	Card         map[string]any   `json:"card"` // {input_summary, does, output_summary, why}
	InputSchema  map[string]any   `json:"inputSchema"`
	OutputSchema map[string]any   `json:"outputSchema"`
	Effects      OperationEffects `json:"effects"`
	ConfigSchema map[string]any   `json:"configSchema"`
	Modes        []PipelineMode   `json:"modes"`
	Agent        AgentPlane       `json:"agent"`
	MinRole      Role             `json:"minRole"`
	AutoEnabled  bool             `json:"autoEnabled"`
	CreatedAt    time.Time        `json:"createdAt"`
	UpdatedAt    time.Time        `json:"updatedAt"`
}

// Spec projects the template onto its executable definition surface.
func (t OperationTemplate) Spec() OperationSpec {
	return OperationSpec{
		Name:         t.Name,
		Description:  t.Description,
		Kind:         t.Kind,
		Card:         t.Card,
		InputSchema:  t.InputSchema,
		OutputSchema: t.OutputSchema,
		Effects:      t.Effects,
		Modes:        t.Modes,
		Agent:        t.Agent,
		MinRole:      t.MinRole,
	}
}

// OperationInstance is a public.v5_tenant_operations row: a template enabled
// for one tenant under an instance wire name (e.g. book_showing over
// schedule_slot), with config validated against the template's
// config_schema on write. No per-instance embedding or re-embed path (R30).
type OperationInstance struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenantId"`
	OperationID string         `json:"operationId"`
	Name        string         `json:"name"`
	Enabled     bool           `json:"enabled"`
	Config      map[string]any `json:"config"`
	Description string         `json:"description,omitempty"` // empty → template description
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

// TenantOperation is the spec's spelling for an enabled per-tenant
// operation row — same type, both names appear in RUNTIME_SPEC.md.
type TenantOperation = OperationInstance

// OperationRun is one public.v5_operation_runs append-only audit row.
// x-sensitive keys are redacted BEFORE this struct is persisted (R6);
// written for every Execute, including denied and invalid outcomes.
type OperationRun struct {
	ID        int64          `json:"id"`
	TenantID  string         `json:"tenantId"`
	SessionID string         `json:"sessionId,omitempty"`
	TurnID    string         `json:"turnId,omitempty"`
	Operation string         `json:"operation"`
	Kind      OperationKind  `json:"kind"`
	Mode      PipelineMode   `json:"mode"`
	ActorRole Role           `json:"actorRole"`
	ActorID   string         `json:"actorId,omitempty"`
	Input     map[string]any `json:"input"`
	Output    map[string]any `json:"output,omitempty"`
	Outcome   OpOutcome      `json:"outcome"`
	Error     string         `json:"error,omitempty"`
	LatencyMS int            `json:"latencyMs"`
	CreatedAt time.Time      `json:"createdAt"`
}
