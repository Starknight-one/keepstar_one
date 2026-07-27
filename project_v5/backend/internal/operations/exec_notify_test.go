package operations

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"keepstar_v5/internal/domain"
)

func TestNotifyExecutorRuntimeChannel(t *testing.T) {
	store := &fakeNotifStore{}
	ex := NewNotifyExecutor(store, nil)

	res, err := ex.Execute(context.Background(), domain.OperationContext{
		TenantID: "tnt-1", Role: domain.RoleSystem,
		Config: map[string]any{"channel": "runtime"},
	}, map[string]any{
		"title": "Showing request — Sea View 2BR",
		"body":  "Ann · +14155550101",
		"ref":   "rec-1",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeOK || res.Summary != "ok: notification stored" {
		t.Fatalf("result = %+v", res)
	}
	if res.Output["notificationId"] != "ntf-1" {
		t.Errorf("output = %#v", res.Output)
	}
	if len(store.stored) != 1 {
		t.Fatalf("stored = %d", len(store.stored))
	}
	n := store.stored[0]
	if n.TenantID != "tnt-1" || n.Title != "Showing request — Sea View 2BR" || n.RecordID != "rec-1" {
		t.Errorf("notification = %+v", n)
	}
}

// An empty channel defaults to runtime — a bare-enabled notify instance
// still delivers.
func TestNotifyExecutorDefaultChannel(t *testing.T) {
	store := &fakeNotifStore{}
	ex := NewNotifyExecutor(store, nil)

	res, err := ex.Execute(context.Background(), domain.OperationContext{TenantID: "tnt-1"},
		map[string]any{"title": "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeOK || len(store.stored) != 1 {
		t.Fatalf("result = %+v stored=%d", res, len(store.stored))
	}
}

func TestNotifyExecutorRequiresTitleAndTenant(t *testing.T) {
	ex := NewNotifyExecutor(&fakeNotifStore{}, nil)

	res, _ := ex.Execute(context.Background(), domain.OperationContext{TenantID: "tnt-1"}, map[string]any{})
	if res.Outcome != domain.OutcomeInvalid {
		t.Errorf("missing title → %q", res.Outcome)
	}

	res, _ = ex.Execute(context.Background(), domain.OperationContext{}, map[string]any{"title": "x"})
	if res.Outcome != domain.OutcomeError {
		t.Errorf("missing tenant → %q", res.Outcome)
	}
}

func TestNotifyExecutorWebhookChannel(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ex := NewNotifyExecutor(&fakeNotifStore{}, srv.Client())
	res, err := ex.Execute(context.Background(), domain.OperationContext{
		TenantID: "tnt-1",
		Config:   map[string]any{"channel": "webhook", "url": srv.URL},
	}, map[string]any{"title": "New lead", "ref": "rec-1"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeOK || res.Summary != "ok: notification posted" {
		t.Fatalf("result = %+v", res)
	}
	if got["title"] != "New lead" || got["tenantId"] != "tnt-1" || got["ref"] != "rec-1" {
		t.Errorf("webhook payload = %#v", got)
	}
}

func TestNotifyExecutorWebhookFailureIsErrorOutcome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ex := NewNotifyExecutor(&fakeNotifStore{}, srv.Client())
	res, err := ex.Execute(context.Background(), domain.OperationContext{
		TenantID: "tnt-1",
		Config:   map[string]any{"channel": "webhook", "url": srv.URL},
	}, map[string]any{"title": "New lead"})
	if err != nil {
		t.Fatalf("webhook failure must not be a transport error: %v", err)
	}
	if res.Outcome != domain.OutcomeError {
		t.Errorf("outcome = %q", res.Outcome)
	}
}
