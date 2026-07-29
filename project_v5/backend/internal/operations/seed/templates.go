// Package seed holds the boot-seed operation templates for
// public.v5_operations (RUNTIME_SPEC.md §3.1): the 6 executor templates,
// compose_turn, and the 12 onboarding meta-operation templates (the §4.3
// eleven plus about_keepstar, owner ruling 2026-07-28). Definitions
// only — executors implement ports.Executor separately and plug in via
// RegisterExecutor (R8). The operations boot-seeder upserts these rows on
// name and embeds Description via ports.EmbeddingPort (nil embedder →
// embedding NULL → SearchLibrary degrades to FTS).
//
// Not here: the legacy wrap rows (catalog_search, _internal_state_filter,
// _internal_history_lookup, visual_assembly) come from the wrappers'
// Template() so their Summary strings stay byte-exact; preset_pack rows are
// library content (M3 realtor content pass).
//
// Effects on meta templates describe APPLY-time effects (what the manifest
// applier touches when the staged step runs) — at stage time a meta-op only
// writes the onboarding manifest zone.
//
// Integrator ruling (M1) on the §6.2-vs-§3.1 spelling conflict: §6.2's
// camelCase seed-time validation is scoped to BINDING vocabulary
// (fieldBinding keys, entity FieldDef keys — engine.SnakeToCamel is their
// only normalizer). Operation wire-schema property names keep the spec's
// literal spellings (to_status, event_type, field_allowlist, datetime_field,
// value_set): they are LLM-facing tool parameters, not binding keys, and
// §3.1/§4.2/§4.3 write them snake_case normatively.
package seed

import "keepstar_v5/internal/domain"

// Templates returns the boot-seed rows in deterministic order:
// 6 executor templates, compose_turn, 12 meta templates.
func Templates() []domain.OperationTemplate {
	return []domain.OperationTemplate{
		queryTemplate(),
		createRecordTemplate(),
		updateRecordTemplate(),
		transitionStatusTemplate(),
		scheduleSlotTemplate(),
		notifyTemplate(),
		composeTurnTemplate(),
		aboutKeepstarTemplate(),
		searchLibraryTemplate(),
		createTenantTemplate(),
		defineEntityTemplate(),
		defineValueSetTemplate(),
		defineAutomationTemplate(),
		enableOperationsTemplate(),
		adoptPresetsTemplate(),
		issueIngestDoorTemplate(),
		registerUserTemplate(),
		issueSurfaceURLsTemplate(),
		applyManifestTemplate(),
	}
}

// tenantModes is the executor-template default (§3.1): onboarding mode
// never sees tenant data operations (R16).
func tenantModes() []domain.PipelineMode {
	return []domain.PipelineMode{domain.ModeStorefront, domain.ModeCRM}
}

func onboardingModes() []domain.PipelineMode {
	return []domain.PipelineMode{domain.ModeOnboarding}
}

// ---------------------------------------------------------------------------
// The 6 executor templates (kind = own kind, agent data, enabled per tenant).
// Input schemas here are the tenant-independent baseline; the real LLM-facing
// schema is derived per tenant by Executor.SpecForTenant (catalog digest /
// entity definition), same pattern as CatalogSearchSchemaForDigest.
// ---------------------------------------------------------------------------

func queryTemplate() domain.OperationTemplate {
	return domain.OperationTemplate{
		Name:  "query",
		Kind:  domain.KindQuery,
		Title: "Search records",
		Description: "Search and filter records from a configured data source: " +
			"the tenant's product catalog (listings, items, offers) or a runtime " +
			"entity such as leads, requests or bookings. Supports keyword queries, " +
			"attribute filters, numeric ranges, status filters and sorting. Results " +
			"load into the session workspace for display. Typical instances: " +
			"catalog search for a storefront, lead_search for a CRM.",
		Card: map[string]any{
			"input_summary":  "A search query with optional filters: attributes, ranges, status, sort",
			"does":           "Runs a typed search over the configured source — the product catalog or an entity such as leads",
			"output_summary": "A ranked list of matching records, loaded into the workspace for display",
			"why":            "Answers questions like 'any new leads?' or '2 bedroom apartments' with real data, never guesses",
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Natural-language search query. Filters and sort fields are derived per tenant from the source's definition.",
				},
				"limit": map[string]any{
					"type":               "integer",
					"description":        "Maximum records to return.",
					domain.SchemaKeyUnit: string(domain.UnitCount),
				},
			},
			"required": []string{"query"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"count": map[string]any{
					"type":               "integer",
					"description":        "Number of records found.",
					domain.SchemaKeyUnit: string(domain.UnitCount),
				},
			},
		},
		Effects: domain.OperationEffects{
			Reads:  []string{"catalog.tenant_search_projection", "v5_entity_records"},
			Writes: []string{},
			Emits:  []domain.EmitDecl{},
		},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source": map[string]any{
					"type":        "string",
					"enum":        []string{"catalog", "entity"},
					"description": "Which plane the instance searches.",
				},
				"entity": map[string]any{
					"type":        "string",
					"description": "Entity slug when source is 'entity' (e.g. 'lead').",
				},
			},
			"required": []string{"source"},
		},
		Modes:       tenantModes(),
		Agent:       domain.AgentData,
		MinRole:     domain.RoleVisitor,
		AutoEnabled: false,
	}
}

