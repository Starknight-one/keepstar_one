package domain

import "time"

// Entity plane — runtime-born data (canonical spec: RUNTIME_SPEC.md §4.4,
// rulings R10/R12/R18/R27/R28). Tenants define entities (lead, request, …)
// at onboarding; records are created by operations, events are written in
// the SAME transaction as the record write (v5_events outbox), automations
// dispatch inline post-commit (notify only, v1).
//
// Guardrails (checked in code, not prose): closed 10-type field set; no
// formulas/rollups/attachments; last-write-wins; one catalog ref per
// record; no delete API (lifecycle = status); flat queries (no aggregation,
// group-by, FTS or vector); definitions additive-only. Anything beyond is a
// v2 conversation.

// Soft caps (spec §4.4 guardrails).
const (
	MaxEntityDefinitionsPerTenant = 20
	MaxFieldsPerEntity            = 50
	MaxValuesPerValueSet          = 50
)

// FieldType is the closed 10-type field vocabulary.
type FieldType string

const (
	FieldText     FieldType = "text"
	FieldNumber   FieldType = "number"
	FieldMoney    FieldType = "money" // stored usd_cents; LLM-facing dollars (R18)
	FieldBool     FieldType = "bool"
	FieldDate     FieldType = "date"
	FieldDatetime FieldType = "datetime"
	FieldEnum     FieldType = "enum" // values via ValueSetRef
	FieldPhone    FieldType = "phone"
	FieldEmail    FieldType = "email"
	FieldRef      FieldType = "ref" // one catalog/entity link per record
)

// FieldDef is one ordered field of an entity definition. Key is camelCase,
// validated SnakeToCamel(key)==key && ^[a-z][a-zA-Z0-9]*$ (§6.2) — the
// binding vocabulary and the record Data keys are the same namespace.
type FieldDef struct {
	Key         string    `json:"key"`
	Label       string    `json:"label"`
	Type        FieldType `json:"type"`
	Unit        UnitName  `json:"unit,omitempty"`        // number only; money/date/datetime imply theirs (R18)
	ValueSetRef string    `json:"valueSetRef,omitempty"` // enum only; slug into v5_value_sets
	RefTarget   string    `json:"refTarget,omitempty"`   // ref only ('product' or an entity slug)
	Required    bool      `json:"required,omitempty"`
	Default     any       `json:"default,omitempty"`
}

// EntityDisplay drives how presets title/badge/sort records
// (v5_entity_definitions.display JSONB).
type EntityDisplay struct {
	TitleField    string `json:"titleField,omitempty"`
	SubtitleField string `json:"subtitleField,omitempty"`
	BadgeField    string `json:"badgeField,omitempty"`
	DefaultSort   string `json:"defaultSort,omitempty"`
}

// EntityDefinition is a public.v5_entity_definitions row. Additive-only:
// type changes, field removal and slug rename are rejected once records
// exist.
type EntityDefinition struct {
	ID          string        `json:"id"`
	TenantID    string        `json:"tenantId"`
	Slug        string        `json:"slug"`
	Name        string        `json:"name"`
	NamePlural  string        `json:"namePlural,omitempty"`
	Fields      []FieldDef    `json:"fields"`
	Display     EntityDisplay `json:"display"`
	StatusField string        `json:"statusField,omitempty"`
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
}

// ValueSetEntry is one ordered value: {value, label, color?} — ordered by
// array position, no key/order spellings (R27).
type ValueSetEntry struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Color string `json:"color,omitempty"`
}

