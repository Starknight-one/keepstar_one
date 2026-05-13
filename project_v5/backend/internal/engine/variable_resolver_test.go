package engine

import "testing"

func TestResolvePassthrough(t *testing.T) {
	r := NewVariableResolver()
	doc := NewDocument()
	ctx := &ResolveContext{Document: doc, ActiveThemes: nil}

	if got := r.ResolveColor("#FFF", ctx); got != "#FFF" {
		t.Errorf("plain color passthrough failed: %q", got)
	}
	if got := r.ResolveNumber(42, ctx); got != 42 {
		t.Errorf("plain number passthrough failed: %v", got)
	}
}

func TestResolveSimpleVariable(t *testing.T) {
	r := NewVariableResolver()
	doc := NewDocument()
	doc.Variables = map[string]VariableDef{
		"primary": {Type: VariableTypeColor, Value: "#FF0000"},
	}
	ctx := &ResolveContext{Document: doc}
	if got := r.ResolveColor("$primary", ctx); got != "#FF0000" {
		t.Errorf("$primary = %q, want #FF0000", got)
	}
}

func TestResolveThemed(t *testing.T) {
	r := NewVariableResolver()
	doc := NewDocument()
	doc.Variables = map[string]VariableDef{
		"accent": {
			Type: VariableTypeColor,
			Value: []ThemedValue{
				{Value: "#FF0000", Theme: map[string]string{"mode": "light"}},
				{Value: "#00FF00", Theme: map[string]string{"mode": "dark"}},
			},
		},
	}
	ctxLight := &ResolveContext{Document: doc, ActiveThemes: map[string]string{"mode": "light"}}
	ctxDark := &ResolveContext{Document: doc, ActiveThemes: map[string]string{"mode": "dark"}}
	if got := r.ResolveColor("$accent", ctxLight); got != "#FF0000" {
		t.Errorf("light: %q, want #FF0000", got)
	}
	if got := r.ResolveColor("$accent", ctxDark); got != "#00FF00" {
		t.Errorf("dark: %q, want #00FF00", got)
	}
}

func TestResolveFallback(t *testing.T) {
	r := NewVariableResolver()
	doc := NewDocument()
	ctx := &ResolveContext{Document: doc}
	// Unknown variable → magenta for color
	if got := r.ResolveColor("$missing", ctx); got != "#FF00FFFF" {
		t.Errorf("missing var color = %q, want #FF00FFFF", got)
	}
	// Unknown variable → 0 for number
	if got := r.ResolveNumber("$missing", ctx); got != 0 {
		t.Errorf("missing var number = %v, want 0", got)
	}
}

func TestResolveChainedSingleLevel(t *testing.T) {
	r := NewVariableResolver()
	doc := NewDocument()
	doc.Variables = map[string]VariableDef{
		"a": {Type: VariableTypeColor, Value: "$b"},
		"b": {Type: VariableTypeColor, Value: "#123456"},
	}
	ctx := &ResolveContext{Document: doc}
	if got := r.ResolveColor("$a", ctx); got != "#123456" {
		t.Errorf("$a → $b → #123456, got %q", got)
	}
}
