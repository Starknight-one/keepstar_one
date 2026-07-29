package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine"
	"keepstar_v5/internal/engine/presets"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/streaming"
)

// ComposeTurnTool is the compose_turn visual executor (RUNTIME_SPEC.md
// §4.7 + final owner decision 3): one call composes the whole turn as an
// ordered list of TurnBlocks — text paragraphs interleaved with rendered
// documents. Each render block runs the exact visual_assembly chain
// (Materialise → ApplyOps → ExpandReplicates → ResolveAndInline → BindData
// → InjectDefaultActions) against the session's live data zone.
//
// Streaming (R9 as OVERRIDDEN): every block is emitted to the ctx-carried
// domain.TurnBlockCollector AS PRODUCED — the SSE handler turns each into
// its own `event: block` frame — and the collector retains the ordered
// list for the terminal `result` frame / POST /pipeline JSON body.
//
// Contract enforced here (the registry runs this executor as passthrough,
// like visual_assembly — the tool owns its own input validation):
//   - max 8 blocks, at least 1;
//   - kind "text" requires non-empty text; kind "render" requires preset
//     and/or ops;
//   - one call per turn: a second call after blocks were already emitted
//     this turn → ToolResult IsError. A call that failed validation before
//     emitting anything does NOT consume the turn — the LLM may retry.
//
// Writes: a display:"screen" block ALSO writes state.Current.Template via
// UpdateTemplate (navigation / back-stack semantics unchanged); inline
// blocks live only on the wire. Input validation failures reject the WHOLE
// call before any block is emitted; a per-block assembly failure (preset
// not found) skips that block and is reported in the summary so the LLM
// can address it without losing the rest of the turn.
type ComposeTurnTool struct {
	state      ports.StatePort
	presets    ports.PresetPort
	components ports.ComponentPort
	manifest   ManifestStepGate
}

// ManifestStepGate is the slice of the onboarding ManifestApplier this tool
// needs to honour the owner's law that a user-visible artifact must never
// depend on the model having called a tool (handoff 2026-07-28, law #1).
// Declared here rather than imported: tools cannot import usecases (the
// usecases tests import tools, which would cycle the test binary);
// *usecases.ManifestApplier satisfies it.
type ManifestStepGate interface {
	// EnsureZeroInputStep stages the zero-input step when it is missing and
	// runs the applier. Reports whether it did any work — true means session
	// state changed and must be re-read.
	EnsureZeroInputStep(ctx context.Context, sessionID, op string) (bool, error)
}

// zeroInputStepForPreset maps a preset whose data source is the synthetic
// EntitySet of a ZERO-INPUT manifest step onto that step's op. Rendering
// such a preset IS the intent — the same proof as a submitted form or an
// uploaded file — so the server stages and applies the step deterministically
// instead of showing an empty card and hoping the model calls apply_manifest.
// A small table, deliberately: every entry needs a synthetic set that only
// its step can produce.
var zeroInputStepForPreset = map[string]string{
	"surface_links": "issue_surface_urls",
}

// NewComposeTurnTool constructs the tool with the three ports it needs.
func NewComposeTurnTool(state ports.StatePort, presets ports.PresetPort, components ports.ComponentPort) *ComposeTurnTool {
	return &ComposeTurnTool{
		state:      state,
		presets:    presets,
		components: components,
	}
}

// SetManifestGate closes the boot wiring cycle (applier → registry →
// compose_turn): the tool is registered before the ManifestApplier exists,
// same deferred-wiring pattern as PresetOperationBinder.SetResolver. Unset
// (storefront-only wirings) disables the zero-input auto-apply pass — the
// render then binds whatever the data zone already holds.
func (t *ComposeTurnTool) SetManifestGate(g ManifestStepGate) { t.manifest = g }

// Compile-time check that ComposeTurnTool satisfies Tool.
var _ Tool = (*ComposeTurnTool)(nil)

// maxTurnBlocks caps one composed turn (§4.7).
const maxTurnBlocks = 8

