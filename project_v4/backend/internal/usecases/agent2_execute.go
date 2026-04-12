package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"keepstar_v4/internal/domain"
	engine_v4 "keepstar_v4/internal/engine_v4"
	"keepstar_v4/internal/logger"
	"keepstar_v4/internal/ports"
	"keepstar_v4/internal/prompts"
	"keepstar_v4/internal/tools"
)

// Agent2ExecuteRequest is the input for Agent 2
type Agent2ExecuteRequest struct {
	SessionID     string
	TurnID        string         // Turn ID for delta grouping
	TenantSlug    string         // Tenant context (for field definitions lookup)
	UserQuery     string         // User's original query (for style selection)
	Microcontext  string         // Pipeline-generated context signal (e.g. "new_search: 23 items found")
	ScreenContext *ScreenContext  // Current UI state from frontend
}

// Agent2ExecuteResponse is the output from Agent 2
type Agent2ExecuteResponse struct {
	Template   *domain.FormationTemplate
	Formation  *domain.FormationWithData // New: formation built by tool
	Usage      domain.LLMUsage
	LatencyMs  int
	ToolCalled bool   // Whether a tool was called
	ToolName   string // Name of the tool called
	// Detailed timing and data
	LLMCallMs    int64    `json:"llmCallMs"`
	PromptSent   string   `json:"promptSent"`
	RawResponse  string   `json:"rawResponse"`
	TemplateJSON string   `json:"templateJson"`
	MetaCount    int      `json:"metaCount"`
	MetaFields   []string `json:"metaFields"`
	// Trace enrichment
	SystemPrompt      string                 `json:"systemPrompt,omitempty"`
	SystemPromptChars int                    `json:"systemPromptChars,omitempty"`
	ToolInput         string                 `json:"toolInput,omitempty"`
	ToolBreakdown     map[string]interface{} `json:"toolBreakdown,omitempty"`
	MessageCount      int                    `json:"messageCount,omitempty"`
	ToolDefCount      int                    `json:"toolDefCount,omitempty"`
}

// promptCacheEntry is one cached system prompt with its design context
// version. When the version counter bumps (admin publish), the next
// buildSystemPrompt call rebuilds and .Store replaces this entry.
type promptCacheEntry struct {
	version int64
	prompt  string
}

// Agent2ExecuteUseCase executes Agent 2 (Preset Selector)
type Agent2ExecuteUseCase struct {
	llm                 ports.LLMPort
	statePort           ports.StatePort
	toolRegistry        *tools.Registry
	log                 *logger.Logger
	fieldDefPort        ports.FieldDefinitionPort        // field definitions
	designContextLoader *engine_v4.TenantPresetLoader    // Phase 3: design context

	// promptCache memoizes the per-tenant system prompt (base + <fields> +
	// <tenant_design_context>). Keyed by tenantSlug, value is
	// *promptCacheEntry. Version-aware: a design context version bump causes
	// a rebuild on the next Agent2 turn. Byte-stable output is load-bearing
	// for Anthropic prompt caching.
	promptCache sync.Map // key: tenantSlug (string) → value: *promptCacheEntry
}

// NewAgent2ExecuteUseCase creates Agent 2 use case with field definitions and
// design context support. designContextLoader may be nil — the usecase then
// skips the <tenant_design_context> block (boot-without-DB fallback).
func NewAgent2ExecuteUseCase(
	llm ports.LLMPort,
	statePort ports.StatePort,
	toolRegistry *tools.Registry,
	log *logger.Logger,
	fieldDefPort ports.FieldDefinitionPort,
	designContextLoader *engine_v4.TenantPresetLoader,
) *Agent2ExecuteUseCase {
	return &Agent2ExecuteUseCase{
		llm:                 llm,
		statePort:           statePort,
		toolRegistry:        toolRegistry,
		log:                 log,
		fieldDefPort:        fieldDefPort,
		designContextLoader: designContextLoader,
	}
}

