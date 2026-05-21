//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"keepstar-admin/internal/adapters/postgres"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
)

// TestInboxBulkUpsert_Idempotency_PreservesAppliedAt locks in item A4 from
// the alpha-0.9.4 milestone: identical re-imports must NOT cause re-apply.
//
// Contract verified:
//
//	(a) Re-upserting the same row (same payload_hash) preserves applied_at
//	    so apply_v2's ListUnapplied filter skips it.
//	(b) Re-upserting with a different payload_hash resets applied_at = NULL
//	    so the row is picked up again by the next apply run.
//
// This contract lives in inbox_adapter.go's ON CONFLICT clause. It is
// implicit (no separate flag/column) — this test makes it explicit so a
// future refactor of the upsert SQL can't silently break daily-cron's
// idempotency guarantee.
func TestInboxBulkUpsert_Idempotency_PreservesAppliedAt(t *testing.T) {
	pool := setupDB(t)
	tenantID := seedTenant(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := postgres.NewClient(ctx, pool.Config().ConnString())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(client.Close)
	inbox := postgres.NewInboxAdapter(client, logger.New("error"))

	mk := func(eid, hash, body string) *domain.InboxItem {
		return &domain.InboxItem{
			TenantID:    tenantID,
			SourceKind:  domain.InboxSourceCSV,
			ExternalID:  eid,
			Raw:         json.RawMessage(body),
			PayloadHash: hash,
		}
	}

	// Step 1 — first import of 3 rows.
	first := []*domain.InboxItem{
		mk("idem-1", "h1", `{"sku":"A","name":"Apple"}`),
		mk("idem-2", "h2", `{"sku":"B","name":"Banana"}`),
		mk("idem-3", "h3", `{"sku":"C","name":"Cherry"}`),
	}
	changed, err := inbox.BulkUpsert(ctx, first)
	if err != nil {
		t.Fatalf("first BulkUpsert: %v", err)
	}
	if changed != 3 {
		t.Errorf("first BulkUpsert changed=%d, want 3", changed)
	}

	// Step 2 — mark all 3 as applied.
	idsByEID := func() map[string]string {
		out := map[string]string{}
		rows, err := pool.Query(ctx, `
			SELECT id::text, external_id
			FROM catalog.inbox_items
			WHERE tenant_id = $1
		`, tenantID)
		if err != nil {
			t.Fatalf("select ids: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id, eid string
			if err := rows.Scan(&id, &eid); err != nil {
				t.Fatalf("scan: %v", err)
			}
			out[eid] = id
		}
		return out
	}()
	allIDs := []string{idsByEID["idem-1"], idsByEID["idem-2"], idsByEID["idem-3"]}
	if err := inbox.BulkMarkApplied(ctx, allIDs); err != nil {
		t.Fatalf("BulkMarkApplied: %v", err)
	}

	// Snapshot applied_at after marking.
	appliedAt := map[string]time.Time{}
	for eid, id := range idsByEID {
		var at *time.Time
		if err := pool.QueryRow(ctx, `SELECT applied_at FROM catalog.inbox_items WHERE id::text = $1`, id).Scan(&at); err != nil {
			t.Fatalf("select applied_at for %s: %v", eid, err)
		}
		if at == nil {
			t.Fatalf("applied_at nil for %s after BulkMarkApplied", eid)
		}
		appliedAt[eid] = *at
	}

	// Step 3 — re-import the EXACT same 3 rows (same payload_hash).
	// Expectation: applied_at preserved for all 3.
	identical := []*domain.InboxItem{
		mk("idem-1", "h1", `{"sku":"A","name":"Apple"}`),
		mk("idem-2", "h2", `{"sku":"B","name":"Banana"}`),
		mk("idem-3", "h3", `{"sku":"C","name":"Cherry"}`),
	}
	changed, err = inbox.BulkUpsert(ctx, identical)
	if err != nil {
		t.Fatalf("identical re-import BulkUpsert: %v", err)
	}
	if changed != 0 {
		t.Errorf("identical re-import changed=%d, want 0 (no real content change)", changed)
	}
	for eid, id := range idsByEID {
		var at *time.Time
		_ = pool.QueryRow(ctx, `SELECT applied_at FROM catalog.inbox_items WHERE id::text = $1`, id).Scan(&at)
		if at == nil {
			t.Errorf("idempotency broken: applied_at cleared for %s after identical re-import", eid)
		} else if !at.Equal(appliedAt[eid]) {
			t.Errorf("idempotency broken: applied_at moved for %s (was %v, now %v)", eid, appliedAt[eid], *at)
		}
	}

	// Step 4 — re-import with row idem-2's hash changed (real content change).
	// Expectation: applied_at preserved for idem-1 and idem-3; cleared for idem-2.
	withChange := []*domain.InboxItem{
		mk("idem-1", "h1", `{"sku":"A","name":"Apple"}`),
		mk("idem-2", "h2-new", `{"sku":"B","name":"Banana Updated"}`),
		mk("idem-3", "h3", `{"sku":"C","name":"Cherry"}`),
	}
	changed, err = inbox.BulkUpsert(ctx, withChange)
	if err != nil {
		t.Fatalf("partial re-import BulkUpsert: %v", err)
	}
	if changed != 1 {
		t.Errorf("partial re-import changed=%d, want 1 (only idem-2 differs)", changed)
	}
	for eid, id := range idsByEID {
		var at *time.Time
		_ = pool.QueryRow(ctx, `SELECT applied_at FROM catalog.inbox_items WHERE id::text = $1`, id).Scan(&at)
		switch eid {
		case "idem-2":
			if at != nil {
				t.Errorf("applied_at NOT cleared for idem-2 after hash change (got %v)", *at)
			}
		default:
			if at == nil {
				t.Errorf("applied_at cleared for %s — only idem-2's hash changed, the other rows are unchanged", eid)
			} else if !at.Equal(appliedAt[eid]) {
				t.Errorf("applied_at moved for %s (was %v, now %v) — unchanged rows shouldn't shift", eid, appliedAt[eid], *at)
			}
		}
	}
}
