package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// hashingInbox is a stricter fake than upsertingInbox: it actually compares
// PayloadHash on Upsert (mirroring the real adapter contract documented in
// docs/pre_launch_scenarios.md sec 16, sc 104). Same-hash repeat returns
// changed=false; different-hash overwrites Raw and resets applied state.
type hashingInbox struct {
	fakeInbox
	upsertCalls int
}

func newHashingInbox() *hashingInbox {
	return &hashingInbox{fakeInbox: fakeInbox{applied: map[string]bool{}}}
}

func (h *hashingInbox) Upsert(_ context.Context, item *domain.InboxItem) (bool, error) {
	h.upsertCalls++
	for _, existing := range h.items {
		if existing.TenantID != item.TenantID || existing.SourceKind != item.SourceKind || existing.ExternalID != item.ExternalID {
			continue
		}
		if existing.PayloadHash == item.PayloadHash {
			return false, nil
		}
		existing.Raw = item.Raw
		existing.PayloadHash = item.PayloadHash
		existing.AppliedAt = nil
		delete(h.applied, existing.ID)
		return true, nil
	}
	if item.ID == "" {
		item.ID = item.ExternalID
	}
	cp := *item
	h.items = append(h.items, &cp)
	return true, nil
}

// TestScenario_103_ShopifyBulk_CreatesInboxItem_WithPayloadHash verifies:
// «Каждый product = 1 inbox_item с source_kind='shopify', external_id=Shopify GID,
// raw=полный JSONB, payload_hash=sha256(raw)».
func TestScenario_103_ShopifyBulk_CreatesInboxItem_WithPayloadHash(t *testing.T) {
	inbox := newHashingInbox()
	log := &fakeActionLog{}
	uc := NewInboxUseCase(inbox, log, logger.New("error"))

	raw := json.RawMessage(`{"title":"Hyaluronic Serum","vendor":"Ordinary"}`)
	items := []ShopifyItem{{GID: "gid://shopify/Product/1", Raw: raw}}

	res, err := uc.WriteFromShopifyBulk(context.Background(), "t-1", items)
	if err != nil {
		t.Fatalf("WriteFromShopifyBulk: %v", err)
	}
	if res.Inserted != 1 || res.Unchanged != 0 || res.Errors != 0 {
		t.Fatalf("res = %+v, want inserted=1 unchanged=0 errors=0", res)
	}
	if len(inbox.items) != 1 {
		t.Fatalf("inbox has %d items, want 1", len(inbox.items))
	}
	it := inbox.items[0]
	if it.SourceKind != domain.InboxSourceShopify {
		t.Errorf("source_kind = %q, want shopify", it.SourceKind)
	}
	if it.ExternalID != "gid://shopify/Product/1" {
		t.Errorf("external_id = %q, want gid://shopify/Product/1", it.ExternalID)
	}
	if string(it.Raw) != string(raw) {
		t.Errorf("raw not stored verbatim: got %q, want %q", it.Raw, raw)
	}
	sum := sha256.Sum256(raw)
	wantHash := hex.EncodeToString(sum[:])
	if it.PayloadHash != wantHash {
		t.Errorf("payload_hash = %q, want %q (hex(sha256(raw)))", it.PayloadHash, wantHash)
	}
}

// TestScenario_104a_Reingest_SameHash_ChangedFalse verifies:
// «Hash совпадает (товар не менялся) — changed=false, обновляет только fetched_at,
// applied_at не сбрасывается».
func TestScenario_104a_Reingest_SameHash_ChangedFalse(t *testing.T) {
	inbox := newHashingInbox()
	log := &fakeActionLog{}
	uc := NewInboxUseCase(inbox, log, logger.New("error"))

	raw := json.RawMessage(`{"title":"Toner","vendor":"Brand","variants":[{"sku":"T-1"}]}`)
	items := []ShopifyItem{{GID: "gid://shopify/Product/1", Raw: raw}}

	if _, err := uc.WriteFromShopifyBulk(context.Background(), "t-1", items); err != nil {
		t.Fatalf("pull 1: %v", err)
	}
	_ = inbox.MarkApplied(context.Background(), "gid://shopify/Product/1")

	res, err := uc.WriteFromShopifyBulk(context.Background(), "t-1", items)
	if err != nil {
		t.Fatalf("pull 2: %v", err)
	}
	if res.Inserted != 0 || res.Unchanged != 1 {
		t.Fatalf("res = %+v, want inserted=0 unchanged=1", res)
	}
	if !inbox.applied["gid://shopify/Product/1"] {
		t.Error("applied state must be preserved on same-hash re-pull")
	}
	if inbox.upsertCalls != 2 {
		t.Errorf("upsert calls = %d, want 2", inbox.upsertCalls)
	}
}

