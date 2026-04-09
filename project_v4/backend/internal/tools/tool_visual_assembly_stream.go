package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"keepstar_v4/internal/engine_v4"
)

// ============================================================================
// StreamingToolDecoder
//
// Incrementally parses the JSON input of a visual_assembly tool_use block as
// it arrives via Anthropic's input_json_delta events. The decoder detects
// widget boundaries inside the "ops" array and emits one WidgetComplete event
// per widget group — a widget group consists of an "insert widget" op
// followed by all its child ops (layout nodes, atoms) up until the next
// "insert widget" op or the end of the array.
//
// Usage:
//
//	dec := NewStreamingToolDecoder()
//	for delta := range anthropicDeltas {
//	    for _, ev := range dec.Feed(delta) {
//	        switch ev.Kind {
//	        case WidgetComplete:
//	            // ev.Ops contains a full widget group
//	        case DecodeError:
//	            // ev.Err — malformed JSON
//	        }
//	    }
//	}
//	for _, ev := range dec.Flush() {
//	    // final widget (if ops array closed without emitting)
//	}
//
// The decoder is NOT concurrency-safe; one decoder per stream.
// ============================================================================

type StreamingToolDecoder struct {
	buf       strings.Builder
	cursor    int // next unprocessed index into buf.String()
	state     decoderState
	curWidget []engine_v4.Op
	depth     int // debug/diagnostics only
}

type decoderState int

const (
	stateScanningPreamble decoderState = iota // before "ops":[
	stateInOpsArray                           // inside the array, reading objects
	stateAfterOpsArray                        // past the closing ]
)

// DecodedEventKind identifies the type of a DecodedEvent.
type DecodedEventKind int

const (
	// WidgetComplete signals that a full widget group (insert widget + its
	// children) has been parsed and can be executed immediately.
	WidgetComplete DecodedEventKind = iota
	// DecodeError signals a malformed op object or unexpected input.
	DecodeError
)

// DecodedEvent is an output of StreamingToolDecoder.Feed / Flush.
type DecodedEvent struct {
	Kind DecodedEventKind
	Ops  []engine_v4.Op // for WidgetComplete
	Err  error          // for DecodeError
}

// NewStreamingToolDecoder creates a fresh decoder.
func NewStreamingToolDecoder() *StreamingToolDecoder {
	return &StreamingToolDecoder{state: stateScanningPreamble}
}

// Feed appends a new delta to the internal buffer and attempts to parse as
// much as possible. Returns any events (widget completions or errors) that
// became available after consuming this delta.
func (d *StreamingToolDecoder) Feed(delta string) []DecodedEvent {
	if delta == "" {
		return nil
	}
	d.buf.WriteString(delta)
	return d.drain()
}

// Flush emits any buffered widget that's still being accumulated. Call this
// when the upstream SSE stream ends (content_block_stop for the tool_use).
// Returns an empty slice if nothing is pending.
func (d *StreamingToolDecoder) Flush() []DecodedEvent {
	out := d.drain()
	if len(d.curWidget) > 0 {
		out = append(out, DecodedEvent{Kind: WidgetComplete, Ops: d.curWidget})
		d.curWidget = nil
	}
	return out
}

// FullRawJSON returns the raw accumulated JSON buffer. Useful as a fallback
// when the caller wants to reparse the whole tool input (e.g. for fields
// outside the ops array like mode/layout/columns that live at the root).
func (d *StreamingToolDecoder) FullRawJSON() string {
	return d.buf.String()
}