// Execute runs Agent 2: meta → LLM (tools) → render tool → formation in state
func (uc *Agent2ExecuteUseCase) Execute(ctx context.Context, req Agent2ExecuteRequest) (*Agent2ExecuteResponse, error) {
	start := time.Now()

	// Span instrumentation
	sc := domain.SpanFromContext(ctx)
	if sc != nil {
		endAgent := sc.Start("agent2")
		defer endAgent()
	}
	ctx = domain.WithStage(ctx, "agent2")

	// Get current state (must exist after Agent 1)
	state, err := uc.statePort.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// Update meta counts
	state.Current.Meta.ProductCount = len(state.Current.Data.Products)
	state.Current.Meta.ServiceCount = len(state.Current.Data.Services)

	// Check if we have data — no data means nothing to render
	if state.Current.Meta.ProductCount == 0 && state.Current.Meta.ServiceCount == 0 {
		return &Agent2ExecuteResponse{
			Formation: &domain.FormationWithData{
				Mode: domain.FormationTypeSingle,
				Widgets: []domain.Widget{{
					ID: "no-results",
					Atoms: []domain.Atom{
						{
							Type:    domain.AtomTypeText,
							Subtype: domain.SubtypeString,
							Slot:    domain.AtomSlotTitle,
							Value:   "Ничего не найдено",
							TextStyle: &domain.TextStyle{
								FontSize:   "lg",
								FontWeight: "semibold",
							},
						},
						{
							Type:    domain.AtomTypeText,
							Subtype: domain.SubtypeString,
							Slot:    domain.AtomSlotSecondary,
							Value:   "Попробуйте изменить запрос или уточнить категорию",
							TextStyle: &domain.TextStyle{
								FontSize: "sm",
							},
						},
						{
							Type:    domain.AtomTypeText,
							Subtype: domain.SubtypeString,
							Slot:    domain.AtomSlotSecondary,
							Value:   "Показать все товары",
							Wrapper: &domain.WrapperConfig{
								Type:    "tag",
								Variant: "default",
							},
							Meta: map[string]interface{}{"action": "show_all"},
						},
					},
				}},
			},
			LatencyMs: int(time.Since(start).Milliseconds()),
		}, nil
	}

	// Get data delta for current turn (for Agent2 context)
	var dataDelta *domain.Delta
	if req.TurnID != "" {
		deltas, _ := uc.statePort.GetDeltasSince(ctx, req.SessionID, 0)
		for i := len(deltas) - 1; i >= 0; i-- {
			if deltas[i].TurnID == req.TurnID && strings.HasPrefix(deltas[i].Path, "data.") {
				dataDelta = &deltas[i]
				break
			}
		}
	}

	// Extract current RenderConfig and formation tree from state (what is on screen now)
	var currentConfig *domain.RenderConfig
	var formationTree map[string]interface{}
	if state.Current.Template != nil {
		if formationData, ok := state.Current.Template["formation"]; ok {
			var f *domain.FormationWithData
			if typed, ok := formationData.(*domain.FormationWithData); ok {
				f = typed
			} else {
				// After DB roundtrip: map[string]interface{} → convert via JSON
				f = convertToFormation(formationData)
			}
			if f != nil {
				if f.Config != nil {
					currentConfig = f.Config
				}
				formationTree = engine_v4.BuildTreeMap(f)
			}
		}
	}

	// Load all deltas for history summary
	allDeltas, _ := uc.statePort.GetDeltas(ctx, req.SessionID)

	// Build screen context for prompt
	var screenCtx *prompts.ScreenContext
	if req.ScreenContext != nil {
		screenCtx = &prompts.ScreenContext{
			Mode:        req.ScreenContext.Mode,
			WidgetCount: req.ScreenContext.WidgetCount,
			Fields:      req.ScreenContext.Fields,
		}
	}

	// Build user message and system prompt. The system prompt now includes
	// a per-tenant <fields> block so Agent2 can match atom slots against
	// the tenant's actual data fields (metadata-driven binding, see
	// docs/New features/METADATA_DRIVEN_BINDING_2026-04-09.md).
	systemPrompt := uc.buildSystemPrompt(ctx, req.TenantSlug)
	// Load field labels from field_definitions for context
	fieldLabels := uc.loadFieldLabels(ctx, req.TenantSlug, state)
	userPrompt := prompts.BuildAgent2ToolPrompt(state.Current.Meta, state.View, req.UserQuery, dataDelta, currentConfig, allDeltas, req.Microcontext, screenCtx, fieldLabels, formationTree)

	// Agent2's own tool call history (last 2 turns = 4 messages: assistant:tool_use + user:tool_result × 2).
	// This replaces the old approach of including Agent1's text user messages which caused stale parameters.
	var messages []domain.LLMMessage
	if len(state.Agent2History) > 0 {
		historyLimit := 4 // 2 turns × 2 messages (assistant:tool_use + user:tool_result)
		start := len(state.Agent2History) - historyLimit
		if start < 0 {
			start = 0
		}
		messages = append(messages, state.Agent2History[start:]...)
	}
	messages = append(messages, domain.LLMMessage{
		Role:    "user",
		Content: userPrompt,
	})

	// Get render tool definitions (filter only render_* tools)
	toolDefs := uc.getAgent2Tools()

	// Call LLM with caching and forced tool use
	llmStart := time.Now()
	llmResp, err := uc.llm.ChatWithToolsCached(
		ctx,
		systemPrompt,
		messages,
		toolDefs,
		&ports.CacheConfig{
			CacheTools:        true,
			CacheSystem:       true,
			CacheConversation: len(messages) > 1, // also cache Agent2History when present
			ToolChoice:        "any",              // Force tool call — Agent2 must always render
		},
	)
	llmDuration := time.Since(llmStart).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("llm call: %w", err)
	}

	// Log LLM usage with cache metrics
	uc.log.LLMUsageWithCache(
		"agent2",
		llmResp.Usage.Model,
		llmResp.Usage.InputTokens,
		llmResp.Usage.OutputTokens,
		llmResp.Usage.CacheCreationInputTokens,
		llmResp.Usage.CacheReadInputTokens,
		llmResp.Usage.CostUSD,
		llmDuration,
	)

	response := &Agent2ExecuteResponse{
		Usage:             llmResp.Usage,
		LatencyMs:         int(time.Since(start).Milliseconds()),
		LLMCallMs:         llmDuration,
		PromptSent:        userPrompt,
		MetaCount:         state.Current.Meta.ProductCount + state.Current.Meta.ServiceCount,
		MetaFields:        state.Current.Meta.Fields,
		SystemPrompt:      systemPrompt,
		SystemPromptChars: len(systemPrompt),
		MessageCount:      len(messages),
		ToolDefCount:      len(toolDefs),
	}

	// Execute tool calls — tools create deltas via zone-write internally
	for _, toolCall := range llmResp.ToolCalls {
		response.ToolCalled = true
		response.ToolName = toolCall.Name

		// Capture tool input for tracing
		if inputJSON, err := json.Marshal(toolCall.Input); err == nil {
			response.ToolInput = string(inputJSON)
		}

		uc.log.Debug("tool_call_received",
			"tool", toolCall.Name,
			"input", toolCall.Input,
			"session_id", req.SessionID,
			"actor", "agent2",
		)

		var endToolSpan func(...string)
		if sc != nil {
			endToolSpan = sc.Start("agent2.tool")
		}
		toolStart := time.Now()
		result, err := uc.toolRegistry.Execute(ctx, tools.ToolContext{
			SessionID:  req.SessionID,
			TurnID:     req.TurnID,
			ActorID:    "agent2",
			TenantSlug: req.TenantSlug,
			UserQuery:  req.UserQuery,
		}, toolCall)
		toolDuration := time.Since(toolStart).Milliseconds()
		if endToolSpan != nil {
			endToolSpan(toolCall.Name)
		}

		if err != nil {
			uc.log.Error("tool_execution_failed", "error", err, "tool", toolCall.Name, "actor", "agent2")
			// Graceful degradation: retry with no parameters
			fallbackResult, fallbackErr := uc.toolRegistry.Execute(ctx, tools.ToolContext{
				SessionID:  req.SessionID,
				TurnID:     req.TurnID,
				ActorID:    "agent2",
				TenantSlug: req.TenantSlug,
				UserQuery:  req.UserQuery,
			}, domain.ToolCall{Name: "visual_assembly", Input: map[string]interface{}{}})
			if fallbackErr != nil {
				return nil, fmt.Errorf("execute tool %s (fallback also failed): %w", toolCall.Name, err)
			}
			result = fallbackResult
			uc.log.Info("graceful_degradation", "session_id", req.SessionID, "original_error", err.Error())
		}

		uc.log.ToolExecuted(toolCall.Name, req.SessionID, result.Content, toolDuration)

		response.RawResponse = result.Content
		if result.Metadata != nil {
			response.ToolBreakdown = result.Metadata
		}

		// Tool writes formation to state
		if result.IsError {
			return nil, fmt.Errorf("tool error: %s", result.Content)
		}

		// Save Agent2's tool call + result for multi-turn history
		agent2Messages := append(state.Agent2History,
			domain.LLMMessage{Role: "assistant", ToolCalls: llmResp.ToolCalls},
			domain.LLMMessage{
				Role: "user",
				ToolResult: &domain.ToolResult{
					ToolUseID: toolCall.ID,
					Content:   result.Content,
				},
			},
		)
		if err := uc.statePort.AppendAgent2History(ctx, req.SessionID, agent2Messages); err != nil {
			uc.log.Error("append_agent2_history_failed", "error", err)
		}
	}

	// Get formation from state (built by tool)
	state, err = uc.statePort.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state after tool: %w", err)
	}

	if formationData, ok := state.Current.Template["formation"]; ok {
		// Try direct type assertion first (in-memory)
		if formation, ok := formationData.(*domain.FormationWithData); ok {
			response.Formation = formation
		} else {
			// After DB read, it's map[string]interface{} - convert via JSON
			response.Formation = convertToFormation(formationData)
		}
	}

	return response, nil
}

