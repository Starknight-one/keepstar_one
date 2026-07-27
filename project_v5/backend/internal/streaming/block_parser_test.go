package streaming

// BlockParser is the correctness keystone of mid-generation streaming: a
// block emitted twice double-renders on the wire; a block emitted from a
// misparsed slice renders junk. These tests feed inputs at hostile split
// points and assert exact-once, exact-bytes yields.

import (
	"encoding/json"
	"fmt"
	"testing"
)

// feedAll feeds the whole input in one fragment and returns all yields.
func feedAll(t *testing.T, p *BlockParser, input string) [][]byte {
	t.Helper()
	return p.Feed(input)
}

// feedBytewise feeds the input one byte at a time — the most hostile
// split pattern possible — collecting every yield.
func feedBytewise(p *BlockParser, input string) [][]byte {
	var out [][]byte
	for i := 0; i < len(input); i++ {
		out = append(out, p.Feed(input[i:i+1])...)
	}
	return out
}

// assertBlocks unmarshals each yield and compares against want (as JSON
// round-trip maps — byte layout of the source is preserved by contract,
// so we also check raw equality when wantRaw is given).
func assertBlockTexts(t *testing.T, got [][]byte, wantTexts []string) {
	t.Helper()
	if len(got) != len(wantTexts) {
		t.Fatalf("yielded %d blocks, want %d: %s", len(got), len(wantTexts), got)
	}
	for i, raw := range got {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("block[%d] is not valid JSON: %v — %s", i, err, raw)
		}
		if text, _ := m["text"].(string); text != wantTexts[i] {
			t.Errorf("block[%d].text = %q, want %q", i, text, wantTexts[i])
		}
	}
}

func TestBlockParserSimpleTwoBlocks(t *testing.T) {
	input := `{"blocks":[{"kind":"text","text":"hello"},{"kind":"text","text":"world"}]}`
	got := feedAll(t, NewBlockParser(), input)
	assertBlockTexts(t, got, []string{"hello", "world"})
}

// TestBlockParserBytewise — every split point, exactly-once yields.
func TestBlockParserBytewise(t *testing.T) {
	input := `{ "blocks" : [ {"kind":"text","text":"a"} , {"kind":"render","preset":"card","display":"inline"} , {"kind":"text","text":"b"} ] }`
	got := feedBytewise(NewBlockParser(), input)
	if len(got) != 3 {
		t.Fatalf("yielded %d blocks, want 3: %s", len(got), got)
	}
	var m map[string]any
	if err := json.Unmarshal(got[1], &m); err != nil {
		t.Fatalf("block[1] invalid: %v", err)
	}
	if m["preset"] != "card" {
		t.Errorf("block[1].preset = %v, want card", m["preset"])
	}
}

// TestBlockParserYieldsAsSoonAsBraceCloses — the streaming property
// itself: the first block must be yielded from the fragment that carries
// its closing brace, NOT when the input completes.
func TestBlockParserYieldsAsSoonAsBraceCloses(t *testing.T) {
	p := NewBlockParser()
	if got := p.Feed(`{"blocks":[{"kind":"text","text":"hi"`); len(got) != 0 {
		t.Fatalf("yielded before closing brace: %s", got)
	}
	got := p.Feed(`}`)
	assertBlockTexts(t, got, []string{"hi"})
	// Nothing new until the second block closes.
	if got := p.Feed(`,{"kind":"text","text":"more"`); len(got) != 0 {
		t.Fatalf("yielded mid-second-block: %s", got)
	}
	got = p.Feed(`}]}`)
	assertBlockTexts(t, got, []string{"more"})
}

// TestBlockParserEscapesAndBracesInStrings — braces, brackets, escaped
// quotes and \u escapes inside string values must not confuse depth
// tracking; the yield preserves them byte-exact.
func TestBlockParserEscapesAndBracesInStrings(t *testing.T) {
	block := `{"kind":"text","text":"a \"quoted\" brace } and ] and \\ and A {"}`
	input := `{"blocks":[` + block + `]}`
	got := feedBytewise(NewBlockParser(), input)
	if len(got) != 1 {
		t.Fatalf("yielded %d blocks, want 1: %s", len(got), got)
	}
	if string(got[0]) != block {
		t.Errorf("raw bytes drifted:\n got %s\nwant %s", got[0], block)
	}
	var m map[string]any
	if err := json.Unmarshal(got[0], &m); err != nil {
		t.Fatalf("yield not valid JSON: %v", err)
	}
	if want := `a "quoted" brace } and ] and \ and A {`; m["text"] != want {
		t.Errorf("text = %q, want %q", m["text"], want)
	}
}