func createRecordTemplate() domain.OperationTemplate {
	return domain.OperationTemplate{
		Name:  "create_record",
		Kind:  domain.KindCreateRecord,
		Title: "Create a record",
		Description: "Create a new record of a configured entity — a lead, request, " +
			"application, order note or any runtime-defined shape. Validates field " +
			"values against the entity definition, applies configured defaults, can " +
			"link the record to a catalog item, and emits a record.created event in " +
			"the same transaction so automations (such as staff notifications) fire.",
		Card: map[string]any{
			"input_summary":  "Field values for the configured entity, validated against its definition",
			"does":           "Creates the record with defaults applied and writes its event in one transaction",
			"output_summary": "The new record's id and a one-line confirmation",
			"why":            "Turns a conversation or form into structured CRM data instead of a lost message",
		},
		InputSchema: map[string]any{
			"type":        "object",
			"properties":  map[string]any{},
			"description": "Derived per tenant: EntityRecordSchema(definition, value sets) restricted to the configured field_allowlist.",
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"recordId": map[string]any{
					"type":               "string",
					"description":        "Id of the created record.",
					domain.SchemaKeyUnit: string(domain.UnitIDRef),
				},
			},
		},
		Effects: domain.OperationEffects{
			Reads:  []string{"v5_entity_definitions", "v5_value_sets"},
			Writes: []string{"v5_entity_records", "v5_events"},
			// EntitySlug resolves from instance config at enable time.
			Emits: []domain.EmitDecl{{EventType: domain.EventRecordCreated, EntitySlug: ""}},
		},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"entity":          map[string]any{"type": "string", "description": "Entity slug the instance creates."},
				"defaults":        map[string]any{"type": "object", "description": "Field values applied when the input omits them (e.g. {\"status\":\"new\"})."},
				"field_allowlist": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Fields the caller may set; others come from defaults."},
			},
			"required": []string{"entity"},
		},
		Modes:       tenantModes(),
		Agent:       domain.AgentData,
		MinRole:     domain.RoleVisitor,
		AutoEnabled: false,
	}
}

func updateRecordTemplate() domain.OperationTemplate {
	return domain.OperationTemplate{
		Name:  "update_record",
		Kind:  domain.KindUpdateRecord,
		Title: "Update a record",
		Description: "Update fields of an existing entity record — correct a lead's " +
			"phone number, add a note, change a preferred time. Validates the patch " +
			"against the entity definition, applies last-write-wins semantics and " +
			"emits a record.updated event with the before/after diff.",
		Card: map[string]any{
			"input_summary":  "A record id plus the fields to change",
			"does":           "Validates and applies the patch, keeping an audited before/after diff",
			"output_summary": "The updated record and a one-line confirmation",
			"why":            "Keeps CRM data current without exposing raw database writes",
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":               "string",
					"description":        "Id of the record to update.",
					domain.SchemaKeyUnit: string(domain.UnitIDRef),
				},
				"patch": map[string]any{
					"type":        "object",
					"description": "Fields to change. Allowed keys derive per tenant from the entity definition and field_allowlist.",
				},
			},
			"required": []string{"id", "patch"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"recordId": map[string]any{
					"type":               "string",
					domain.SchemaKeyUnit: string(domain.UnitIDRef),
				},
			},
		},
		Effects: domain.OperationEffects{
			Reads:  []string{"v5_entity_definitions", "v5_value_sets", "v5_entity_records"},
			Writes: []string{"v5_entity_records", "v5_events"},
			Emits:  []domain.EmitDecl{{EventType: domain.EventRecordUpdated, EntitySlug: ""}},
		},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"entity":          map[string]any{"type": "string", "description": "Entity slug the instance updates."},
				"field_allowlist": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Fields the caller may patch."},
			},
			"required": []string{"entity"},
		},
		Modes:       tenantModes(),
		Agent:       domain.AgentData,
		MinRole:     domain.RoleStaff,
		AutoEnabled: false,
	}
}