// Definition returns the LLM-facing declaration. It MUST stay in lockstep
// with the seeded v5_operations row (seed.Templates() "compose_turn") —
// the registry serves the seeded schema to the LLM, this tool validates
// the input; compose_turn_test.go in internal/operations asserts the two
// stay byte-equal so they cannot drift silently.
func (t *ComposeTurnTool) Definition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name: "compose_turn",
		Description: "Compose the assistant's answer as an ordered sequence of " +
			"blocks: text paragraphs interleaved with rendered documents — cards, " +
			"forms, tables, previews built from presets with real data bound. Each " +
			"block is text or a render; renders display inline in the chat column " +
			"or as the main screen. One call per turn, up to 8 blocks.",
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
	}
}

// composeBlock is one parsed input block.
type composeBlock struct {
	kind      string // "text" | "render"
	text      string
	preset    string
	display   string // "inline" | "screen" (render blocks; defaulted to inline)
	ops       []map[string]any
	replicate any // raw input value; resolved per block (count or source name)
}

// Execute runs one compose_turn call: validate everything up front (no
// partial emission on bad input), then produce and emit blocks in order.
//
// Streaming dedupe handshake (lane D): when agent2_execute installed a
// streaming.EarlyEmitter on the ctx, the leading run of this call's TEXT
// blocks may ALREADY be on the wire — parsed out of the model's partial
// tool input and emitted mid-generation. Execute then skips re-emitting
// exactly that prefix (specs[0..early.Count()-1]; both sides parsed the
// same JSON, so index identity holds) and emits the rest as today.
// Document blocks are never early-emitted — they need this assembly
// chain — so they always emit here, on assembly (the chosen simplest
// correct path: no placeholders, no replace frames, no reordering).
// The one-call-per-turn guard moves to the emitter's Claim flag: claimed
// when validation passes OR when this call's early prefix already
// reached the wire; a call refused before anything was emitted keeps the
// retry path open, exactly like the legacy collector-count guard that
// still covers hook-less turns.
func (t *ComposeTurnTool) Execute(ctx context.Context, toolCtx domain.ToolContext, input map[string]interface{}) (*domain.ToolResult, error) {
	collector := domain.TurnBlockCollectorFromContext(ctx)
	early := streaming.EarlyEmitterFromContext(ctx)
	if early != nil {
		if early.Claimed() {
			return errorResult("compose_turn: one call per turn — this turn already composed its blocks"), nil
		}
	} else if collector != nil && collector.Count() > 0 {
		return errorResult("compose_turn: one call per turn — this turn already composed its blocks"), nil
	}

	specs, problem := parseComposeBlocks(input)
	if early != nil && (problem == "" || early.Count() > 0) {
		// Consume the turn: validation passed, or this call's early text
		// prefix is already on the wire — either way a sibling
		// compose_turn call must not compose again on top of it.
		early.Claim()
	}
	if problem != "" {
		return errorResult("compose_turn: " + problem), nil
	}

	state, err := t.state.GetState(ctx, toolCtx.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// Deterministic uploader arming (R25): the minted ingest token NEVER
	// passes through the LLM — when the session's manifest carries one,
	// every rendered upload node is armed server-side (armUploadNodes).
	ingestToken := t.ingestTokenFor(ctx, toolCtx.SessionID)

	slog.Info("compose_turn",
		"blocks", len(specs),
		"session_id", toolCtx.SessionID,
		"tenant", toolCtx.TenantSlug,
	)

	var (
		emitted                          int
		textCount, docCount, screenCount int
		failures                         []string
		// ensured guards the zero-input pass against repeating inside one
		// call — a turn may render the same preset twice, and a gate that
		// errored must not be retried seven more times.
		ensured map[string]bool
	)
	emit := func(b domain.TurnBlock) {
		if collector != nil {
			collector.Emit(b)
		}
		emitted++
	}

	// Blocks already streamed mid-generation by the EarlyEmitter (the
	// leading text run — see the handshake note on Execute). They are in
	// the collector and on the wire; count them, never re-emit.
	skip := 0
	if early != nil {
		skip = early.Count()
		if skip > len(specs) {
			skip = len(specs) // defensive; both sides parsed the same JSON
		}
	}

	for i, spec := range specs {
		if i < skip {
			if spec.kind == "text" {
				textCount++
			} else {
				// Impossible unless the incremental parser drifted from
				// parseComposeBlocks — refuse to double-emit, say so loudly.
				slog.Warn("compose_turn: early-emitted block is not text — parser drift, block not re-emitted",
					"index", i, "kind", spec.kind, "session_id", toolCtx.SessionID)
			}
			emitted++
			continue
		}
		switch spec.kind {
		case "text":
			emit(domain.TurnBlock{
				BlockID: newBlockID(),
				Kind:    domain.BlockKindText,
				Text:    spec.text,
			})
			textCount++

		case "render":
			// A zero-input step whose block is about to render gets staged +
			// applied HERE, before assembly, so the block goes on the wire
			// carrying real data instead of an empty shell.
			if op, ok := zeroInputStepForPreset[spec.preset]; ok && t.manifest != nil && !ensured[op] {
				if ensured == nil {
					ensured = map[string]bool{}
				}
				ensured[op] = true
				changed, gerr := t.manifest.EnsureZeroInputStep(ctx, toolCtx.SessionID, op)
				if gerr != nil {
					slog.Warn("compose_turn: zero-input step auto-apply failed — block renders on existing data",
						"preset", spec.preset, "op", op, "session_id", toolCtx.SessionID, "err", gerr)
				} else if changed {
					// The apply wrote the step's synthetic EntitySet into the
					// data zone; the snapshot read at the top of Execute
					// predates it.
					fresh, serr := t.state.GetState(ctx, toolCtx.SessionID)
					if serr != nil {
						return nil, fmt.Errorf("get state after %s: %w", op, serr)
					}
					state = fresh
				}
			}
			templateMap, err := t.assembleRenderBlock(ctx, toolCtx, state, spec, ingestToken)
			if err != nil {
				failures = append(failures, fmt.Sprintf("block %d (%s): %v", i+1, spec.preset, err))
				continue
			}
			// A screen block ALSO becomes the main surface: write it to
			// current.template BEFORE emitting so the server state is
			// consistent by the time the widget receives the frame.
			if spec.display == domain.DisplayScreen {
				deltaInfo := domain.DeltaInfo{
					TurnID:    toolCtx.TurnID,
					Trigger:   domain.TriggerUserQuery,
					Source:    domain.SourceLLM,
					ActorID:   "agent2",
					DeltaType: domain.DeltaTypeUpdate,
					Path:      "current.template",
					Action:    domain.Action{Tool: "compose_turn", Params: map[string]interface{}{"preset": spec.preset, "display": spec.display}},
				}
				if _, err := t.state.UpdateTemplate(ctx, toolCtx.SessionID, templateMap, deltaInfo); err != nil {
					return nil, fmt.Errorf("update template: %w", err)
				}
				screenCount++
			}
			emit(domain.TurnBlock{
				BlockID:  newBlockID(),
				Kind:     domain.BlockKindDocument,
				Document: templateMap,
				Display:  spec.display,
			})
			docCount++
		}
	}

	if emitted == 0 {
		return errorResult("compose_turn: no blocks rendered — " + strings.Join(failures, "; ")), nil
	}
	summary := fmt.Sprintf("OK blocks=%d text=%d documents=%d screen=%d", emitted, textCount, docCount, screenCount)
	if len(failures) > 0 {
		summary += "; failed: " + strings.Join(failures, "; ")
	}
	return &domain.ToolResult{Content: summary}, nil
}

// assembleRenderBlock runs the visual_assembly chain for one render block
// and returns the marshaled Document map (§4.7: Materialise → ApplyOps →
// ExpandReplicates → ResolveAndInline → BindData → InjectDefaultActions).
// Mirrors VisualAssemblyTool's rebuild path — preset-based or ops-only
// freestyle; there is no modify shape at the block level.
func (t *ComposeTurnTool) assembleRenderBlock(ctx context.Context, toolCtx domain.ToolContext, state *domain.SessionState, spec composeBlock, ingestToken string) (map[string]interface{}, error) {
	var merged *engine.Document
	if spec.preset != "" {
		preset, err := t.presets.GetPublishedPreset(ctx, toolCtx.TenantSlug, spec.preset)
		if err != nil {
			return nil, fmt.Errorf("preset %q not found: %v", spec.preset, err)
		}
		presetDoc, err := unmarshalDoc(preset.DocumentJSON)
		if err != nil {
			return nil, fmt.Errorf("unmarshal preset doc: %w", err)
		}
		components, err := t.components.ListPublishedComponents(ctx, toolCtx.TenantSlug)
		if err != nil {
			return nil, fmt.Errorf("list components: %v", err)
		}
		componentDocs := make([]*engine.Document, 0, len(components))
		for _, c := range components {
			cd, err := unmarshalDoc(c.DocumentJSON)
			if err != nil {
				slog.Warn("compose_turn: component doc unmarshal failed; skipping", "component", c.Name, "err", err)
				continue
			}
			componentDocs = append(componentDocs, cd)
		}
		merged = engine.Materialise(presetDoc, componentDocs)
	} else {
		merged = engine.NewDocument()
	}

	if err := engine.ApplyOps(merged, spec.ops); err != nil {
		return nil, fmt.Errorf("applyOps: %v", err)
	}

	// After ApplyOps so the deterministic arm pass always wins over any
	// LLM-emitted op touching the same props.
	armUploadNodes(merged, ingestToken)

	data, count := resolveReplicate(state.Current.Data, spec.replicate, spec.preset)
	engine.ExpandReplicates(merged, count)

	// Freestyle blocks may reference tenant components; load them on demand
	// (preset blocks got theirs through Materialise) — visual_assembly's
	// on-demand path verbatim.
	if spec.preset == "" && documentHasRefs(merged) {
		components, err := t.components.ListPublishedComponents(ctx, toolCtx.TenantSlug)
		if err != nil {
			return nil, fmt.Errorf("list components: %v", err)
		}
		for _, c := range components {
			cd, err := unmarshalDoc(c.DocumentJSON)
			if err != nil {
				slog.Warn("compose_turn: component doc unmarshal failed; skipping", "component", c.Name, "err", err)
				continue
			}
			for _, child := range cd.Children {
				child["reusable"] = true
				merged.Children = append(merged.Children, child)
			}
		}
	}
	engine.ResolveAndInline(merged)
	engine.BindData(merged, data)

	// After BindData — the address only exists in `content` once the record
	// is bound.
	linkBoundURLs(merged)

	// Default like/cart actions are catalog actions: fed PRODUCT maps only,
	// exactly like visual_assembly — entity-plane blocks (leads, synthetic
	// opCard sets) never receive them.
	engine.InjectDefaultActions(merged, domain.EntityTypeProduct, productsToBindData(state.Current.Data.Products))

	templateMap, err := docToMap(merged)
	if err != nil {
		return nil, fmt.Errorf("marshal document: %w", err)
	}
	if spec.preset != "" {
		templateMap[domain.TemplatePresetInUseKey] = spec.preset
	}
	return templateMap, nil
}

// parseComposeBlocks validates the whole input up front — any violation
// rejects the call BEFORE a single block is emitted, so the LLM's retry
// does not trip the one-call-per-turn guard. Returns the parsed blocks or
// a problem string for the ToolResult IsError path.
func parseComposeBlocks(input map[string]interface{}) ([]composeBlock, string) {
	raw, ok := input["blocks"]
	if !ok || raw == nil {
		return nil, "blocks required: pass an ordered list of text/render blocks"
	}
	var items []map[string]any
	switch v := raw.(type) {
	case []map[string]any:
		items = v
	case []interface{}:
		items = make([]map[string]any, 0, len(v))
		for i, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Sprintf("blocks[%d]: expected object, got %T", i, item)
			}
			items = append(items, m)
		}
	default:
		return nil, fmt.Sprintf("blocks: expected array, got %T", raw)
	}
	if len(items) == 0 {
		return nil, "blocks required: pass at least one block"
	}
	if len(items) > maxTurnBlocks {
		return nil, fmt.Sprintf("too many blocks: %d, max %d per turn", len(items), maxTurnBlocks)
	}

	out := make([]composeBlock, 0, len(items))
	for i, m := range items {
		kind, _ := m["kind"].(string)
		switch kind {
		case "text":
			text, _ := m["text"].(string)
			if strings.TrimSpace(text) == "" {
				return nil, fmt.Sprintf("blocks[%d]: kind \"text\" requires non-empty `text`", i)
			}
			out = append(out, composeBlock{kind: kind, text: text})
		case "render":
			preset, _ := m["preset"].(string)
			ops, err := parseOpsList(m["ops"])
			if err != nil {
				return nil, fmt.Sprintf("blocks[%d]: %v", i, err)
			}
			if preset == "" && len(ops) == 0 {
				return nil, fmt.Sprintf("blocks[%d]: kind \"render\" requires `preset` or `ops`", i)
			}
			display, _ := m["display"].(string)
			switch display {
			case "":
				display = domain.DisplayInline
			case domain.DisplayInline, domain.DisplayScreen:
				// valid as-is
			default:
				return nil, fmt.Sprintf("blocks[%d]: display must be \"inline\" or \"screen\", got %q", i, display)
			}
			out = append(out, composeBlock{kind: kind, preset: preset, ops: ops, display: display, replicate: m["replicate"]})
		default:
			return nil, fmt.Sprintf("blocks[%d]: kind must be \"text\" or \"render\", got %q", i, kind)
		}
	}
	return out, ""
}