// getAgent2Tools returns visual_* tools for Agent 2
func (uc *Agent2ExecuteUseCase) getAgent2Tools() []domain.ToolDefinition {
	allTools := uc.toolRegistry.GetDefinitions()
	var agent2Tools []domain.ToolDefinition
	for _, t := range allTools {
		if strings.HasPrefix(t.Name, "visual_") {
			agent2Tools = append(agent2Tools, t)
		}
	}
	return agent2Tools
}

// loadFieldLabels loads field name → label mapping from field_definitions for prompt context.
func (uc *Agent2ExecuteUseCase) loadFieldLabels(ctx context.Context, tenantSlug string, state *domain.SessionState) map[string]string {
	if uc.fieldDefPort == nil || tenantSlug == "" {
		return nil
	}

	labels := make(map[string]string)

	// Load product field labels
	if state.Current.Meta.ProductCount > 0 {
		defs, err := uc.fieldDefPort.ListFieldDefinitions(ctx, tenantSlug, domain.EntityTypeProduct)
		if err != nil {
			uc.log.Warn("failed to load product field definitions for prompt", "error", err)
		} else {
			for _, d := range defs {
				if d.Label != "" {
					labels[d.FieldName] = d.Label
				}
			}
		}
	}

	// Load service field labels
	if state.Current.Meta.ServiceCount > 0 {
		defs, err := uc.fieldDefPort.ListFieldDefinitions(ctx, tenantSlug, domain.EntityTypeService)
		if err != nil {
			uc.log.Warn("failed to load service field definitions for prompt", "error", err)
		} else {
			for _, d := range defs {
				if d.Label != "" {
					labels[d.FieldName] = d.Label
				}
			}
		}
	}

	if len(labels) == 0 {
		return nil
	}
	return labels
}