func transitionStatusTemplate() domain.OperationTemplate {
	return domain.OperationTemplate{
		Name:  "transition_status",
		Kind:  domain.KindTransitionStatus,
		Title: "Advance a record's status",
		Description: "Move an entity record through its pipeline: mark a lead " +
			"contacted, a booking confirmed, a request closed. The target status " +
			"must belong to the configured value set (and to the transition map when " +
			"one is configured); anything else is rejected as invalid. Emits a " +
			"record.status_changed event so automations can react.",
		Card: map[string]any{
			"input_summary":  "A record id and the status to move it to",
			"does":           "Validates the transition against the pipeline's value set and applies it",
			"output_summary": "The record with its new status",
			"why":            "A pipeline with enforced statuses stays trustworthy — no free-text stage soup",
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":               "string",
					"description":        "Id of the record to transition.",
					domain.SchemaKeyUnit: string(domain.UnitIDRef),
				},
				"to_status": map[string]any{
					"type":               "string",
					"description":        "Target status. Enum derives per tenant from the configured value set.",
					domain.SchemaKeyUnit: string(domain.UnitEnumValueSet),
				},
			},
			"required": []string{"id", "to_status"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"recordId": map[string]any{"type": "string", domain.SchemaKeyUnit: string(domain.UnitIDRef)},
				"status":   map[string]any{"type": "string"},
			},
		},
		Effects: domain.OperationEffects{
			Reads:  []string{"v5_entity_definitions", "v5_value_sets", "v5_entity_records"},
			Writes: []string{"v5_entity_records", "v5_events"},
			Emits:  []domain.EmitDecl{{EventType: domain.EventRecordStatusChanged, EntitySlug: ""}},
		},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"entity":      map[string]any{"type": "string", "description": "Entity slug the instance transitions."},
				"field":       map[string]any{"type": "string", "description": "Status field key (e.g. 'status')."},
				"value_set":   map[string]any{"type": "string", "description": "Value-set slug listing the allowed statuses (e.g. 'lead_pipeline')."},
				"transitions": map[string]any{"type": "object", "description": "Optional allowed-transition map {from: [to, ...]}. Absent = any status in the value set."},
			},
			"required": []string{"entity", "field", "value_set"},
		},
		Modes:       tenantModes(),
		Agent:       domain.AgentData,
		MinRole:     domain.RoleStaff,
		AutoEnabled: false,
	}
}

func scheduleSlotTemplate() domain.OperationTemplate {
	return domain.OperationTemplate{
		Name:  "schedule_slot",
		Kind:  domain.KindScheduleSlot,
		Title: "Book a time slot",
		Description: "Book a date and time against a catalog item by creating a " +
			"scheduling record — a property showing, a viewing, an appointment, a " +
			"call-back. Validates the requested datetime deterministically: rejects " +
			"past datetimes and times outside the configured business-hours window, " +
			"with a human-readable reason. Otherwise behaves like create_record and " +
			"emits record.created so automations notify staff. No overlap/inventory " +
			"check in v1 (R11). Typical instance: book_showing over entity 'lead'.",
		Card: map[string]any{
			"input_summary":  "Contact details plus a preferred date and time, linked to a listing",
			"does":           "Checks the datetime (no past, business hours only) and creates the scheduling record",
			"output_summary": "A confirmed booking record referencing the listing",
			"why":            "Converts a visitor's interest into a scheduled, trackable appointment the CRM can act on",
		},
		InputSchema: map[string]any{
			"type":        "object",
			"properties":  map[string]any{},
			"description": "Derived per tenant from the configured entity definition: the datetime field, the catalog link field and the allowed contact fields.",
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"recordId": map[string]any{"type": "string", domain.SchemaKeyUnit: string(domain.UnitIDRef)},
			},
		},
		Effects: domain.OperationEffects{
			Reads:  []string{"v5_entity_definitions", "v5_value_sets"},
			Writes: []string{"v5_entity_records", "v5_events"},
			Emits:  []domain.EmitDecl{{EventType: domain.EventRecordCreated, EntitySlug: ""}},
		},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"entity":         map[string]any{"type": "string", "description": "Entity slug the booking creates (e.g. 'lead')."},
				"datetime_field": map[string]any{"type": "string", "description": "Field key holding the requested datetime (e.g. 'preferredTime')."},
				"link_field":     map[string]any{"type": "string", "description": "Field key holding the catalog item id (e.g. 'listingId')."},
				"reject_past":    map[string]any{"type": "boolean", "description": "Reject datetimes in the past."},
				"hours": map[string]any{
					"type":        "object",
					"description": "Business-hours window.",
					"properties": map[string]any{
						"from": map[string]any{"type": "integer", "description": "Opening hour, 0-23."},
						"to":   map[string]any{"type": "integer", "description": "Closing hour, 0-23."},
					},
				},
				"defaults": map[string]any{"type": "object", "description": "Field values applied when the input omits them (e.g. {\"status\":\"new\"})."},
			},
			"required": []string{"entity", "datetime_field"},
		},
		Modes:       tenantModes(),
		Agent:       domain.AgentData,
		MinRole:     domain.RoleVisitor,
		AutoEnabled: false,
	}
}

