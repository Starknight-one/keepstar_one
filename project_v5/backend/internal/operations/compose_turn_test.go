package operations

// Tests for the compose_turn wrap: the executor must register under the
// §3.1 seed row (single source of truth for the LLM-facing bytes), the
// tool's own Definition must stay byte-equal to that row (drift guard),
// and the registry must gate it to the onboarding/crm forms on the visual
// plane — storefront keeps visual_assembly only.

import (
	"context"
	"log/slog"
	"reflect"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/tools"
)

func TestWrapComposeTurnUsesSeedRow(t *testing.T) {
	seedRow, ok := seedTemplateByName("compose_turn")
	if !ok {
		t.Fatal("seed.Templates() lost the compose_turn row — §3.1 seed table broken")
	}
	ex := WrapComposeTurn(tools.NewComposeTurnTool(nil, nil, nil))

	row := ex.TemplateRow()
	if !reflect.DeepEqual(row, seedRow) {
		t.Errorf("TemplateRow diverged from the seed row:\n got %+v\nwant %+v", row, seedRow)
	}
	if row.Kind != domain.KindVisual || row.Agent != domain.AgentVisual {
		t.Errorf("kind/agent = %s/%s, want visual/visual", row.Kind, row.Agent)
	}
	if !row.AutoEnabled {
		t.Error("compose_turn must be auto_enabled (§3.1)")
	}
	wantModes := []domain.PipelineMode{domain.ModeOnboarding, domain.ModeCRM}
	if !reflect.DeepEqual(row.Modes, wantModes) {
		t.Errorf("modes = %v, want %v", row.Modes, wantModes)
	}
	if !ex.Passthrough() {
		t.Error("compose_turn wrap must be passthrough — the tool owns its validation")
	}
}

// TestComposeTurnDefinitionMatchesSeedSchema — the tool validates what the
// seeded schema promises: name, description and input schema must stay
// byte-equal or the LLM-facing contract and the executor drift silently.
func TestComposeTurnDefinitionMatchesSeedSchema(t *testing.T) {
	seedRow, ok := seedTemplateByName("compose_turn")
	if !ok {
		t.Fatal("seed.Templates() lost the compose_turn row")
	}
	def := tools.NewComposeTurnTool(nil, nil, nil).Definition()

	if def.Name != seedRow.Name {
		t.Errorf("name = %q, want %q", def.Name, seedRow.Name)
	}
	if def.Description != seedRow.Description {
		t.Errorf("description drifted:\n tool %q\n seed %q", def.Description, seedRow.Description)
	}
	if !reflect.DeepEqual(def.InputSchema, seedRow.InputSchema) {
		t.Errorf("input schema drifted:\n tool %+v\n seed %+v", def.InputSchema, seedRow.InputSchema)
	}
}

// TestComposeTurnVisibilityPerForm — DefinitionsFor must expose
// compose_turn on the visual plane of the onboarding and crm forms and
// NEVER on storefront (its prompt cache stays untouched, §3.1).
func TestComposeTurnVisibilityPerForm(t *testing.T) {
	reg := NewRegistry(RegistryConfig{Log: slog.Default()})
	reg.RegisterExecutor(domain.KindVisual, WrapComposeTurn(tools.NewComposeTurnTool(nil, nil, nil)))

	has := func(mode domain.PipelineMode, agent domain.AgentPlane) bool {
		for _, def := range reg.DefinitionsFor(context.Background(), "", mode, agent, domain.RoleVisitor) {
			if def.Name == "compose_turn" {
				return true
			}
		}
		return false
	}
	if !has(domain.ModeOnboarding, domain.AgentVisual) {
		t.Error("compose_turn missing on onboarding/visual")
	}
	if !has(domain.ModeCRM, domain.AgentVisual) {
		t.Error("compose_turn missing on crm/visual")
	}
	if has(domain.ModeStorefront, domain.AgentVisual) {
		t.Error("compose_turn leaked into the storefront form — prompt-cache stability broken")
	}
	if has(domain.ModeOnboarding, domain.AgentData) {
		t.Error("compose_turn leaked onto the data plane")
	}
}
