package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"keepstar_v5/internal/domain"
)

// In-memory StatePort stub. Only the methods Action handler touches
// are wired; everything else panics so test failures surface fast.
type fakeStatePort struct {
	state    *domain.SessionState
	notFound bool

	updates int
}

func (f *fakeStatePort) GetState(_ context.Context, _ string) (*domain.SessionState, error) {
	if f.notFound {
		return nil, errFake("not found")
	}
	return f.state, nil
}

func (f *fakeStatePort) UpdateActions(_ context.Context, _ string, actions domain.StateActions, _ domain.DeltaInfo) (int, error) {
	f.state.Actions = actions
	f.updates++
	return 1, nil
}

// Unused interface methods — panic so misuse is loud.
func (f *fakeStatePort) CreateState(context.Context, string) (*domain.SessionState, error) {
	panic("CreateState unused")
}
func (f *fakeStatePort) UpdateState(context.Context, *domain.SessionState) error {
	panic("UpdateState unused")
}
func (f *fakeStatePort) AddDelta(context.Context, string, *domain.Delta) (int, error) {
	panic("AddDelta unused")
}
func (f *fakeStatePort) GetDeltas(context.Context, string) ([]domain.Delta, error) {
	panic("GetDeltas unused")
}
func (f *fakeStatePort) GetDeltasSince(context.Context, string, int) ([]domain.Delta, error) {
	panic("GetDeltasSince unused")
}
func (f *fakeStatePort) GetDeltasUntil(context.Context, string, int) ([]domain.Delta, error) {
	panic("GetDeltasUntil unused")
}
func (f *fakeStatePort) UpdateData(context.Context, string, domain.StateData, domain.StateMeta, domain.DeltaInfo) (int, error) {
	panic("UpdateData unused")
}
func (f *fakeStatePort) UpdateTemplate(context.Context, string, map[string]interface{}, domain.DeltaInfo) (int, error) {
	panic("UpdateTemplate unused")
}
func (f *fakeStatePort) UpdateView(context.Context, string, domain.ViewState, []domain.ViewSnapshot, domain.DeltaInfo) (int, error) {
	panic("UpdateView unused")
}
func (f *fakeStatePort) AppendConversation(context.Context, string, []domain.LLMMessage) error {
	panic("AppendConversation unused")
}
func (f *fakeStatePort) AppendAgent2History(context.Context, string, []domain.LLMMessage) error {
	panic("AppendAgent2History unused")
}
func (f *fakeStatePort) PushView(context.Context, string, *domain.ViewSnapshot) error {
	panic("PushView unused")
}
func (f *fakeStatePort) PopView(context.Context, string) (*domain.ViewSnapshot, error) {
	panic("PopView unused")
}
func (f *fakeStatePort) GetViewStack(context.Context, string) ([]domain.ViewSnapshot, error) {
	panic("GetViewStack unused")
}

type fakeErr string

func errFake(msg string) error           { return fakeErr(msg) }
func (e fakeErr) Error() string          { return string(e) }
func newFakeState() *domain.SessionState { return &domain.SessionState{Actions: domain.StateActions{}} }

func newActionRouter(state *fakeStatePort) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/actions", NewActionHandler(state).Action)
	return mux
}

func TestActionHandler_LikeAddsId(t *testing.T) {
	state := &fakeStatePort{state: newFakeState()}
	srv := httptest.NewServer(newActionRouter(state))
	defer srv.Close()

	body := `{"sessionId":"s1","kind":"like","entity":{"type":"product","id":"p-1"}}`
	resp, err := http.Post(srv.URL+"/api/v1/actions", "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out struct {
		Success bool `json:"success"`
		Actions struct {
			LikedIds []string `json:"likedIds"`
		} `json:"actions"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if !out.Success {
		t.Fatal("success=false")
	}
	if len(out.Actions.LikedIds) != 1 || out.Actions.LikedIds[0] != "p-1" {
		t.Fatalf("expected liked=[p-1], got %+v", out.Actions.LikedIds)
	}
}

func TestActionHandler_UnlikeRemovesId(t *testing.T) {
	state := &fakeStatePort{state: &domain.SessionState{
		Actions: domain.StateActions{LikedIds: []string{"p-1", "p-2"}},
	}}
	srv := httptest.NewServer(newActionRouter(state))
	defer srv.Close()

	body := `{"sessionId":"s1","kind":"unlike","entity":{"type":"product","id":"p-1"}}`
	resp, err := http.Post(srv.URL+"/api/v1/actions", "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Actions struct {
			LikedIds []string `json:"likedIds"`
		} `json:"actions"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Actions.LikedIds) != 1 || out.Actions.LikedIds[0] != "p-2" {
		t.Fatalf("expected [p-2], got %+v", out.Actions.LikedIds)
	}
}

func TestActionHandler_CartAddIncrementsExisting(t *testing.T) {
	state := &fakeStatePort{state: &domain.SessionState{
		Actions: domain.StateActions{
			CartItems: []domain.CartItem{{EntityType: domain.EntityTypeProduct, EntityId: "p-1", Quantity: 1}},
		},
	}}
	srv := httptest.NewServer(newActionRouter(state))
	defer srv.Close()

	body := `{"sessionId":"s1","kind":"cart_add","entity":{"type":"product","id":"p-1"},"params":{"quantity":2}}`
	resp, err := http.Post(srv.URL+"/api/v1/actions", "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Actions struct {
			CartItems []domain.CartItem `json:"cartItems"`
		} `json:"actions"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Actions.CartItems) != 1 || out.Actions.CartItems[0].Quantity != 3 {
		t.Fatalf("expected quantity 3, got %+v", out.Actions.CartItems)
	}
}

func TestActionHandler_SyncSkipsBody(t *testing.T) {
	state := &fakeStatePort{state: newFakeState()}
	srv := httptest.NewServer(newActionRouter(state))
	defer srv.Close()

	body := `{"sessionId":"s1","kind":"like","entity":{"type":"product","id":"p-1"}}`
	resp, err := http.Post(srv.URL+"/api/v1/actions?sync=true", "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

func TestActionHandler_RejectsUnknownKind(t *testing.T) {
	state := &fakeStatePort{state: newFakeState()}
	srv := httptest.NewServer(newActionRouter(state))
	defer srv.Close()

	body := `{"sessionId":"s1","kind":"bogus","entity":{"type":"product","id":"p-1"}}`
	resp, err := http.Post(srv.URL+"/api/v1/actions", "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestActionHandler_RejectsClientHandledKind(t *testing.T) {
	state := &fakeStatePort{state: newFakeState()}
	srv := httptest.NewServer(newActionRouter(state))
	defer srv.Close()

	body := `{"sessionId":"s1","kind":"drill_detail","entity":{"type":"product","id":"p-1"}}`
	resp, err := http.Post(srv.URL+"/api/v1/actions", "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 (client-handled kind), got %d", resp.StatusCode)
	}
}