func notifyTemplate() domain.OperationTemplate {
	return domain.OperationTemplate{
		Name:  "notify",
		Kind:  domain.KindNotify,
		Title: "Notify staff",
		Description: "Send a notification to the tenant's staff: writes a runtime " +
			"notification row (title + body, optionally linked to a record) for the " +
			"CRM audience, or posts to a configured webhook. Typically run by " +
			"automations when records are created or change status; {field} " +
			"placeholders in templates are substituted from the triggering event " +
			"payload. No email or SMS pretence in v1.",
		Card: map[string]any{
			"input_summary":  "A title and body, with {field} placeholders filled from the triggering event",
			"does":           "Writes a notification for the CRM audience or posts it to a webhook",
			"output_summary": "A stored notification visible to staff",
			"why":            "Closes the loop — staff learn about new leads and bookings the moment they happen",
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{"type": "string", "description": "Notification title."},
				"body":  map[string]any{"type": "string", "description": "Notification body."},
				"ref": map[string]any{
					"type":               "string",
					"description":        "Optional id of the record the notification is about.",
					domain.SchemaKeyUnit: string(domain.UnitIDRef),
				},
			},
			"required": []string{"title"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"notificationId": map[string]any{"type": "string", domain.SchemaKeyUnit: string(domain.UnitIDRef)},
			},
		},
		Effects: domain.OperationEffects{
			Reads:  []string{},
			Writes: []string{"v5_notifications"},
			Emits:  []domain.EmitDecl{},
		},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"channel": map[string]any{"type": "string", "enum": []string{"runtime", "webhook"}, "description": "Delivery channel; 'runtime' writes v5_notifications."},
				"url":     map[string]any{"type": "string", "description": "Webhook URL when channel is 'webhook'.", domain.SchemaKeyUnit: string(domain.UnitURL)},
			},
			"required": []string{"channel"},
		},
		Modes:       tenantModes(),
		Agent:       domain.AgentData,
		MinRole:     domain.RoleSystem, // automation-facing (OperationRunner runs with Role system)
		AutoEnabled: false,
	}
}

// ---------------------------------------------------------------------------
// compose_turn (§3.1: kind visual, modes onboarding+crm, auto_enabled —
// storefront keeps visual_assembly only, its prompt cache untouched).
// ---------------------------------------------------------------------------

func composeTurnTemplate() domain.OperationTemplate {
	return domain.OperationTemplate{
		Name:  "compose_turn",
		Kind:  domain.KindVisual,
		Title: "Compose a turn of text and interface blocks",
		Description: "Compose the assistant's answer as an ordered sequence of " +
			"blocks: text paragraphs interleaved with rendered documents — cards, " +
			"forms, tables, previews built from presets with real data bound. Each " +
			"block is text or a render; renders display inline in the chat column " +
			"or as the main screen. One call per turn, up to 8 blocks.",
		Card: map[string]any{
			"input_summary":  "An ordered list of up to 8 text or render blocks",
			"does":           "Renders each block — text verbatim, documents through the preset engine with live data",
			"output_summary": "A structured turn: prose interleaved with working interface",
			"why":            "One answer can explain AND show — no wall of text, no detached screens",
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"blocks": map[string]any{
					"type":        "array",
					"description": "Ordered blocks, max 8. kind 'text' uses text; kind 'render' uses preset/replicate/ops/display.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"kind":      map[string]any{"type": "string", "enum": []string{"text", "render"}},
							"text":      map[string]any{"type": "string", "description": "Paragraph text for kind 'text'."},
							"preset":    map[string]any{"type": "string", "description": "Preset name to render for kind 'render'."},
							"replicate": map[string]any{"type": "string", "description": "Optional replicate source for list-style presets."},
							"ops":       map[string]any{"type": "array", "description": "Optional scene-graph ops applied before binding."},
							"display":   map[string]any{"type": "string", "enum": []string{"inline", "screen"}, "description": "'inline' renders in chat; 'screen' becomes the main surface."},
						},
						"required": []string{"kind"},
					},
				},
			},
			"required": []string{"blocks"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"blockCount": map[string]any{"type": "integer", domain.SchemaKeyUnit: string(domain.UnitCount)},
			},
		},
		Effects: domain.OperationEffects{
			Reads:  []string{"v5_presets"},
			Writes: []string{"v5_chat_session_state"},
			Emits:  []domain.EmitDecl{},
		},
		ConfigSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		Modes:        []domain.PipelineMode{domain.ModeOnboarding, domain.ModeCRM},
		Agent:        domain.AgentVisual,
		MinRole:      domain.RoleVisitor,
		AutoEnabled:  true,
	}
}