// buildSystemPrompt returns the fully-assembled Agent2 system prompt:
//
//	base prompt + <fields entity="product"> block + <tenant_design_context> block
//
// Memoized per (tenantSlug, designContextVersion). A version bump on the
// admin side causes the next read to rebuild and .Store under the same key.
// Byte-stable output is load-bearing for Anthropic prompt caching.
//
// Graceful degradation:
//   - empty tenantSlug → base prompt unchanged
//   - nil fieldDefPort → skip <fields> block
//   - nil designContextLoader → skip <tenant_design_context> block
//   - per-section errors → skip that section, keep others
func (uc *Agent2ExecuteUseCase) buildSystemPrompt(ctx context.Context, tenantSlug string) string {
	if tenantSlug == "" {
		return prompts.Agent2ToolSystemPrompt
	}

	// Phase 3: fetch design context snapshot (version-aware).
	var snapshot *engine_v4.DesignContextSnapshot
	var version int64
	if uc.designContextLoader != nil {
		snapshot = uc.designContextLoader.LoadDesignContext(ctx, tenantSlug)
		if snapshot != nil {
			version = snapshot.Version
		}
	}

	// Cache check — reuse if version hasn't changed.
	if cached, ok := uc.promptCache.Load(tenantSlug); ok {
		if entry := cached.(*promptCacheEntry); entry.version == version {
			return entry.prompt
		}
	}

	// Rebuild the prompt.
	var b strings.Builder
	b.WriteString(prompts.Agent2ToolSystemPrompt)

	// <fields> block (Phase 2 — unchanged logic, just moved here).
	if uc.fieldDefPort != nil {
		fields, err := uc.fieldDefPort.ListFieldDefinitions(ctx, tenantSlug, domain.EntityTypeProduct)
		if err == nil && len(fields) > 0 {
			samples, _ := uc.fieldDefPort.SampleFieldValues(ctx, tenantSlug, domain.EntityTypeProduct, 3)
			if block := formatFieldsBlock(fields, samples); block != "" {
				b.WriteString("\n\n")
				b.WriteString(block)
				b.WriteByte('\n')
			}
		}
	}

	// <tenant_design_context> block (Phase 3).
	if snapshot != nil {
		if block := formatDesignContextBlock(snapshot); block != "" {
			b.WriteString("\n\n")
			b.WriteString(block)
			b.WriteByte('\n')
		}
	}

	full := b.String()
	uc.promptCache.Store(tenantSlug, &promptCacheEntry{version: version, prompt: full})
	return full
}