// TestScenario_104b_Reingest_DifferentHash_ResetsAppliedAt verifies:
// «Hash отличается — changed=true, переписывает raw, сбрасывает applied_at=NULL».
func TestScenario_104b_Reingest_DifferentHash_ResetsAppliedAt(t *testing.T) {
	inbox := newHashingInbox()
	log := &fakeActionLog{}
	uc := NewInboxUseCase(inbox, log, logger.New("error"))

	v1 := json.RawMessage(`{"title":"Toner v1"}`)
	v2 := json.RawMessage(`{"title":"Toner v2 — renamed"}`)

	if _, err := uc.WriteFromShopifyBulk(context.Background(), "t-1", []ShopifyItem{
		{GID: "gid://shopify/Product/1", Raw: v1},
	}); err != nil {
		t.Fatalf("pull 1: %v", err)
	}
	_ = inbox.MarkApplied(context.Background(), "gid://shopify/Product/1")

	res, err := uc.WriteFromShopifyBulk(context.Background(), "t-1", []ShopifyItem{
		{GID: "gid://shopify/Product/1", Raw: v2},
	})
	if err != nil {
		t.Fatalf("pull 2: %v", err)
	}
	// "Changed" rows land in the Inserted bucket per current InboxWriteResult
	// semantics (Inserted = new-or-changed; Updated is always 0 — see inbox.go
	// lines 75-78). That's a known coarseness, not a bug the test should fail on.
	if res.Inserted != 1 || res.Unchanged != 0 {
		t.Fatalf("res = %+v, want inserted=1 unchanged=0 (changed→inserted bucket)", res)
	}
	if inbox.applied["gid://shopify/Product/1"] {
		t.Error("applied state must be reset when payload hash differs")
	}
	if string(inbox.items[0].Raw) != string(v2) {
		t.Errorf("raw not overwritten: got %q, want %q", inbox.items[0].Raw, v2)
	}
}

// TestScenario_106_UTF8_NonASCII_StoredVerbatim verifies:
// «Товар с не-ASCII символами в title (русский, эмодзи, китайский) — raw JSONB
// корректно хранит UTF-8, никаких \u escape».
func TestScenario_106_UTF8_NonASCII_StoredVerbatim(t *testing.T) {
	inbox := newHashingInbox()
	log := &fakeActionLog{}
	uc := NewInboxUseCase(inbox, log, logger.New("error"))

	raw := json.RawMessage(`{"title":"Тонер 🌸 化妆水","vendor":"COSRX"}`)
	if _, err := uc.WriteFromShopifyBulk(context.Background(), "t-1", []ShopifyItem{
		{GID: "gid://shopify/Product/X", Raw: raw},
	}); err != nil {
		t.Fatalf("WriteFromShopifyBulk: %v", err)
	}
	stored := string(inbox.items[0].Raw)
	if !strings.Contains(stored, "Тонер") {
		t.Errorf("cyrillic missing from stored raw: %q", stored)
	}
	if !strings.Contains(stored, "🌸") {
		t.Errorf("emoji missing from stored raw: %q", stored)
	}
	if !strings.Contains(stored, "化妆水") {
		t.Errorf("chinese missing from stored raw: %q", stored)
	}
	if strings.Contains(stored, `\u`) {
		t.Errorf("raw must be verbatim, got escaped form: %q", stored)
	}
}

// TestScenario_107_ActionLog_InboxPull_Written verifies:
// «action_log пишет inbox_pull с payload {source, total, inserted, updated,
// unchanged, errors}, status=ok (или warning если errors>0)».
func TestScenario_107_ActionLog_InboxPull_Written(t *testing.T) {
	inbox := newHashingInbox()
	log := &fakeActionLog{}
	uc := NewInboxUseCase(inbox, log, logger.New("error"))

	items := []ShopifyItem{
		{GID: "gid://shopify/Product/1", Raw: json.RawMessage(`{"title":"A"}`)},
		{GID: "gid://shopify/Product/2", Raw: json.RawMessage(`{"title":"B"}`)},
		// Empty GID → error bucket → status should escalate to "warning".
		{GID: "", Raw: json.RawMessage(`{"title":"C"}`)},
	}
	res, err := uc.WriteFromShopifyBulk(context.Background(), "t-1", items)
	if err != nil {
		t.Fatalf("WriteFromShopifyBulk: %v", err)
	}
	if res.Total != 3 || res.Errors != 1 {
		t.Fatalf("res = %+v, want total=3 errors=1", res)
	}

	var pull *ports.TenantActionLogEntry
	for _, e := range log.entries {
		if e.Action == "inbox_pull" {
			pull = e
			break
		}
	}
	if pull == nil {
		t.Fatal("inbox_pull entry not written to action_log")
	}
	if pull.Status != "warning" {
		t.Errorf("status = %q, want warning (1 of 3 errored)", pull.Status)
	}
	var payload map[string]any
	if err := json.Unmarshal(pull.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	for _, key := range []string{"source", "total", "inserted", "updated", "unchanged", "errors"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("payload missing key %q (have %+v)", key, payload)
		}
	}
	if payload["source"] != "shopify" {
		t.Errorf("source = %v, want shopify", payload["source"])
	}
}
