package usecases

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// upsertingInbox extends the minimum InboxPort surface we need for the
// orchestrator: real Upsert that stores rows (so the apply step finds them).
type upsertingInbox struct {
	fakeInbox
	upsertCalls int
}

func newUpsertingInbox() *upsertingInbox {
	return &upsertingInbox{fakeInbox: fakeInbox{applied: map[string]bool{}}}
}

func (u *upsertingInbox) Upsert(_ context.Context, item *domain.InboxItem) (bool, error) {
	u.upsertCalls++
	item.ID = item.ExternalID // deterministic id for tests
	u.items = append(u.items, item)
	return true, nil
}

// fakeRateLimit implements ApplyRateLimitPort. Returns the seeded time and
// records each query so tests can verify the orchestrator consults the port.
type fakeRateLimit struct {
	lastAt   time.Time
	queries  int
	queryErr error
}

func (f *fakeRateLimit) LastAppliedAt(_ context.Context, _ string) (time.Time, error) {
	f.queries++
	if f.queryErr != nil {
		return time.Time{}, f.queryErr
	}
	return f.lastAt, nil
}

// mkOrchestrator wires InboxUseCase + ApplyV2 + UpdateOrchestrator with
// shared fakes. Returns handles so tests can pre-seed state.
func mkOrchestrator(art *domain.MappingArtifactV3) (
	*UpdateOrchestrator,
	*upsertingInbox,
	*fakeWriter,
	*fakeActionLog,
	*fakeRateLimit,
) {
	inbox := newUpsertingInbox()
	writer := newFakeWriter()
	log := &fakeActionLog{}
	rate := &fakeRateLimit{}
	llog := logger.New("error")

	inboxUC := NewInboxUseCase(inbox, log, llog)
	applyUC := NewApplyV2(inbox, &fakeArtifact{art: art}, writer, log, nil, llog)

	o := NewUpdateOrchestrator(inboxUC, applyUC, nil, log, rate, llog)
	return o, inbox, writer, log, rate
}

func TestOnWebhook_OutsideWindowApplies(t *testing.T) {
	o, inbox, writer, log, rate := mkOrchestrator(cosmeticsArtifact())
	seedCosmeticsAlias(writer)
	rate.lastAt = time.Now().Add(-25 * time.Hour) // > 24h ago → window closed

	payload := mustJSON(map[string]any{
		"title":        "Cream",
		"vendor":       "Brand",
		"product_type": "Cream",
		"variants":     []any{map[string]any{"sku": "C-1"}},
	})

	outcome, err := o.OnWebhook(context.Background(), WebhookEvent{
		TenantID:   "t-test",
		Source:     "shopify",
		ExternalID: "gid://shopify/Product/1",
		Verb:       "updated",
		Payload:    payload,
	})
	if err != nil {
		t.Fatalf("OnWebhook: %v", err)
	}
	if outcome != "applied" {
		t.Errorf("outcome = %q, want applied", outcome)
	}
	if rate.queries != 1 {
		t.Errorf("rate.queries = %d, want 1", rate.queries)
	}
	if inbox.upsertCalls != 1 {
		t.Errorf("inbox.upsertCalls = %d, want 1", inbox.upsertCalls)
	}
	if len(writer.listings) != 1 {
		t.Errorf("listings written = %d, want 1 (apply happened)", len(writer.listings))
	}
	if !hasAction(log.entries, "webhook_received") {
		t.Error("webhook_received not logged")
	}
}

func TestOnWebhook_InsideWindowAbsorbsOnly(t *testing.T) {
	o, inbox, writer, log, rate := mkOrchestrator(cosmeticsArtifact())
	rate.lastAt = time.Now().Add(-1 * time.Hour) // inside 24h window

	payload := mustJSON(map[string]any{"title": "X", "variants": []any{map[string]any{"sku": "X-1"}}})
	outcome, err := o.OnWebhook(context.Background(), WebhookEvent{
		TenantID:   "t-test",
		Source:     "shopify",
		ExternalID: "gid://shopify/Product/9",
		Verb:       "updated",
		Payload:    payload,
	})
	if err != nil {
		t.Fatalf("OnWebhook: %v", err)
	}
	if outcome != "absorbed_only" {
		t.Errorf("outcome = %q, want absorbed_only", outcome)
	}
	if inbox.upsertCalls != 1 {
		t.Errorf("inbox upserted = %d, want 1", inbox.upsertCalls)
	}
	if len(writer.listings) != 0 {
		t.Errorf("apply ran (listings=%d) — should be absorbed_only", len(writer.listings))
	}
	if !hasAction(log.entries, "webhook_received") {
		t.Error("webhook_received not logged even in absorbed_only path")
	}
}

