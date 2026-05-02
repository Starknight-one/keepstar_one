package engine

// DocumentVersion is the .pen schema version this engine is built against.
const DocumentVersion = "2.10"

// VariableType discriminates VariableDef payloads.
type VariableType string

const (
	VariableTypeBoolean VariableType = "boolean"
	VariableTypeColor   VariableType = "color"
	VariableTypeNumber  VariableType = "number"
	VariableTypeString  VariableType = "string"
)

// ThemedValue is one variant of a variable's value, optionally constrained to
// active theme selections. Match-all-keys semantics: every key in Theme must be
// present in ResolveContext.ActiveThemes with the same value.
type ThemedValue struct {
	Value any               `json:"value"`
	Theme map[string]string `json:"theme,omitempty"`
}

// VariableDef holds the definition of a $variable. Value is either:
//   - a scalar (bool/string/number/Color) — non-themed variable
//   - a []ThemedValue — themed variants
type VariableDef struct {
	Type  VariableType `json:"type"`
	Value any          `json:"value"`
}

// Document is the .pen file root.
type Document struct {
	Version   string                 `json:"version"`
	Themes    map[string][]string    `json:"themes,omitempty"`
	Imports   map[string]string      `json:"imports,omitempty"`
	Variables map[string]VariableDef `json:"variables,omitempty"`
	Children  []Node                 `json:"children"`
}

// NewDocument returns an empty Document at the current schema version.
func NewDocument() *Document {
	return &Document{
		Version:  DocumentVersion,
		Children: []Node{},
	}
}
