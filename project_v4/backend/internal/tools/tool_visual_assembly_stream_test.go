package tools

import (
	"testing"

	"keepstar_v4/internal/engine_v4"
)

// feedAllAtOnce feeds the entire input as a single delta.
func feedAllAtOnce(t *testing.T, input string) []DecodedEvent {
	t.Helper()
	d := NewStreamingToolDecoder()
	events := d.Feed(input)
	events = append(events, d.Flush()...)
	return events
}

// feedSymbolBySymbol feeds the input one byte at a time and aggregates all
// events. Result should match feedAllAtOnce — this is the invariance that
// guarantees the state machine has no chunk-boundary bugs.
func feedSymbolBySymbol(t *testing.T, input string) []DecodedEvent {
	t.Helper()
	d := NewStreamingToolDecoder()
	var events []DecodedEvent
	for i := 0; i < len(input); i++ {
		events = append(events, d.Feed(input[i:i+1])...)
	}
	events = append(events, d.Flush()...)
	return events
}

// assertWidgetOpCounts checks the number of WidgetComplete events and the
// number of ops in each, returning the events for further inspection.
func assertWidgetOpCounts(t *testing.T, events []DecodedEvent, wantOpsPerWidget ...int) []DecodedEvent {
	t.Helper()
	var widgets []DecodedEvent
	for _, ev := range events {
		if ev.Kind == DecodeError {
			t.Fatalf("unexpected decode error: %v", ev.Err)
		}
		if ev.Kind == WidgetComplete {
			widgets = append(widgets, ev)
		}
	}
	if len(widgets) != len(wantOpsPerWidget) {
		t.Fatalf("widget count = %d, want %d", len(widgets), len(wantOpsPerWidget))
	}
	for i, w := range widgets {
		if len(w.Ops) != wantOpsPerWidget[i] {
			t.Errorf("widget %d: ops=%d, want %d; ops=%+v", i, len(w.Ops), wantOpsPerWidget[i], w.Ops)
		}
	}
	return widgets
}

// ---------------------------------------------------------------------------
// Sample 1: single widget with nested children
// ---------------------------------------------------------------------------

const sampleSingleWidget = `{"mode":"rebuild","layout":"grid","columns":3,"ops":[
  {"op":"insert","ref":"w","parent":"formation","props":{"type":"widget","size":"medium"}},
  {"op":"insert","ref":"col","parent":"$w","props":{"type":"column"}},
  {"op":"insert","parent":"$col","props":{"type":"image","fieldName":"images"}},
  {"op":"insert","parent":"$col","props":{"type":"text","fieldName":"name","wrapper":{"type":"h1"}}},
  {"op":"insert","parent":"$col","props":{"type":"number","fieldName":"price","format":"currency"}}
]}`

func TestDecoder_SingleWidget_Whole(t *testing.T) {
	events := feedAllAtOnce(t, sampleSingleWidget)
	assertWidgetOpCounts(t, events, 5)
}

func TestDecoder_SingleWidget_SymbolBySymbol(t *testing.T) {
	events := feedSymbolBySymbol(t, sampleSingleWidget)
	assertWidgetOpCounts(t, events, 5)
}

// ---------------------------------------------------------------------------
// Sample 2: multi-widget composition — hero literal + replicated grid
// ---------------------------------------------------------------------------

const sampleMultiWidget = `{"mode":"rebuild","ops":[
  {"op":"insert","ref":"hero","parent":"formation","props":{"type":"widget","size":"large"}},
  {"op":"insert","parent":"$hero","props":{"type":"text","value":"New collection","wrapper":{"type":"h1"}}},
  {"op":"insert","ref":"card","parent":"formation","props":{"type":"widget","preset":"product_card","replicate":true,"replicateLimit":6}},
  {"op":"insert","parent":"$card","props":{"type":"text","fieldName":"name"}}
]}`

func TestDecoder_MultiWidget_Whole(t *testing.T) {
	events := feedAllAtOnce(t, sampleMultiWidget)
	widgets := assertWidgetOpCounts(t, events, 2, 2)
	// Widget 1: hero
	if widgets[0].Ops[0].Ref != "hero" {
		t.Errorf("widget 1 first op ref = %q, want hero", widgets[0].Ops[0].Ref)
	}
	// Widget 2: card with preset
	if widgets[1].Ops[0].Ref != "card" {
		t.Errorf("widget 2 first op ref = %q, want card", widgets[1].Ops[0].Ref)
	}
	preset, _ := widgets[1].Ops[0].Props["preset"].(string)
	if preset != "product_card" {
		t.Errorf("widget 2 preset = %q, want product_card", preset)
	}
}