// drain runs the state machine until it needs more data or hits a terminal.
func (d *StreamingToolDecoder) drain() []DecodedEvent {
	var events []DecodedEvent
	s := d.buf.String()

	for {
		switch d.state {
		case stateScanningPreamble:
			idx, ok := findOpsArrayStart(s, d.cursor)
			if !ok {
				return events
			}
			d.cursor = idx
			d.state = stateInOpsArray

		case stateInOpsArray:
			// Skip whitespace and commas between elements
			d.cursor = skipWSAndCommas(s, d.cursor)
			if d.cursor >= len(s) {
				return events
			}
			// Check for end of array
			if s[d.cursor] == ']' {
				// Flush pending widget at array end
				if len(d.curWidget) > 0 {
					events = append(events, DecodedEvent{Kind: WidgetComplete, Ops: d.curWidget})
					d.curWidget = nil
				}
				d.cursor++
				d.state = stateAfterOpsArray
				return events
			}
			// Must be an object start
			if s[d.cursor] != '{' {
				// Wait for more data — could be whitespace or partial
				return events
			}
			// Find the balanced close brace
			end, ok := findBalancedObjectEnd(s, d.cursor)
			if !ok {
				return events // need more data
			}
			// Parse the op
			var op engine_v4.Op
			raw := s[d.cursor : end+1]
			if err := json.Unmarshal([]byte(raw), &op); err != nil {
				events = append(events, DecodedEvent{
					Kind: DecodeError,
					Err:  fmt.Errorf("op decode at offset %d: %w; raw=%s", d.cursor, err, raw),
				})
				// Skip past this object to avoid infinite loop on malformed input
				d.cursor = end + 1
				continue
			}
			// Widget boundary: emit previous group if this op starts a new widget
			if isWidgetInsert(op) && len(d.curWidget) > 0 {
				events = append(events, DecodedEvent{Kind: WidgetComplete, Ops: d.curWidget})
				d.curWidget = nil
			}
			d.curWidget = append(d.curWidget, op)
			d.cursor = end + 1

		case stateAfterOpsArray:
			return events
		}
	}
}

// ============================================================================
// Helpers
// ============================================================================

// findOpsArrayStart scans for the pattern `"ops"` (key) followed by `:` and
// an opening `[`. Returns the index *after* the `[` and true if found.
// Tolerates whitespace. Does not attempt to handle `ops` appearing as a
// value inside a string — this is acceptable because the tool input is a
// well-defined schema where "ops" is always the top-level array key.
func findOpsArrayStart(s string, from int) (int, bool) {
	needle := `"ops"`
	for {
		i := strings.Index(s[from:], needle)
		if i < 0 {
			return 0, false
		}
		pos := from + i + len(needle)
		// skip whitespace
		for pos < len(s) && isWS(s[pos]) {
			pos++
		}
		if pos >= len(s) {
			return 0, false
		}
		if s[pos] != ':' {
			// Not a key — could be inside a string value. Keep searching.
			from = pos
			continue
		}
		pos++
		for pos < len(s) && isWS(s[pos]) {
			pos++
		}
		if pos >= len(s) {
			return 0, false
		}
		if s[pos] != '[' {
			// "ops": but not an array — give up, caller may treat as decode error later
			return 0, false
		}
		return pos + 1, true
	}
}

// skipWSAndCommas advances past whitespace and ',' characters.
func skipWSAndCommas(s string, from int) int {
	for from < len(s) {
		c := s[from]
		if isWS(c) || c == ',' {
			from++
			continue
		}
		break
	}
	return from
}

// findBalancedObjectEnd starting at s[from] == '{', walks forward counting
// braces while respecting JSON string literals (with \-escapes). Returns
// (endIndex, true) where s[endIndex] == '}' is the matching close brace,
// or (0, false) if the buffer does not yet contain the full object.
func findBalancedObjectEnd(s string, from int) (int, bool) {
	if from >= len(s) || s[from] != '{' {
		return 0, false
	}
	depth := 0
	inString := false
	escaped := false
	for i := from; i < len(s); i++ {
		c := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// isWidgetInsert mirrors the detection rule in engine_v4/ops.go:330:
//
//	propType == "widget" || op.Parent == "formation"
//
// for insert ops. Non-insert ops never start a widget group.
func isWidgetInsert(op engine_v4.Op) bool {
	if op.Type != engine_v4.OpInsert {
		return false
	}
	if op.Parent == "formation" {
		return true
	}
	if op.Props != nil {
		if t, ok := op.Props["type"].(string); ok && t == "widget" {
			return true
		}
	}
	return false
}

func isWS(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