// resolveReplicate maps a block's `replicate` value onto (bind rows, clone
// count). The seeded schema types it as a string "replicate source", so
// two shapes are accepted:
//
//   - a count ("3", or a raw number for robustness) → visual_assembly
//     semantics: fan out N clones over the full data zone (products first,
//     then entity sets — dataToBindData order);
//   - a source name ("products" or an entity-set slug, incl. synthetic
//     sets like opCard/manifestStep, R23) → fan out one clone per record
//     of THAT source and bind only its rows, so a scoped list preset
//     replicates over exactly its own data.
//
// Absent/empty/unknown → the preset's AUTHORED source when it declares one
// (presets.SystemPresetReplicateSource) and that source is live in the data
// zone; otherwise no fan-out (count 0), full data — a single instance binds
// record 0, same as visual_assembly without replicate.
//
// The authored-source fallback is the fix for the surface-links handover
// (tail #3): a preset built for a synthetic set has exactly one sensible
// source, and letting a mis-spelled argument collapse its rows put an empty
// card in front of the business user.
func resolveReplicate(d domain.StateData, raw any, preset string) ([]map[string]any, int) {
	source, count, hasCount := parseReplicateArg(raw)

	// 1. The model named a source that is live in the data zone — it wins.
	if source == "products" {
		rows := productsToBindData(d.Products)
		return rows, len(rows)
	}
	if source != "" {
		if rows, n, ok := entitySetRows(d, source); ok {
			return rows, n
		}
	}

	// 2. The preset's AUTHORED source: the model omitted it, mistyped it, or
	//    sent a count for a preset that has exactly one sensible source.
	if src, ok := presets.SystemPresetReplicateSource[preset]; ok {
		if rows, n, ok := entitySetRows(d, src); ok {
			if source != "" {
				slog.Warn("compose_turn: unknown replicate source — using the preset's authored source",
					"replicate", source, "preset", preset, "source", src)
			}
			return rows, n
		}
	}
	if source != "" {
		slog.Warn("compose_turn: unknown replicate source — no fan-out", "replicate", source)
	}

	// 3. Count semantics over the full data zone (visual_assembly parity).
	if hasCount {
		return dataToBindData(d), count
	}
	return dataToBindData(d), 0
}