// ---------------------------------------------------------------------------
// The 12 onboarding meta-operation templates (§4.3 + about_keepstar: kind
// meta, modes {onboarding}, min_role visitor — the onboarding cookie + form
// gate does the work, R14). about_keepstar and search_library are immediate;
// apply_manifest is control; the rest stage ManifestSteps — nothing mutates
// the world at stage time.
// ---------------------------------------------------------------------------

func metaTemplate(t domain.OperationTemplate) domain.OperationTemplate {
	t.Kind = domain.KindMeta
	t.Modes = onboardingModes()
	t.Agent = domain.AgentData
	t.MinRole = domain.RoleVisitor
	t.AutoEnabled = true // onboarding-form only
	if t.ConfigSchema == nil {
		t.ConfigSchema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return t
}

func aboutKeepstarTemplate() domain.OperationTemplate {
	return metaTemplate(domain.OperationTemplate{
		Name:  "about_keepstar",
		Title: "Explain what Keepstar is",
		Description: "Load the canonical explanation of the Keepstar interface " +
			"runtime — what it is (data + operations + interface), what a business " +
			"gets (a storefront and a CRM assembled by conversation), how assembly " +
			"and approval work, and what it is honest about. Immediate: the doc " +
			"content returns in the result so the agent can answer the visitor's " +
			"question in its own words and in the visitor's language. Use it " +
			"whenever the visitor is exploring or asking what this is, and again " +
			"for follow-up questions. Stages nothing.",
		Card: map[string]any{
			"input_summary":  "Optionally the visitor's question or topic of interest",
			"does":           "Returns the canonical Keepstar explanation for the agent to answer from",
			"output_summary": "The about-doc content, ready to be retold in the agent's own words",
			"why":            "Visitors who are just curious get a real, honest explanation — not a sales script or a guess",
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"topic": map[string]any{"type": "string", "description": "Optional: what the visitor asked about, in plain language."},
			},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content": map[string]any{"type": "string", "description": "The about-doc content."},
			},
		},
		Effects: domain.OperationEffects{Reads: []string{}, Writes: []string{}, Emits: []domain.EmitDecl{}},
	})
}

func searchLibraryTemplate() domain.OperationTemplate {
	return metaTemplate(domain.OperationTemplate{
		Name:  "search_library",
		Title: "Search the operation and preset library",
		Description: "Search the Keepstar library of operation templates and preset " +
			"packs by meaning. Returns matching templates with their cards (input, " +
			"what it does, output, why) ready to present to the business and stage " +
			"into the onboarding manifest. Immediate: results are available in the " +
			"same turn, in the manifest's library context and as an op-card set.",
		Card: map[string]any{
			"input_summary":  "A natural-language description of what the business needs",
			"does":           "Semantic search over the system operation and preset library",
			"output_summary": "Matching templates with presentation-ready cards",
			"why":            "The onboarding agent proposes from the library, never invents — every proposal is a real, executable capability",
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "What the business needs, in plain language."},
				"kind":  map[string]any{"type": "string", "enum": []string{"operation", "preset", "any"}, "description": "Restrict results to operations, preset packs, or both."},
				"limit": map[string]any{"type": "integer", "description": "Maximum results, up to 20.", domain.SchemaKeyUnit: string(domain.UnitCount)},
			},
			"required": []string{"query"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"count": map[string]any{"type": "integer", domain.SchemaKeyUnit: string(domain.UnitCount)},
			},
		},
		Effects: domain.OperationEffects{Reads: []string{"v5_operations"}, Writes: []string{}, Emits: []domain.EmitDecl{}},
	})
}