func TestDecoder_MultiWidget_SymbolBySymbol(t *testing.T) {
	events := feedSymbolBySymbol(t, sampleMultiWidget)
	assertWidgetOpCounts(t, events, 2, 2)
}

// ---------------------------------------------------------------------------
// Sample 3: escaped quotes and unicode inside string values
// ---------------------------------------------------------------------------

const sampleEscapedStrings = `{"ops":[
  {"op":"insert","parent":"formation","props":{"type":"widget"}},
  {"op":"insert","parent":"$w","props":{"type":"text","value":"She said \"hi\" — крема"}},
  {"op":"insert","parent":"$w","props":{"type":"text","value":"Unicode: \u00e9 \u2014 done"}},
  {"op":"insert","parent":"$w","props":{"type":"text","value":"Backslash \\ and brace } inside string"}}
]}`

func TestDecoder_EscapedStrings_Whole(t *testing.T) {
	events := feedAllAtOnce(t, sampleEscapedStrings)
	widgets := assertWidgetOpCounts(t, events, 4)
	// Check the escape-heavy value survived
	v, _ := widgets[0].Ops[1].Props["value"].(string)
	if v != `She said "hi" — крема` {
		t.Errorf("escaped value = %q", v)
	}
	// Brace inside string must not fool depth counter
	b, _ := widgets[0].Ops[3].Props["value"].(string)
	if b != `Backslash \ and brace } inside string` {
		t.Errorf("brace-in-string value = %q", b)
	}
}

func TestDecoder_EscapedStrings_SymbolBySymbol(t *testing.T) {
	events := feedSymbolBySymbol(t, sampleEscapedStrings)
	assertWidgetOpCounts(t, events, 4)
}

// ---------------------------------------------------------------------------
// Sample 4: deep nested props (wrapper.variant, meta.tracking)
// ---------------------------------------------------------------------------

const sampleDeepNesting = `{"ops":[
  {"op":"insert","parent":"formation","props":{"type":"widget","meta":{"tracking":{"event":"view","params":{"section":"hero","index":0}}}}},
  {"op":"insert","parent":"$w","props":{"type":"text","value":"Buy now","wrapper":{"type":"button","variant":"primary","meta":{"size":"lg","icon":{"name":"cart"}}}}}
]}`

func TestDecoder_DeepNesting_Whole(t *testing.T) {
	events := feedAllAtOnce(t, sampleDeepNesting)
	widgets := assertWidgetOpCounts(t, events, 2)
	// Access nested meta.tracking.event
	meta, _ := widgets[0].Ops[0].Props["meta"].(map[string]interface{})
	tracking, _ := meta["tracking"].(map[string]interface{})
	ev, _ := tracking["event"].(string)
	if ev != "view" {
		t.Errorf("deep nested event = %q, want view", ev)
	}
}

func TestDecoder_DeepNesting_SymbolBySymbol(t *testing.T) {
	events := feedSymbolBySymbol(t, sampleDeepNesting)
	assertWidgetOpCounts(t, events, 2)
}

// ---------------------------------------------------------------------------
// Sample 5: empty ops array
// ---------------------------------------------------------------------------

const sampleEmptyOps = `{"mode":"modify","ops":[]}`

func TestDecoder_EmptyOps(t *testing.T) {
	events := feedAllAtOnce(t, sampleEmptyOps)
	for _, ev := range events {
		if ev.Kind == DecodeError {
			t.Fatalf("unexpected decode error: %v", ev.Err)
		}
	}
	assertWidgetOpCounts(t, events /* none */)
}

// ---------------------------------------------------------------------------
// Sample 6: modification ops (no insert widget at all — just updates to
// existing atoms). Decoder should still emit them as a single "widget group"
// attached to the first op, OR keep them ungrouped. Current rule: the first
// op becomes the start of an implicit group even if it's not a widget insert;
// all subsequent ops stay in that group until a new insert-widget appears.
// ---------------------------------------------------------------------------