// parseReplicateArg splits a block's raw `replicate` value into the source
// name it asked for and/or the clone count it asked for. Exactly one of the
// two is ever set: the seeded schema types the field as a string, so a count
// arrives spelled ("3") as often as raw.
func parseReplicateArg(raw any) (source string, count int, hasCount bool) {
	clamp := func(n int) int {
		if n < 0 {
			return 0
		}
		return n
	}
	switch v := raw.(type) {
	case int:
		return "", clamp(v), true
	case int64:
		return "", clamp(int(v)), true
	case float64:
		return "", clamp(int(v)), true
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return "", 0, false
		}
		if n, err := strconv.Atoi(s); err == nil {
			return "", clamp(n), true
		}
		return s, 0, false
	}
	return "", 0, false
}

// entitySetRows flattens one entity set of the data zone into bind rows.
// ok is false when the zone carries no set under that slug.
func entitySetRows(d domain.StateData, slug string) ([]map[string]any, int, bool) {
	for i := range d.Entities {
		set := &d.Entities[i]
		if set.Slug != slug {
			continue
		}
		rows := make([]map[string]any, 0, len(set.Records))
		for _, rec := range set.Records {
			rows = append(rows, engine.EntityToMap(rec, set))
		}
		return rows, len(rows), true
	}
	return nil, 0, false
}

