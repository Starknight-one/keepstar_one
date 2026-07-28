package operations

import (
	"context"
	"encoding/json"
	"testing"

	"keepstar_v5/internal/domain"
)

// The tenant that broke the live run: the seeded booking_form submits
// `book_showing` over `preferredTime`/`listingId`, the model named the
// instance `book_viewing` over `viewingTime`/`propertyInterest`.
const liveBookingForm = `{
  "version": "2.10",
  "children": [{
    "type": "frame", "id": "booking-form", "formId": "booking_form",
    "children": [
      {"type": "input", "id": "l", "name": "listingId", "operationField": "link_field", "inputType": "hidden"},
      {"type": "datetime", "id": "t", "name": "preferredTime", "operationField": "datetime_field"},
      {"type": "input", "id": "p", "name": "phone", "inputType": "tel"},
      {"type": "submit", "id": "s", "label": "Book showing",
       "action": {"kind": "operation_invoke", "operation": "book_showing", "operationKind": "schedule_slot"}}
    ]
  }]
}`

// stubPresets serves one document as both a published preset and a list.
type stubPresets struct {
	doc  string
	name string
}

func (s *stubPresets) GetPublishedPreset(_ context.Context, _, name string) (*domain.Preset, error) {
	return &domain.Preset{Name: name, DocumentJSON: json.RawMessage(s.doc)}, nil
}

func (s *stubPresets) ListPublishedPresets(_ context.Context, _ string) ([]domain.Preset, error) {
	return []domain.Preset{{Name: s.name, DocumentJSON: json.RawMessage(s.doc)}}, nil
}

// stubResolver answers ResolveForKind from a fixed table.
type stubResolver struct {
	name string
	cfg  map[string]any
	ok   bool
}

func (s *stubResolver) ResolveForKind(_ context.Context, _, _ string, _ domain.OperationKind) (ResolvedOperation, bool) {
	if !s.ok {
		return ResolvedOperation{}, false
	}
	return ResolvedOperation{Name: s.name, Config: s.cfg}, true
}

// bindOnce runs the binder and returns the parsed submit action + the
// input nodes by id.
func bindOnce(t *testing.T, doc string, r OperationKindResolver) (map[string]any, map[string]map[string]any) {
	t.Helper()
	b := NewPresetOperationBinder(&stubPresets{doc: doc, name: "booking_form"}, testLogger())
	if r != nil {
		b.SetResolver(r)
	}
	p, err := b.GetPublishedPreset(context.Background(), "store", "booking_form")
	if err != nil {
		t.Fatalf("GetPublishedPreset: %v", err)
	}
	var parsed struct {
		Children []struct {
			Children []map[string]any `json:"children"`
		} `json:"children"`
	}
	if err := json.Unmarshal(p.DocumentJSON, &parsed); err != nil {
		t.Fatalf("bound document is not valid JSON: %v", err)
	}
	nodes := map[string]map[string]any{}
	var action map[string]any
	for _, n := range parsed.Children[0].Children {
		id, _ := n["id"].(string)
		nodes[id] = n
		if act, ok := n["action"].(map[string]any); ok {
			action = act
		}
	}
	return action, nodes
}

// A library form must submit the operation THIS tenant enabled, posting
// the field names that operation's config actually reads — otherwise the
// visitor's booking dies as "unknown operation" or "invalid".
func TestBinderBindsOperationAndFieldNamesToTenantInstance(t *testing.T) {
	action, nodes := bindOnce(t, liveBookingForm, &stubResolver{
		name: "book_viewing", ok: true,
		cfg: map[string]any{"datetime_field": "viewingTime", "link_field": "propertyInterest"},
	})

	if got := action["operation"]; got != "book_viewing" {
		t.Errorf("operation = %v, want book_viewing (tenant instance)", got)
	}
	if got := nodes["t"]["name"]; got != "viewingTime" {
		t.Errorf("datetime input name = %v, want viewingTime (instance config)", got)
	}
	if got := nodes["l"]["name"]; got != "propertyInterest" {
		t.Errorf("link input name = %v, want propertyInterest (instance config)", got)
	}
	if got := nodes["p"]["name"]; got != "phone" {
		t.Errorf("unannotated input renamed to %v — only role-annotated inputs bind", got)
	}
}

// Annotations are authoring intent, not wire data: they must never reach
// the browser, bound or not.
func TestBinderStripsAnnotationsEvenWhenUnresolved(t *testing.T) {
	for _, tc := range []struct {
		name     string
		resolver OperationKindResolver
	}{
		{"unresolved", &stubResolver{ok: false}},
		{"no resolver wired", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			action, nodes := bindOnce(t, liveBookingForm, tc.resolver)
			if _, present := action[AnnotationOperationKind]; present {
				t.Error("operationKind leaked to the wire")
			}
			for _, id := range []string{"t", "l"} {
				if _, present := nodes[id][AnnotationOperationField]; present {
					t.Errorf("operationField leaked to the wire on node %q", id)
				}
			}
			// Unresolvable intent keeps the authored name so the denial
			// names what the author asked for.
			if got := action["operation"]; got != "book_showing" {
				t.Errorf("operation = %v, want the authored book_showing", got)
			}
			if got := nodes["t"]["name"]; got != "preferredTime" {
				t.Errorf("datetime input name = %v, want the authored preferredTime", got)
			}
		})
	}
}

// Documents without operation intent must come back byte-identical — the
// binder sits on every preset read.
func TestBinderPassesThroughUnannotatedDocuments(t *testing.T) {
	const plain = `{"version":"2.10","children":[{"type":"frame","id":"card","children":[{"type":"text","id":"t","content":"hi"}]}]}`
	b := NewPresetOperationBinder(&stubPresets{doc: plain, name: "product_card"}, testLogger())
	b.SetResolver(&stubResolver{name: "x", ok: true})
	p, err := b.GetPublishedPreset(context.Background(), "store", "product_card")
	if err != nil {
		t.Fatalf("GetPublishedPreset: %v", err)
	}
	if string(p.DocumentJSON) != plain {
		t.Errorf("unannotated document rewritten:\n got %s\nwant %s", p.DocumentJSON, plain)
	}
}

func TestBinderBindsListedPresets(t *testing.T) {
	b := NewPresetOperationBinder(&stubPresets{doc: liveBookingForm, name: "booking_form"}, testLogger())
	b.SetResolver(&stubResolver{name: "book_viewing", ok: true})
	list, err := b.ListPublishedPresets(context.Background(), "store")
	if err != nil {
		t.Fatalf("ListPublishedPresets: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list = %d presets, want 1", len(list))
	}
	if !jsonContains(list[0].DocumentJSON, "book_viewing") {
		t.Errorf("listed preset not bound: %s", list[0].DocumentJSON)
	}
}

func jsonContains(raw json.RawMessage, needle string) bool {
	return len(raw) > 0 && json.Valid(raw) && contains(string(raw), needle)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