// TestBlockParserSplitMidEscape — fragment boundary right between '\' and
// '"' must not terminate the string.
func TestBlockParserSplitMidEscape(t *testing.T) {
	p := NewBlockParser()
	p.Feed(`{"blocks":[{"kind":"text","text":"say \`)
	got := p.Feed(`"hi\" now"}]}`)
	assertBlockTexts(t, got, []string{`say "hi" now`})
}

// TestBlockParserNestedStructures — ops arrays with nested objects inside
// a render block stay inside that block's yield.
func TestBlockParserNestedStructures(t *testing.T) {
	block := `{"kind":"render","ops":[{"op":"add","node":{"type":"text","children":[{"a":1}]}}],"display":"screen"}`
	input := `{"blocks":[` + block + `,{"kind":"text","text":"after"}]}`
	got := feedBytewise(NewBlockParser(), input)
	if len(got) != 2 {
		t.Fatalf("yielded %d blocks, want 2: %s", len(got), got)
	}
	if string(got[0]) != block {
		t.Errorf("nested block bytes drifted:\n got %s\nwant %s", got[0], block)
	}
	assertBlockTexts(t, got[1:], []string{"after"})
}

// TestBlockParserOtherTopLevelKeys — keys before AND after "blocks" are
// skipped with full value tracking (objects, arrays, strings, scalars).
func TestBlockParserOtherTopLevelKeys(t *testing.T) {
	cases := []string{
		`{"note":"blocks are here","blocks":[{"kind":"text","text":"x"}]}`,
		`{"meta":{"blocks":[1,2]},"blocks":[{"kind":"text","text":"x"}]}`,
		`{"n":42,"ok":true,"z":null,"blocks":[{"kind":"text","text":"x"}]}`,
		`{"list":[[1,2],[3]],"blocks":[{"kind":"text","text":"x"}]}`,
		`{"blocks":[{"kind":"text","text":"x"}],"trailing":{"a":1}}`,
	}
	for i, input := range cases {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got := feedBytewise(NewBlockParser(), input)
			assertBlockTexts(t, got, []string{"x"})
		})
	}
}

// TestBlockParserDecoyBlocksKey — a "blocks" key nested inside another
// top-level value must not open the array; only the top-level one counts.
func TestBlockParserDecoyBlocksKey(t *testing.T) {
	input := `{"meta":{"blocks":[{"kind":"text","text":"DECOY"}]},"blocks":[{"kind":"text","text":"real"}]}`
	got := feedBytewise(NewBlockParser(), input)
	assertBlockTexts(t, got, []string{"real"})
}

// TestBlockParserExactlyOnce — repeated feeding after the array closed
// yields nothing more.
func TestBlockParserExactlyOnce(t *testing.T) {
	p := NewBlockParser()
	got := p.Feed(`{"blocks":[{"kind":"text","text":"once"}]}`)
	assertBlockTexts(t, got, []string{"once"})
	if extra := p.Feed(` `); len(extra) != 0 {
		t.Errorf("post-done feed yielded %s", extra)
	}
	if extra := p.Feed(`{"blocks":[{"kind":"text","text":"again"}]}`); len(extra) != 0 {
		t.Errorf("second document yielded %s — must be exactly-once per turn", extra)
	}
}

// TestBlockParserBrokenInputsStopSilently — structural surprises break
// the parser (no yields, no panic); prior valid yields stand.
func TestBlockParserBrokenInputs(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string // texts yielded before breakage
	}{
		{"non-object element", `{"blocks":["just a string"]}`, nil},
		{"blocks not array", `{"blocks":{"kind":"text"}}`, nil},
		{"garbage start", `not json at all`, nil},
		{"break after one good block", `{"blocks":[{"kind":"text","text":"ok"},42]}`, []string{"ok"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewBlockParser()
			got := feedBytewise(p, tc.input)
			assertBlockTexts(t, got, tc.want)
			if !p.Broken() && tc.name != "" {
				t.Error("parser not marked broken")
			}
			if extra := p.Feed(`{"blocks":[{"kind":"text","text":"late"}]}`); len(extra) != 0 {
				t.Errorf("broken parser yielded %s", extra)
			}
		})
	}
}

// TestBlockParserEmptyArray — {"blocks":[]} yields nothing and finishes
// cleanly (compose_turn's validator rejects it at execute time).
func TestBlockParserEmptyArray(t *testing.T) {
	p := NewBlockParser()
	if got := feedBytewise(p, `{"blocks":[]}`); len(got) != 0 {
		t.Fatalf("empty array yielded %s", got)
	}
	if p.Broken() {
		t.Error("empty array must not break the parser")
	}
}

// TestBlockParserWhitespaceEverywhere — newlines/tabs at every legal
// position (models pretty-print under long inputs).
func TestBlockParserWhitespaceEverywhere(t *testing.T) {
	input := "{\n\t\"blocks\" :\n[\n\t{ \"kind\" : \"text\" ,\n\t  \"text\" : \"spaced\" }\n\t,\n\t{\"kind\":\"text\",\"text\":\"tight\"}\n]\n}"
	got := feedBytewise(NewBlockParser(), input)
	assertBlockTexts(t, got, []string{"spaced", "tight"})
}

// TestBlockParserIncompleteTailNeverYields — a truncated final block (the
// stream died mid-generation) must never yield a partial object.
func TestBlockParserIncompleteTailNeverYields(t *testing.T) {
	p := NewBlockParser()
	got := p.Feed(`{"blocks":[{"kind":"text","text":"whole"},{"kind":"render","preset":"car`)
	assertBlockTexts(t, got, []string{"whole"})
	if extra := p.Feed(``); len(extra) != 0 {
		t.Errorf("truncated tail yielded %s", extra)
	}
}