// linkBoundURLs makes a bound address tappable: a node authored as
// wrapper "link" (domain.WrapperLink) carries its URL in `content` after
// BindData, and the widget's link wrapper reads the href from
// action.params.url — nothing in the engine can bridge the two, because
// binding never writes action params. This deterministic pass does, right
// after BindData, for every link node that has no action of its own.
//
// Sibling of armUploadNodes: the server owns what the model cannot express
// per record.
func linkBoundURLs(doc *engine.Document) {
	if doc == nil {
		return
	}
	engine.WalkNodes(doc, func(n engine.Node, _ int) {
		if w, _ := n["wrapper"].(string); w != string(domain.WrapperLink) {
			return
		}
		if _, exists := n["action"]; exists {
			return // authored intent wins
		}
		url, _ := n["content"].(string)
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			return
		}
		n["action"] = map[string]any{
			"kind":   string(domain.UserActionExternalLink),
			"params": map[string]any{"url": url},
		}
	})
}

// ingestTokenFor reads the session's minted issue_ingest_door token from
// the onboarding manifest (R25). The capability token stays server-side:
// the choreography just re-renders uploader_card and this pass binds the
// token deterministically. Sessions without a manifest zone (storefront /
// CRM, or the state port not implementing the onboarding extension) and
// pre-apply turns return "".
func (t *ComposeTurnTool) ingestTokenFor(ctx context.Context, sessionID string) string {
	ob, ok := t.state.(ports.OnboardingStatePort)
	if !ok {
		return ""
	}
	m, err := ob.GetOnboarding(ctx, sessionID)
	if err != nil || m == nil {
		return ""
	}
	for i := range m.Steps {
		st := &m.Steps[i]
		if st.Op != "issue_ingest_door" {
			continue
		}
		if tok, _ := st.Result["token"].(string); tok != "" {
			return tok
		}
	}
	return ""
}

// armUploadNodes flips every upload node in the document to its armed
// shape (§5.2 upload, R25): disarmed false + the minted token bound; the
// seeded "activates once you confirm" note is cleared (the surrounding
// choreography text invites the upload). No-op without a token — beat-2
// renders stay disarmed exactly as seeded.
func armUploadNodes(doc *engine.Document, token string) {
	if doc == nil || token == "" {
		return
	}
	engine.WalkNodes(doc, func(n engine.Node, _ int) {
		if engine.NodeType(n) != "upload" {
			return
		}
		n["disarmed"] = false
		n["token"] = token
		n["note"] = ""
	})
}

// newBlockID mints a per-block wire id. Random (not per-turn ordinal) so
// apply-target references ({target:"block", blockId}) stay unambiguous
// across the turns of one session transcript.
func newBlockID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is effectively fatal elsewhere; degrade to a
		// clock-based id rather than crashing a turn.
		return fmt.Sprintf("blk_%x", time.Now().UnixNano())
	}
	return "blk_" + hex.EncodeToString(b[:])
}
