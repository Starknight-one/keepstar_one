package streaming

// BlockParser incrementally parses the growing JSON prefix of a
// compose_turn tool input — {"blocks":[{...},{...},...]} — and yields each
// COMPLETE element of the blocks array exactly once, the moment its
// closing brace lands.
//
// Tolerances (unit-tested):
//   - fragments split at arbitrary byte positions (mid-token, mid-escape,
//     mid-key);
//   - whitespace anywhere JSON allows it;
//   - string escapes (\" \\ \uXXXX) and braces/brackets inside strings;
//   - arbitrarily nested objects/arrays inside block objects;
//   - other top-level keys before or after "blocks" (their values are
//     skipped with full depth/string tracking).
//
// Fail-safe: any structural surprise (non-object array element, malformed
// syntax where a specific token is required) marks the parser broken — it
// stops yielding forever. The caller degrades to no early emission and
// compose_turn emits everything at execute time, so a parser bail-out
// costs only perceived latency, never correctness or duplicates.

type parsePhase int

const (
	phaseSeekObject parsePhase = iota // expect the opening '{'
	phaseSeekKey                      // top object level: expect '"', ',' or '}'
	phaseInKey                        // reading a top-level key string
	phaseAfterKey                     // expect ':'
	phaseSeekValue                    // expect the pending key's value
	phaseSkipValue                    // consuming a non-"blocks" value
	phaseInArray                      // inside the blocks array, between elements
	phaseInBlock                      // inside one block object
	phaseDone                         // blocks array (or top object) closed
)

// BlockParser is single-goroutine (fed from the adapter's stream loop).
// Zero value is NOT ready — use NewBlockParser.
type BlockParser struct {
	buf []byte // full accumulated input (compose inputs are small — KBs)
	pos int    // scan cursor into buf

	phase  parsePhase
	broken bool

	key         []byte // raw bytes of the top-level key being read
	keyIsBlocks bool   // pending key == "blocks"
	sawBlocks   bool   // guards a duplicate "blocks" key

	// String/nesting state shared by phaseSkipValue and phaseInBlock.
	depth      int
	inString   bool
	escaped    bool
	scalarSkip bool // phaseSkipValue is consuming an unquoted scalar

	blockStart int // buf offset of the current block object's '{'
}

// NewBlockParser returns a parser positioned before the top-level '{'.
func NewBlockParser() *BlockParser {
	return &BlockParser{phase: phaseSeekObject}
}

// Broken reports whether the parser bailed out on malformed input. Once
// broken, Feed never yields again.
func (p *BlockParser) Broken() bool { return p.broken }

// Feed appends one raw fragment and returns every block object completed
// by it, in order, each as its exact raw JSON bytes (copied — safe to
// retain). Returns nil when no block completed.
func (p *BlockParser) Feed(fragment string) [][]byte {
	if p.broken || p.phase == phaseDone {
		return nil
	}
	p.buf = append(p.buf, fragment...)

	var out [][]byte
	for p.pos < len(p.buf) && !p.broken && p.phase != phaseDone {
		c := p.buf[p.pos]
		switch p.phase {

		case phaseSeekObject:
			switch {
			case isJSONSpace(c):
				p.pos++
			case c == '{':
				p.phase = phaseSeekKey
				p.pos++
			default:
				p.broken = true
			}

		case phaseSeekKey:
			switch {
			case isJSONSpace(c) || c == ',':
				p.pos++
			case c == '"':
				p.key = p.key[:0]
				p.escaped = false
				p.phase = phaseInKey
				p.pos++
			case c == '}':
				p.phase = phaseDone
				p.pos++
			default:
				p.broken = true
			}

		case phaseInKey:
			switch {
			case p.escaped:
				p.key = append(p.key, c)
				p.escaped = false
				p.pos++
			case c == '\\':
				p.escaped = true
				p.pos++
			case c == '"':
				// Literal byte match — an escaped spelling of "blocks"
				// simply doesn't match and the value gets skipped (safe).
				p.keyIsBlocks = !p.sawBlocks && string(p.key) == "blocks"
				p.phase = phaseAfterKey
				p.pos++
			default:
				p.key = append(p.key, c)
				p.pos++
			}

		case phaseAfterKey:
			switch {
			case isJSONSpace(c):
				p.pos++
			case c == ':':
				p.phase = phaseSeekValue
				p.pos++
			default:
				p.broken = true
			}

		case phaseSeekValue:
			if isJSONSpace(c) {
				p.pos++
				continue
			}
			if p.keyIsBlocks {
				if c == '[' {
					p.sawBlocks = true
					p.keyIsBlocks = false
					p.phase = phaseInArray
					p.pos++
					continue
				}
				p.broken = true // "blocks" must be an array
				continue
			}
			// Some other top-level key: skip its whole value.
			p.depth, p.inString, p.escaped, p.scalarSkip = 0, false, false, false
			switch c {
			case '{', '[':
				p.depth = 1
			case '"':
				p.inString = true
			default:
				p.scalarSkip = true // number / true / false / null
			}
			p.phase = phaseSkipValue
			p.pos++

		case phaseSkipValue:
			if p.inString {
				switch {
				case p.escaped:
					p.escaped = false
				case c == '\\':
					p.escaped = true
				case c == '"':
					p.inString = false
					if p.depth == 0 { // string scalar finished
						p.phase = phaseSeekKey
					}
				}
				p.pos++
				continue
			}
			if p.scalarSkip {
				if c == ',' || c == '}' {
					// Terminator belongs to phaseSeekKey — do not consume.
					p.scalarSkip = false
					p.phase = phaseSeekKey
					continue
				}
				p.pos++
				continue
			}
			switch c {
			case '"':
				p.inString = true
			case '{', '[':
				p.depth++
			case '}', ']':
				p.depth--
				if p.depth == 0 {
					p.phase = phaseSeekKey
				}
			}
			p.pos++

		case phaseInArray:
			switch {
			case isJSONSpace(c) || c == ',':
				p.pos++
			case c == ']':
				p.phase = phaseDone
				p.pos++
			case c == '{':
				p.blockStart = p.pos
				p.depth = 1
				p.inString, p.escaped = false, false
				p.phase = phaseInBlock
				p.pos++
			default:
				p.broken = true // non-object array element — bail out
			}

		case phaseInBlock:
			if p.inString {
				switch {
				case p.escaped:
					p.escaped = false
				case c == '\\':
					p.escaped = true
				case c == '"':
					p.inString = false
				}
				p.pos++
				continue
			}
			switch c {
			case '"':
				p.inString = true
			case '{', '[':
				p.depth++
			case '}', ']':
				p.depth--
				if p.depth == 0 {
					if c != '}' {
						p.broken = true // '[' ... ']' imbalance
						continue
					}
					blk := make([]byte, p.pos+1-p.blockStart)
					copy(blk, p.buf[p.blockStart:p.pos+1])
					out = append(out, blk)
					p.phase = phaseInArray
				}
			}
			p.pos++
		}
	}
	return out
}

// isJSONSpace reports whether c is JSON insignificant whitespace.
func isJSONSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