// ValueSet is a public.v5_value_sets row.
type ValueSet struct {
	ID        string          `json:"id"`
	TenantID  string          `json:"tenantId"`
	Slug      string          `json:"slug"`
	Name      string          `json:"name"`
	Values    []ValueSetEntry `json:"values"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// EntityRecord is a public.v5_entity_records row. Data keys are camelCase
// (the FieldDef.Key namespace); Status mirrors data[statusField] as a hot
// filter column — EntityPort is the ONLY writer, keeping the mirror true.
type EntityRecord struct {
	ID                 string         `json:"id"`
	TenantID           string         `json:"tenantId"`
	EntityDefinitionID string         `json:"entityDefinitionId,omitempty"`
	EntitySlug         string         `json:"entitySlug"`
	Data               map[string]any `json:"data"`
	Status             string         `json:"status,omitempty"`
	RefEntityType      string         `json:"refEntityType,omitempty"` // link into the catalog plane ('product')
	RefEntityID        string         `json:"refEntityId,omitempty"`
	CreatedBy          string         `json:"createdBy,omitempty"` // "visitor:<session_id>" | "user:<admin_user_id>"
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
}

// Note: the catalog link on a new record (EntityWrite.CreateRecord's
// `ref *EntityRef`) reuses the EXISTING domain.EntityRef from state.go —
// same {type, id} wire shape, no second ref type.

// RecordFilter is the flat query surface for QueryRecords — no aggregation,
// group-by, FTS or vector (guardrail).
type RecordFilter struct {
	Status      string
	Since       *time.Time
	RefEntityID string
	Attrs       map[string]string
	AttrMin     map[string]float64
	AttrMax     map[string]float64
	SortField   string
	SortOrder   string
	Limit       int
	Offset      int
}

// Runtime event types (v5_events.event_type; identity = event_type +
// entity_slug, R10).
const (
	EventRecordCreated       = "record.created"
	EventRecordUpdated       = "record.updated"
	EventRecordStatusChanged = "record.status_changed"
)

// RuntimeEvent is a public.v5_events row: outbox + audit, written in the
// SAME tx as the record write. processed_at is stamped by inline dispatch;
// the sweeper is v2 (R12) — unprocessed rows are curator-visible.
type RuntimeEvent struct {
	ID          int64          `json:"id"`
	TenantID    string         `json:"tenantId"`
	EventType   string         `json:"eventType"`
	EntitySlug  string         `json:"entitySlug,omitempty"`
	RecordID    string         `json:"recordId,omitempty"`
	Payload     map[string]any `json:"payload"` // {actor, snapshot} | {actor, diff:{before,after}}
	OccurredAt  time.Time      `json:"occurredAt"`
	ProcessedAt *time.Time     `json:"processedAt,omitempty"`
}

// AutomationPredicate is the single flat condition an automation may carry:
// {field, op: eq|neq|in, value}. Deterministic evaluation, no chains.
type AutomationPredicate struct {
	Field string `json:"field"`
	Op    string `json:"op"` // eq | neq | in
	Value any    `json:"value"`
}

// Automation is a public.v5_automations row. Flat v1: on event
// [if predicate] run operation — OperationSlug is 'notify' only in v1.
type Automation struct {
	ID              string               `json:"id"`
	TenantID        string               `json:"tenantId"`
	Name            string               `json:"name"`
	EventType       string               `json:"eventType"`
	EntitySlug      string               `json:"entitySlug,omitempty"` // empty = any
	Predicate       *AutomationPredicate `json:"predicate,omitempty"`
	OperationSlug   string               `json:"operationSlug"`
	OperationParams map[string]any       `json:"operationParams"`
	Enabled         bool                 `json:"enabled"`
	CreatedAt       time.Time            `json:"createdAt"`
	UpdatedAt       time.Time            `json:"updatedAt"`
}

// Notification is a public.v5_notifications row (R12). Write path only in
// v1 — read endpoints, unread badge and the sweeper are v2.
type Notification struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenantId"`
	Audience   string     `json:"audience"` // default 'crm'
	Title      string     `json:"title"`
	Body       string     `json:"body,omitempty"`
	EntitySlug string     `json:"entitySlug,omitempty"`
	RecordID   string     `json:"recordId,omitempty"`
	ReadAt     *time.Time `json:"readAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}