func createTenantTemplate() domain.OperationTemplate {
	return metaTemplate(domain.OperationTemplate{
		Name:  "create_tenant",
		Title: "Create the business workspace",
		Description: "Stage the creation of a new tenant workspace: a named " +
			"business with its own storefront, CRM, data and design. The vertical " +
			"is a free-form label describing the business (e.g. 'real estate " +
			"agency'), not an enum. Applied first when the manifest runs; the " +
			"applier also writes the brand-default theme (R22).",
		Card: map[string]any{
			"input_summary":  "The business name and what kind of business it is",
			"does":           "Provisions the tenant workspace and applies the default design tokens",
			"output_summary": "A live tenant with its slug, ready to receive data and operations",
			"why":            "Everything else — data, operations, surfaces — hangs off this workspace",
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":     map[string]any{"type": "string", "description": "Business display name."},
				"vertical": map[string]any{"type": "string", "description": "Free-form business label, e.g. 'real estate agency'."},
			},
			"required": []string{"name", "vertical"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"staged": map[string]any{"type": "boolean"},
				"stepId": map[string]any{"type": "string", domain.SchemaKeyUnit: string(domain.UnitIDRef)},
			},
		},
		Effects: domain.OperationEffects{Reads: []string{}, Writes: []string{"catalog.tenants", "theme"}, Emits: []domain.EmitDecl{}},
	})
}

func defineEntityTemplate() domain.OperationTemplate {
	return metaTemplate(domain.OperationTemplate{
		Name:  "define_entity",
		Title: "Define a data entity",
		Description: "Stage the definition of a runtime entity — the structured " +
			"records this business will collect, such as leads, requests or " +
			"bookings. An entity has a slug, a display name and an ordered list of " +
			"typed fields (text, number, money, bool, date, datetime, enum, phone, " +
			"email, ref). Field keys are camelCase; enum fields reference a value " +
			"set; a ref field links records to catalog items.",
		Card: map[string]any{
			"input_summary":  "An entity shape: slug, name and typed fields",
			"does":           "Registers the entity definition the CRM and operations will validate against",
			"output_summary": "A typed entity ready to hold records",
			"why":            "Structured, validated records — not free-text notes — make the CRM queryable and automatable",
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"entity": map[string]any{
					"type":        "object",
					"description": "The entity definition.",
					"properties": map[string]any{
						"slug":   map[string]any{"type": "string", "description": "Entity slug, e.g. 'lead'."},
						"name":   map[string]any{"type": "string", "description": "Display name."},
						"fields": map[string]any{"type": "array", "description": "Ordered FieldDef list: {key, label, type, unit?, valueSetRef?, refTarget?, required?, default?}."},
					},
				},
			},
			"required": []string{"entity"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"staged": map[string]any{"type": "boolean"},
				"stepId": map[string]any{"type": "string", domain.SchemaKeyUnit: string(domain.UnitIDRef)},
			},
		},
		Effects: domain.OperationEffects{Reads: []string{}, Writes: []string{"v5_entity_definitions"}, Emits: []domain.EmitDecl{}},
	})
}

func defineValueSetTemplate() domain.OperationTemplate {
	return metaTemplate(domain.OperationTemplate{
		Name:  "define_value_set",
		Title: "Define a value set",
		Description: "Stage a named, ordered list of allowed values — a pipeline of " +
			"lead statuses, a set of request types. Each entry is {value, label, " +
			"color?}, ordered by array position (R27). Enum fields and status " +
			"transitions validate against it.",
		Card: map[string]any{
			"input_summary":  "A slug, a name and the ordered values with labels",
			"does":           "Registers the closed value list enums and pipelines validate against",
			"output_summary": "A value set entities and operations can reference",
			"why":            "Closed vocabularies keep statuses and categories consistent across chat, CRM and automations",
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"slug":   map[string]any{"type": "string", "description": "Value-set slug, e.g. 'lead_pipeline'."},
				"name":   map[string]any{"type": "string", "description": "Display name."},
				"values": map[string]any{"type": "array", "description": "Ordered entries {value, label, color?}."},
			},
			"required": []string{"slug", "name", "values"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"staged": map[string]any{"type": "boolean"},
				"stepId": map[string]any{"type": "string", domain.SchemaKeyUnit: string(domain.UnitIDRef)},
			},
		},
		Effects: domain.OperationEffects{Reads: []string{}, Writes: []string{"v5_value_sets"}, Emits: []domain.EmitDecl{}},
	})
}

