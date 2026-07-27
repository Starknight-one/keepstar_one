package domain

// Turn protocol (R9 + final owner decision 3): a turn composed via the
// compose_turn operation is an ordered list of TurnBlocks — text paragraphs
// interleaved with rendered documents. Wire: blocks stream to the widget as
// they are produced (`event: block` SSE frames) AND ride the terminal
// `result` frame / POST /pipeline JSON body as `blocks:[TurnBlock]` next to
// `document`. Absent blocks = legacy single-document turn. The word
// "segment" does not appear in any contract.

// TurnBlock kinds.
const (
	BlockKindText     = "text"
	BlockKindDocument = "document"
)

// TurnBlock display targets.
const (
	DisplayInline = "inline" // rendered inside the chat column
	DisplayScreen = "screen" // becomes the main surface template
)

// TurnBlock is one ordered unit of a composed turn.
type TurnBlock struct {
	BlockID  string                 `json:"blockId"`
	Kind     string                 `json:"kind"` // "text" | "document"
	Text     string                 `json:"text,omitempty"`
	Document map[string]interface{} `json:"document,omitempty"`
	Display  string                 `json:"display,omitempty"` // "inline" | "screen" (document blocks)
}