// formatFieldsBlock renders a compact <fields entity="product">...</fields>
// text block for injection into Agent2's system prompt. One line per field,
// ordered by priority ASC (most important first).
//
// Example output:
//
//	<fields entity="product">
//	images         image   label="Images"         samples=["https://..."]
//	name           text    label="Name"           samples=["COSRX BHA Power","Laneige Moisture"]
//	price          number  label="Price"          samples=[2490,3990,1200]
//	brand          text    label="Brand"          samples=["COSRX","Laneige"]
//	...
//	</fields>
//
// Fields without samples still appear (label+type are informative alone).
// Fields with samples get them JSON-encoded so the LLM sees the data shape.
func formatFieldsBlock(fields []ports.FieldDefinition, samples map[string][]interface{}) string {
	if len(fields) == 0 {
		return ""
	}
	// Copy + sort by priority (lower = more important)
	sorted := make([]ports.FieldDefinition, len(fields))
	copy(sorted, fields)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	var b strings.Builder
	b.WriteString(`<fields entity="product">`)
	b.WriteByte('\n')
	for _, fd := range sorted {
		b.WriteString(fd.FieldName)
		// Pad field name to a consistent column width for readability.
		if pad := 16 - len(fd.FieldName); pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		} else {
			b.WriteByte(' ')
		}

		// Type descriptor — atom_type + subtype gives more info than type alone
		// (e.g. "number/currency" vs "number/rating").
		typeStr := string(fd.AtomType)
		if fd.AtomSubtype != "" && string(fd.AtomSubtype) != typeStr {
			typeStr = typeStr + "/" + string(fd.AtomSubtype)
		}
		b.WriteString(typeStr)
		if pad := 18 - len(typeStr); pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		} else {
			b.WriteByte(' ')
		}

		// Label in quotes for human context.
		if fd.Label != "" {
			b.WriteString(`label=`)
			if labelJSON, err := json.Marshal(fd.Label); err == nil {
				b.Write(labelJSON)
			}
			b.WriteString("  ")
		}

		// Unit if present (e.g. "RUB" for price).
		if fd.Unit != "" {
			b.WriteString(`unit="`)
			b.WriteString(fd.Unit)
			b.WriteString(`"  `)
		}

		// Default slot hints where this field "wants" to live.
		if fd.DefaultSlot != "" {
			b.WriteString(`slot=`)
			b.WriteString(string(fd.DefaultSlot))
			b.WriteString("  ")
		}

		// Samples — JSON-encoded array of the first few real values.
		if vals, ok := samples[fd.FieldName]; ok && len(vals) > 0 {
			if sampleJSON, err := json.Marshal(vals); err == nil {
				b.WriteString(`samples=`)
				b.Write(sampleJSON)
			}
		}

		b.WriteByte('\n')
	}
	b.WriteString(`</fields>`)
	return b.String()
}