func TestOnWebhook_FirstEventNoPriorAppliesImmediately(t *testing.T) {
	o, _, writer, _, rate := mkOrchestrator(cosmeticsArtifact())
	seedCosmeticsAlias(writer)
	// rate.lastAt is zero value → IsZero() true → apply proceeds.

	payload := mustJSON(map[string]any{
		"title":        "Toner",
		"vendor":       "Brand",
		"product_type": "Toner",
		"variants":     []any{map[string]any{"sku": "T-1"}},
	})
	outcome, err := o.OnWebhook(context.Background(), WebhookEvent{
		TenantID:   "t-test",
		Source:     "shopify",
		ExternalID: "gid://shopify/Product/1",
		Verb:       "created",
		Payload:    payload,
	})
	if err != nil {
		t.Fatalf("OnWebhook: %v", err)
	}
	if outcome != "applied" {
		t.Errorf("outcome = %q, want applied (zero rate.lastAt should not block)", outcome)
	}
	if rate.queries != 1 {
		t.Errorf("rate consulted %d times, want 1", rate.queries)
	}
}

func TestOnWebhook_RejectsEmptyTenantOrExternalID(t *testing.T) {
	o, _, _, _, _ := mkOrchestrator(cosmeticsArtifact())
	if _, err := o.OnWebhook(context.Background(), WebhookEvent{TenantID: "", ExternalID: "x"}); err == nil {
		t.Error("expected err on empty tenant_id")
	}
	if _, err := o.OnWebhook(context.Background(), WebhookEvent{TenantID: "t", ExternalID: ""}); err == nil {
		t.Error("expected err on empty external_id")
	}
}

func TestManualSync_BypassesRateLimit(t *testing.T) {
	o, _, writer, log, rate := mkOrchestrator(cosmeticsArtifact())
	seedCosmeticsAlias(writer)
	rate.lastAt = time.Now() // just-now apply — would block webhook path

	// Seed an inbox row that apply will pick up.
	body, _ := json.Marshal(map[string]any{
		"title":        "Cream",
		"vendor":       "Brand",
		"product_type": "Cream",
		"variants":     []any{map[string]any{"sku": "MS-1"}},
	})
	o.inbox.inbox.(*upsertingInbox).items = append(o.inbox.inbox.(*upsertingInbox).items, &domain.InboxItem{
		ID: "ms-1", TenantID: "t-test", SourceKind: domain.InboxSourceShopify,
		ExternalID: "gid://1", Raw: body, FetchedAt: time.Now(),
	})

	res, err := o.ManualSync(context.Background(), "t-test")
	if err != nil {
		t.Fatalf("ManualSync: %v", err)
	}
	if res.Applied != 1 {
		t.Errorf("applied = %d, want 1 (manual ignores rate window)", res.Applied)
	}
	if rate.queries != 0 {
		t.Errorf("ManualSync should not consult rate-limit (queries=%d)", rate.queries)
	}
	if !hasAction(log.entries, "manual_sync") {
		t.Error("manual_sync action not logged")
	}
}

func TestManualSync_RejectsEmptyTenant(t *testing.T) {
	o, _, _, _, _ := mkOrchestrator(cosmeticsArtifact())
	if _, err := o.ManualSync(context.Background(), ""); err == nil {
		t.Error("expected err on empty tenant_id")
	}
}

func TestOnMappingMiss_NoDiscoveryWired_ReturnsError(t *testing.T) {
	o, _, _, _, _ := mkOrchestrator(cosmeticsArtifact())
	err := o.OnMappingMiss(context.Background(), "t-test", MappingMissDetails{Reason: "x"})
	if err == nil {
		t.Error("expected err when discovery wired as nil")
	}
}

// ---- helpers ----

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func hasAction(entries []*ports.TenantActionLogEntry, action string) bool {
	for _, e := range entries {
		if e.Action == action {
			return true
		}
	}
	return false
}