const sampleModifyOnly = `{"mode":"modify","ops":[
  {"op":"update","target":"a-price-0","props":{"format":"currency","style":{"color":"red"}}},
  {"op":"update","target":"a-name-0","props":{"style":{"fontWeight":"bold"}}},
  {"op":"delete","target":"a-badge-0"}
]}`

func TestDecoder_ModifyOnly_Whole(t *testing.T) {
	events := feedAllAtOnce(t, sampleModifyOnly)
	// All 3 ops group into one implicit widget since none are insert widget.
	assertWidgetOpCounts(t, events, 3)
}

// ---------------------------------------------------------------------------
// Sample 7: widget boundary detection — ops with props.type="widget" AND
// ops with parent=="formation" (without explicit type) should both trigger
// a new group. Mirrors engine_v4/ops.go:330.
// ---------------------------------------------------------------------------

const sampleBoundaryVariants = `{"ops":[
  {"op":"insert","parent":"formation","props":{"size":"large"}},
  {"op":"insert","parent":"$w","props":{"type":"text","value":"hero"}},
  {"op":"insert","parent":"formation","props":{"type":"widget"}},
  {"op":"insert","parent":"$w","props":{"type":"text","fieldName":"name"}}
]}`

func TestDecoder_BoundaryVariants(t *testing.T) {
	events := feedAllAtOnce(t, sampleBoundaryVariants)
	// Both first and third ops qualify as widget-start (parent=formation and type=widget).
	// Expect 2 widgets of 2 ops each.
	assertWidgetOpCounts(t, events, 2, 2)
}

// ---------------------------------------------------------------------------
// Raw JSON fallback
// ---------------------------------------------------------------------------

func TestDecoder_FullRawJSON(t *testing.T) {
	d := NewStreamingToolDecoder()
	d.Feed(`{"mode":"rebuild",`)
	d.Feed(`"ops":[]}`)
	d.Flush()
	got := d.FullRawJSON()
	want := `{"mode":"rebuild","ops":[]}`
	if got != want {
		t.Errorf("raw = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Invariance: symbol-by-symbol and whole-input feeds must produce identical events.
// ---------------------------------------------------------------------------

func TestDecoder_FeedInvariance(t *testing.T) {
	samples := []string{
		sampleSingleWidget,
		sampleMultiWidget,
		sampleEscapedStrings,
		sampleDeepNesting,
		sampleEmptyOps,
		sampleModifyOnly,
		sampleBoundaryVariants,
	}
	for i, s := range samples {
		whole := feedAllAtOnce(t, s)
		sym := feedSymbolBySymbol(t, s)
		if len(whole) != len(sym) {
			t.Errorf("sample %d: event count whole=%d sym=%d", i, len(whole), len(sym))
			continue
		}
		for j := range whole {
			if whole[j].Kind != sym[j].Kind {
				t.Errorf("sample %d event %d: kind differs", i, j)
			}
			if len(whole[j].Ops) != len(sym[j].Ops) {
				t.Errorf("sample %d event %d: ops count differs whole=%d sym=%d", i, j, len(whole[j].Ops), len(sym[j].Ops))
			}
		}
	}
}

// ---------------------------------------------------------------------------
// isWidgetInsert smoke test
// ---------------------------------------------------------------------------

func TestIsWidgetInsert(t *testing.T) {
	cases := []struct {
		op   engine_v4.Op
		want bool
	}{
		{engine_v4.Op{Type: engine_v4.OpInsert, Parent: "formation"}, true},
		{engine_v4.Op{Type: engine_v4.OpInsert, Props: map[string]interface{}{"type": "widget"}}, true},
		{engine_v4.Op{Type: engine_v4.OpInsert, Parent: "$w", Props: map[string]interface{}{"type": "text"}}, false},
		{engine_v4.Op{Type: engine_v4.OpUpdate, Parent: "formation"}, false},
		{engine_v4.Op{Type: engine_v4.OpDelete, Target: "w-s0-w0"}, false},
	}
	for i, c := range cases {
		got := isWidgetInsert(c.op)
		if got != c.want {
			t.Errorf("case %d: got %v, want %v", i, got, c.want)
		}
	}
}
