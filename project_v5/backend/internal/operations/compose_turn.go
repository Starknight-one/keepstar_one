package operations

import (
	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/operations/seed"
	"keepstar_v5/internal/tools"
)

// WrapComposeTurn registers the compose_turn tool (§4.7) under its §3.1
// seed row — seed.Templates() is the single source of truth for the
// LLM-facing bytes (name, description, input schema, modes onboarding+crm,
// auto_enabled; storefront keeps visual_assembly only, its prompt cache
// untouched).
//
// Passthrough like the legacy wraps: registry input validation is skipped
// — the tool owns its own contract (max 8 blocks, one call per turn →
// IsError; the restricted schema dialect can't express the nested block
// array anyway). The executor implements templateRowProvider, so the boot
// seeder upserts the SAME row it would seed from the extras list — the
// double upsert is byte-identical and idempotent.
func WrapComposeTurn(t tools.Tool) *LegacyExecutor {
	if tmpl, ok := seedTemplateByName("compose_turn"); ok {
		return WrapLegacyTool(t, tmpl)
	}
	// Unreachable while seed.Templates() carries compose_turn; keep a
	// deterministic fallback built from the tool + §3.1 constants so a
	// future seed refactor cannot silently register a schema-less row.
	def := t.Definition()
	return WrapLegacyTool(t, domain.OperationTemplate{
		Name:        def.Name,
		Kind:        domain.KindVisual,
		Title:       "Compose a turn of text and interface blocks",
		Description: def.Description,
		InputSchema: def.InputSchema,
		Effects: domain.OperationEffects{
			Reads:  []string{"v5_presets"},
			Writes: []string{"v5_chat_session_state"},
			Emits:  []domain.EmitDecl{},
		},
		Modes:       []domain.PipelineMode{domain.ModeOnboarding, domain.ModeCRM},
		Agent:       domain.AgentVisual,
		MinRole:     domain.RoleVisitor,
		AutoEnabled: true,
	})
}

// seedTemplateByName finds one boot-seed row by wire name.
func seedTemplateByName(name string) (domain.OperationTemplate, bool) {
	for _, tmpl := range seed.Templates() {
		if tmpl.Name == name {
			return tmpl, true
		}
	}
	return domain.OperationTemplate{}, false
}