func defineAutomationTemplate() domain.OperationTemplate {
	return metaTemplate(domain.OperationTemplate{
		Name:  "define_automation",
		Title: "Define an automation",
		Description: "Stage a flat automation rule: when an event happens " +
			"(record.created or record.status_changed on an entity), optionally " +
			"check one predicate, then run the notify operation with templated " +
			"parameters. The operation is fixed to notify in v1 — no chains, no " +
			"schedules.",
		Card: map[string]any{
			"input_summary":  "An event, an optional condition and the notification to send",
			"does":           "Wires an event to a staff notification, evaluated deterministically",
			"output_summary": "An automation that fires the moment the event commits",
			"why":            "New leads and bookings reach staff without anyone polling a dashboard",
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":       map[string]any{"type": "string", "description": "Automation name."},
				"event_type": map[string]any{"type": "string", "enum": []string{"record.created", "record.status_changed"}, "description": "Triggering event."},
				"entity":     map[string]any{"type": "string", "description": "Entity slug the rule watches; omit for any."},
				"predicate": map[string]any{
					"type":        "object",
					"description": "Optional single condition.",
					"properties": map[string]any{
						"field": map[string]any{"type": "string"},
						"op":    map[string]any{"type": "string", "enum": []string{"eq", "neq", "in"}},
						"value": map[string]any{"description": "Comparison value (string, number or array for 'in')."},
					},
				},
				"params": map[string]any{"type": "object", "description": "notify parameters; {field} placeholders substitute from the event payload."},
			},
			"required": []string{"name", "event_type", "params"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"staged": map[string]any{"type": "boolean"},
				"stepId": map[string]any{"type": "string", domain.SchemaKeyUnit: string(domain.UnitIDRef)},
			},
		},
		Effects: domain.OperationEffects{Reads: []string{}, Writes: []string{"v5_automations"}, Emits: []domain.EmitDecl{}},
	})
}

func enableOperationsTemplate() domain.OperationTemplate {
	return metaTemplate(domain.OperationTemplate{
		Name:  "enable_operations",
		Title: "Enable operations for the business",
		Description: "Stage enabling library operation templates for the new " +
			"tenant, each under an instance name with its configuration — e.g. " +
			"book_showing over schedule_slot, lead_search over query, advance_lead " +
			"over transition_status, notify_agent over notify. Config is validated " +
			"against the template's config schema when applied.",
		Card: map[string]any{
			"input_summary":  "A list of template + instance name + config triples",
			"does":           "Turns library templates into this tenant's named, configured operations",
			"output_summary": "Enabled operations the storefront and CRM agents can call",
			"why":            "The business gets exactly the capabilities it needs — configured, not coded",
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"operations": map[string]any{
					"type":        "array",
					"description": "Operations to enable.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"template": map[string]any{"type": "string", "description": "Library template name, e.g. 'schedule_slot'."},
							"instance": map[string]any{"type": "string", "description": "Instance wire name, e.g. 'book_showing'."},
							"config":   map[string]any{"type": "object", "description": "Instance config, validated against the template's config schema."},
						},
						"required": []string{"template", "instance"},
					},
				},
			},
			"required": []string{"operations"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"staged": map[string]any{"type": "boolean"},
				"stepId": map[string]any{"type": "string", domain.SchemaKeyUnit: string(domain.UnitIDRef)},
			},
		},
		Effects: domain.OperationEffects{Reads: []string{"v5_operations"}, Writes: []string{"v5_tenant_operations"}, Emits: []domain.EmitDecl{}},
	})
}

func adoptPresetsTemplate() domain.OperationTemplate {
	return metaTemplate(domain.OperationTemplate{
		Name:  "adopt_presets",
		Title: "Adopt interface presets",
		Description: "Stage adopting system presets — listing cards, detail views, " +
			"booking forms, lead tables — for the new tenant. When applied, the " +
			"presets are registered for the tenant and every required data binding " +
			"is verified against real data; unresolved bindings fail the step " +
			"loudly rather than rendering broken UI.",
		Card: map[string]any{
			"input_summary":  "Preset names to adopt from the system library",
			"does":           "Registers the presets for the tenant and verifies their data bindings",
			"output_summary": "A working interface kit for storefront and CRM",
			"why":            "Proven, binding-verified interface pieces — not one-off generated screens",
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"presets": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Preset names to adopt."},
				"source":  map[string]any{"type": "string", "enum": []string{"system"}, "description": "Preset source; 'system' only in v1."},
			},
			"required": []string{"presets"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"staged": map[string]any{"type": "boolean"},
				"stepId": map[string]any{"type": "string", domain.SchemaKeyUnit: string(domain.UnitIDRef)},
			},
		},
		Effects: domain.OperationEffects{Reads: []string{"v5_presets"}, Writes: []string{"v5_presets"}, Emits: []domain.EmitDecl{}},
	})
}

func issueIngestDoorTemplate() domain.OperationTemplate {
	return metaTemplate(domain.OperationTemplate{
		Name:  "issue_ingest_door",
		Title: "Open the data upload door",
		Description: "Stage issuing a one-time, session-bound upload token for the " +
			"universal data uploader. The business uploads its records — CSV with a " +
			"header row, or a JSON array of flat objects — and the import maps " +
			"columns, preserves unmapped attributes and rebuilds the search " +
			"projection so the data is immediately searchable.",
		Card: map[string]any{
			"input_summary":  "The accepted file formats (csv, json)",
			"does":           "Mints a scoped upload token and arms the uploader once the manifest applies",
			"output_summary": "An armed uploader that ingests the business's real data",
			"why":            "The business's own catalog powers the storefront from minute one — no sample data",
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"formats": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"csv", "json"}}, "description": "Accepted upload formats."},
			},
			"required": []string{"formats"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"staged": map[string]any{"type": "boolean"},
				"stepId": map[string]any{"type": "string", domain.SchemaKeyUnit: string(domain.UnitIDRef)},
			},
		},
		Effects: domain.OperationEffects{Reads: []string{}, Writes: []string{"v5_ingest_tokens"}, Emits: []domain.EmitDecl{}},
	})
}