// formatDesignContextBlock renders a compact <tenant_design_context> XML
// block for Agent2's system prompt. Describes what the tenant has published
// so Agent2 can reference tenant-only preset names, components, and tokens.
// Returns "" when the snapshot has nothing to show.
//
// Truncation caps: presets 50, components 25, tokens 80. Stable alphabetical
// ordering for cache warmth. Overflows append a <more count="N"/> footer so
// the LLM knows something was hidden. English only (feedback_prompt_language).
func formatDesignContextBlock(snap *engine_v4.DesignContextSnapshot) string {
	if snap == nil ||
		(len(snap.Presets) == 0 && len(snap.Components) == 0 && len(snap.Tokens) == 0) {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<tenant_design_context version="%d">`, snap.Version)

	// ---- presets ----
	if len(snap.Presets) > 0 {
		b.WriteString("\n  <presets>\n")
		presets := sortPresetsByName(snap.Presets)
		const maxPresets = 50
		shown := len(presets)
		if shown > maxPresets {
			shown = maxPresets
		}
		for i := 0; i < shown; i++ {
			p := presets[i]
			fmt.Fprintf(&b, `    <preset name=%q category=%q`, p.Name, p.Category)
			if snap.OverridesGlobal[p.Name] {
				b.WriteString(` overrides="global"`)
			}
			if p.DefaultReplicate {
				b.WriteString(` replicate="true"`)
			}
			b.WriteString(">\n")
			if p.Description != "" {
				fmt.Fprintf(&b, "      <desc>%s</desc>\n", escapeXMLInline(p.Description))
			}
			if binds := compactPresetBinds(p); binds != "" {
				fmt.Fprintf(&b, "      <binds>%s</binds>\n", binds)
			}
			b.WriteString("    </preset>\n")
		}
		if len(presets) > maxPresets {
			fmt.Fprintf(&b, `    <more count=%q/>`, strconv.Itoa(len(presets)-maxPresets))
			b.WriteByte('\n')
		}
		b.WriteString("  </presets>\n")
	}

	// ---- components ----
	if len(snap.Components) > 0 {
		b.WriteString("  <components>\n")
		comps := sortComponentsByName(snap.Components)
		const maxComponents = 25
		shown := len(comps)
		if shown > maxComponents {
			shown = maxComponents
		}
		for i := 0; i < shown; i++ {
			c := comps[i]
			fmt.Fprintf(&b, "    <component name=%q category=%q>%s</component>\n",
				c.Name, c.Category, escapeXMLInline(c.Description))
		}
		if len(comps) > maxComponents {
			fmt.Fprintf(&b, `    <more count=%q/>`, strconv.Itoa(len(comps)-maxComponents))
			b.WriteByte('\n')
		}
		b.WriteString("  </components>\n")
	}

	// ---- tokens ----
	if len(snap.Tokens) > 0 {
		b.WriteString("  <tokens>\n")
		grouped := groupTokensByCategory(snap.Tokens)
		catKeys := sortedCategoryKeys(grouped)
		const maxTokens = 80
		total := 0
		for _, cat := range catKeys {
			if total >= maxTokens {
				break
			}
			fmt.Fprintf(&b, "    <category name=%q>\n", cat)
			for _, tok := range grouped[cat] {
				if total >= maxTokens {
					break
				}
				total++
				fmt.Fprintf(&b, "      %s=%q", tok.Name, tok.Value)
				if tok.ThemeAxis != "" && tok.ThemeValue != "" {
					fmt.Fprintf(&b, " theme=%q:%q", tok.ThemeAxis, tok.ThemeValue)
				}
				b.WriteByte('\n')
			}
			b.WriteString("    </category>\n")
		}
		remaining := len(snap.Tokens) - total
		if remaining > 0 {
			fmt.Fprintf(&b, `    <more count=%q/>`, strconv.Itoa(remaining))
			b.WriteByte('\n')
		}
		b.WriteString("  </tokens>\n")
	}

	b.WriteString("</tenant_design_context>")
	return b.String()
}

// compactPresetBinds walks a preset's ops once and returns an ordered
// comma-separated list of unique fieldName values from atom-level props.
// Deliberately agnostic to Op.Type normalization — we just look at Props for
// "fieldName", which only atom inserts populate. Widget/row/column inserts
// don't have fieldName, so they're skipped automatically.
func compactPresetBinds(p *engine_v4.Preset) string {
	if p == nil || p.Build == nil {
		return ""
	}
	var names []string
	seen := map[string]bool{}
	for _, op := range p.Build() {
		if op.Props == nil {
			continue
		}
		if fn, _ := op.Props["fieldName"].(string); fn != "" && !seen[fn] {
			names = append(names, fn)
			seen[fn] = true
		}
	}
	return strings.Join(names, ", ")
}

// sortPresetsByName returns a sorted copy of the preset slice. Alphabetical
// order ensures byte-stable prompt output for Anthropic cache hits.
func sortPresetsByName(in []*engine_v4.Preset) []*engine_v4.Preset {
	out := make([]*engine_v4.Preset, len(in))
	copy(out, in)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// sortComponentsByName returns a sorted copy of the component slice.
func sortComponentsByName(in []ports.CanvasComponentView) []ports.CanvasComponentView {
	out := make([]ports.CanvasComponentView, len(in))
	copy(out, in)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// groupTokensByCategory groups tokens by Category for the formatted output.
func groupTokensByCategory(tokens []ports.CanvasDesignTokenView) map[string][]ports.CanvasDesignTokenView {
	m := make(map[string][]ports.CanvasDesignTokenView)
	for _, t := range tokens {
		m[t.Category] = append(m[t.Category], t)
	}
	return m
}

// sortedCategoryKeys returns the keys of a category map in sorted order.
func sortedCategoryKeys(m map[string][]ports.CanvasDesignTokenView) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// escapeXMLInline escapes &, <, > in a string for safe embedding in XML-ish
// text nodes. Does not escape quotes — those appear only in attribute values
// which are already handled by %q formatting.
func escapeXMLInline(s string) string {
	return html.EscapeString(s)
}

// Note: convertToFormation is defined in pipeline_execute.go and reused here
