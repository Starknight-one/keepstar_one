package anthropic

// hookTracker tests: fragments of the named tool's input are forwarded in
// order; other blocks (text, other tools, a SECOND block of the same
// tool) are not. Events are built by unmarshalling real wire JSON so the
// test exercises the same union decoding the live stream loop sees.

import (
	"encoding/json"
	"fmt"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
)

// ev unmarshals one wire-shaped stream event.
func ev(t *testing.T, raw string) sdk.MessageStreamEventUnion {
	t.Helper()
	var e sdk.MessageStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("bad test event %s: %v", raw, err)
	}
	return e
}

func startToolUse(t *testing.T, index int, name string) sdk.MessageStreamEventUnion {
	return ev(t, fmt.Sprintf(
		`{"type":"content_block_start","index":%d,"content_block":{"type":"tool_use","id":"tu_%d","name":"%s","input":{}}}`,
		index, index, name))
}

func inputDelta(t *testing.T, index int, partial string) sdk.MessageStreamEventUnion {
	b, _ := json.Marshal(partial)
	return ev(t, fmt.Sprintf(
		`{"type":"content_block_delta","index":%d,"delta":{"type":"input_json_delta","partial_json":%s}}`,
		index, b))
}

func stopBlock(t *testing.T, index int) sdk.MessageStreamEventUnion {
	return ev(t, fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, index))
}

func TestHookTrackerForwardsNamedToolFragmentsInOrder(t *testing.T) {
	var got []string
	tr := &hookTracker{tool: "compose_turn", onFragment: func(s string) { got = append(got, s) }}

	// Block 0: a text block — no forwarding.
	tr.observe(ev(t, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`))
	tr.observe(ev(t, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`))
	tr.observe(stopBlock(t, 0))

	// Block 1: another tool — no forwarding.
	tr.observe(startToolUse(t, 1, "visual_assembly"))
	tr.observe(inputDelta(t, 1, `{"mode":"rebuild"}`))
	tr.observe(stopBlock(t, 1))

	// Block 2: compose_turn — forwarded fragment by fragment.
	tr.observe(startToolUse(t, 2, "compose_turn"))
	tr.observe(inputDelta(t, 2, `{"blocks":[{"kind":"te`))
	tr.observe(inputDelta(t, 2, `xt","text":"hi"}]}`))
	tr.observe(stopBlock(t, 2))

	if len(got) != 2 || got[0] != `{"blocks":[{"kind":"te` || got[1] != `xt","text":"hi"}]}` {
		t.Fatalf("forwarded fragments = %q", got)
	}
}

// TestHookTrackerIgnoresSecondMatchingBlock — only the FIRST compose_turn
// block feeds the hook; a duplicate call's fragments must not interleave
// into the same parser.
func TestHookTrackerIgnoresSecondMatchingBlock(t *testing.T) {
	var got []string
	tr := &hookTracker{tool: "compose_turn", onFragment: func(s string) { got = append(got, s) }}

	tr.observe(startToolUse(t, 0, "compose_turn"))
	tr.observe(inputDelta(t, 0, `{"blocks":[]}`))
	tr.observe(stopBlock(t, 0))

	tr.observe(startToolUse(t, 1, "compose_turn"))
	tr.observe(inputDelta(t, 1, `{"blocks":[{"kind":"text","text":"dup"}]}`))
	tr.observe(stopBlock(t, 1))

	if len(got) != 1 || got[0] != `{"blocks":[]}` {
		t.Fatalf("forwarded fragments = %q, want only the first block's", got)
	}
}

// TestHookTrackerNoMatchNoCalls — a turn without the named tool never
// touches the hook.
func TestHookTrackerNoMatchNoCalls(t *testing.T) {
	calls := 0
	tr := &hookTracker{tool: "compose_turn", onFragment: func(string) { calls++ }}
	tr.observe(startToolUse(t, 0, "visual_assembly"))
	tr.observe(inputDelta(t, 0, `{"mode":"rebuild"}`))
	tr.observe(stopBlock(t, 0))
	if calls != 0 {
		t.Fatalf("hook called %d times, want 0", calls)
	}
}