func registerUserTemplate() domain.OperationTemplate {
	return metaTemplate(domain.OperationTemplate{
		Name:  "register_user",
		Title: "Register the owner account",
		Description: "Stage the owner-account registration step. The step renders " +
			"a registration form in chat; the email and password submit DIRECTLY to " +
			"the step-submit endpoint and NEVER pass through the language model, " +
			"the operation registry, session state or audit logs (R6). The staged " +
			"step completes when the form is submitted.",
		Card: map[string]any{
			"input_summary":  "The account role to provision (owner)",
			"does":           "Stages a registration step completed by a direct, credential-safe form submit",
			"output_summary": "An admin account for the business owner",
			"why":            "The owner gets real console access — with credentials that never touch the AI path",
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"role": map[string]any{"type": "string", "enum": []string{"owner"}, "description": "Account role; 'owner' only in v1."},
			},
			"required": []string{"role"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"staged": map[string]any{"type": "boolean"},
				"stepId": map[string]any{"type": "string", domain.SchemaKeyUnit: string(domain.UnitIDRef)},
			},
		},
		Effects: domain.OperationEffects{Reads: []string{}, Writes: []string{"admin.admin_users"}, Emits: []domain.EmitDecl{}},
	})
}

func issueSurfaceURLsTemplate() domain.OperationTemplate {
	return metaTemplate(domain.OperationTemplate{
		Name:  "issue_surface_urls",
		Title: "Issue the storefront and CRM URLs",
		Description: "Stage the final step: mint the CRM surface token and print " +
			"the live URLs — the public storefront and the token-gated CRM. " +
			"Refuses until every other manifest step has applied, so the URLs " +
			"always point at a fully assembled workspace.",
		Card: map[string]any{
			"input_summary":  "Nothing — it runs last, over the applied manifest",
			"does":           "Mints the CRM access token and resolves both surface URLs",
			"output_summary": "A public storefront URL and a staff CRM URL",
			"why":            "The handover moment: the business walks away with working addresses, not promises",
		},
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"storefrontUrl": map[string]any{"type": "string", domain.SchemaKeyUnit: string(domain.UnitURL)},
				"crmUrl":        map[string]any{"type": "string", domain.SchemaKeyUnit: string(domain.UnitURL)},
			},
		},
		Effects: domain.OperationEffects{Reads: []string{}, Writes: []string{"v5_surface_tokens"}, Emits: []domain.EmitDecl{}},
	})
}

func applyManifestTemplate() domain.OperationTemplate {
	return metaTemplate(domain.OperationTemplate{
		Name:  "apply_manifest",
		Title: "Apply the onboarding manifest",
		Description: "Control operation: flip the staged manifest steps from " +
			"proposed to accepted and run the deterministic applier — tenant " +
			"first, then entities, value sets, automations, operations, presets, " +
			"upload door and registration. Idempotent per step; a failed step " +
			"halts and a retry re-runs from it. Returns a per-step digest.",
		Card: map[string]any{
			"input_summary":  "Optionally a step id to apply up to",
			"does":           "Executes every staged step in order through the deterministic applier",
			"output_summary": "A per-step result digest: applied, failed or waiting",
			"why":            "The plan the business approved is exactly what gets built — no drift between proposal and execution",
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"upTo": map[string]any{"type": "string", "description": "Optional step id: apply steps up to and including it.", domain.SchemaKeyUnit: string(domain.UnitIDRef)},
			},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"applied": map[string]any{"type": "integer", domain.SchemaKeyUnit: string(domain.UnitCount)},
				"total":   map[string]any{"type": "integer", domain.SchemaKeyUnit: string(domain.UnitCount)},
			},
		},
		Effects: domain.OperationEffects{
			Reads: []string{},
			// The applier fans out to the staged steps' targets; see each
			// staged template's Writes for the per-step effect.
			Writes: []string{"catalog.tenants", "v5_entity_definitions", "v5_value_sets", "v5_automations", "v5_tenant_operations", "v5_presets", "v5_ingest_tokens", "v5_surface_tokens"},
			Emits:  []domain.EmitDecl{},
		},
	})
}
