// seed-demo-chat inserts a single deterministic demo chat session into the
// database so the admin /conversations list always has at least one chat to
// drill into. Used as a reference/teaching sample.
//
// Idempotent: every INSERT uses deterministic UUIDs + ON CONFLICT DO NOTHING.
// Delete with:
//   DELETE FROM chat_sessions WHERE id = '<session id printed below>';
// (cascades remove traces/deltas/state.)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

const demoNamespace = "5f3e6d4c-1a2b-4c3d-9e8f-7a6b5c4d3e2f"

func main() {
	for _, p := range []string{"../../project/.env", "../../../project/.env", "project/.env", ".env"} {
		if err := godotenv.Load(p); err == nil {
			break
		}
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	tenant := os.Getenv("TENANT_SLUG")
	if tenant == "" {
		tenant = "keepstar-demo"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	ns := uuid.MustParse(demoNamespace)
	sessionID := uuid.NewSHA1(ns, []byte("session"))
	trace1ID := uuid.NewSHA1(ns, []byte("trace-1"))
	trace2ID := uuid.NewSHA1(ns, []byte("trace-2"))
	delta1ID := uuid.NewSHA1(ns, []byte("delta-1"))

	start := time.Now().Add(-6 * time.Minute)
	t1 := start.Add(1 * time.Second)
	t2 := start.Add(15 * time.Second)
	t3 := start.Add(30 * time.Second)

	trace1Data := buildTraceData("Show me a good moisturizer for dry skin", gridFormationJSON)
	trace2Data := buildTraceData("Tell me more about the Vitamin C serum", detailFormationJSON)

	// Session
	meta := `{"demo":true,"purpose":"reference/training","userLabel":"demo@keepstar.ai"}`
	if _, err = pool.Exec(ctx, `
		INSERT INTO chat_sessions (id, tenant_id, status, started_at, last_activity_at, metadata)
		VALUES ($1, $2, 'active', $3, $4, $5::jsonb)
		ON CONFLICT (id) DO NOTHING
	`, sessionID, tenant, start, t3, meta); err != nil {
		log.Fatalf("insert session: %v", err)
	}

	// Traces
	traces := []struct {
		id    uuid.UUID
		query string
		ts    time.Time
		data  []byte
		cost  float64
	}{
		{trace1ID, "Show me a good moisturizer for dry skin", t1, trace1Data, 0.0061},
		{trace2ID, "Tell me more about the Vitamin C serum", t3, trace2Data, 0.0081},
	}
	for _, tr := range traces {
		if _, err = pool.Exec(ctx, `
			INSERT INTO pipeline_traces (id, session_id, query, timestamp, trace_data, total_ms, cost_usd)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)
			ON CONFLICT (id) DO NOTHING
		`, tr.id, sessionID.String(), tr.query, tr.ts, tr.data, 420, tr.cost); err != nil {
			log.Fatalf("insert trace: %v", err)
		}
	}

	// User click between the two turns (drill into product 2)
	actionJSON, _ := json.Marshal(map[string]any{
		"actorId":   "user_click",
		"deltaType": "click",
		"path":      "widgets[1]",
		"entityId":  "demo-prod-2",
	})
	if _, err = pool.Exec(ctx, `
		INSERT INTO chat_session_deltas
			(id, session_id, step, trigger, action, result, source, actor_id, delta_type, path, created_at)
		VALUES ($1, $2, 1, 'user', $3::jsonb, '{}'::jsonb, 'user', 'user_click', 'click', 'widgets[1]', $4)
		ON CONFLICT (session_id, step) DO NOTHING
	`, delta1ID, sessionID, actionJSON, t2); err != nil {
		log.Fatalf("insert delta: %v", err)
	}

	// Session state (empty likes/cart — keeps stats consistent).
	if _, err = pool.Exec(ctx, `
		INSERT INTO chat_session_state (session_id, actions, view_mode, step)
		VALUES ($1, '{"likedIds":[],"cartItems":[]}'::jsonb, 'single', 2)
		ON CONFLICT (session_id) DO NOTHING
	`, sessionID); err != nil {
		log.Fatalf("insert state: %v", err)
	}

	fmt.Printf("Demo chat seeded.\n  session_id = %s\n  tenant     = %s\n  traces     = 2\n  actions    = 1\n", sessionID, tenant)

	// Billing demo — one subscription row and three invoices for the current
	// demo tenant. Resolves the slug stored on chat_sessions into the UUID
	// that admin.billing_* tables reference.
	var tenantUUID string
	err = pool.QueryRow(ctx, `SELECT id::text FROM catalog.tenants WHERE slug = $1`, tenant).Scan(&tenantUUID)
	if err != nil {
		log.Printf("skip billing seed: tenant %q not found in catalog.tenants (%v)", tenant, err)
		return
	}

	// Subscription — ON CONFLICT preserves any preference changes made via
	// the admin UI between seed runs; we only re-anchor plan + cycle.
	if _, err = pool.Exec(ctx, `
		INSERT INTO admin.billing_subscriptions
			(tenant_id, plan, status, cycle_start, cycle_end, token_multiplier)
		VALUES ($1, 'growth', 'active',
		        date_trunc('month', now())::date,
		        (date_trunc('month', now()) + interval '1 month')::date,
		        5.00)
		ON CONFLICT (tenant_id) DO UPDATE
		   SET plan = EXCLUDED.plan,
		       status = EXCLUDED.status,
		       cycle_start = EXCLUDED.cycle_start,
		       cycle_end   = EXCLUDED.cycle_end,
		       token_multiplier = EXCLUDED.token_multiplier,
		       updated_at = now()
	`, tenantUUID); err != nil {
		log.Fatalf("insert subscription: %v", err)
	}

	// Three invoices: current month (upcoming), prior two months (paid).
	// IDs are INV-YYYY-MM so they stay stable across seed runs.
	now := time.Now().UTC()
	months := []struct {
		offset  int  // 0 = this month, -1 = last month, ...
		status  string
		tokens  int64
		amount  *float64
	}{
		{0, "upcoming", 34200, nil},
		{-1, "paid", 41850, ptrF(49.00)},
		{-2, "paid", 29100, ptrF(49.00)},
	}
	for _, m := range months {
		first := time.Date(now.Year(), now.Month()+time.Month(m.offset), 1, 0, 0, 0, 0, time.UTC)
		last := first.AddDate(0, 1, -1)
		id := fmt.Sprintf("INV-%04d-%02d", first.Year(), int(first.Month()))
		if _, err = pool.Exec(ctx, `
			INSERT INTO admin.billing_invoices
				(id, tenant_id, period_start, period_end, tokens_used, amount_usd, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (id) DO NOTHING
		`, id, tenantUUID, first, last, m.tokens, m.amount, m.status); err != nil {
			log.Fatalf("insert invoice %s: %v", id, err)
		}
	}
	fmt.Printf("Billing demo seeded for tenant %s.\n", tenantUUID)
}

func ptrF(f float64) *float64 { return &f }

func buildTraceData(query, formationJSON string) []byte {
	trace := map[string]any{
		"query": query,
		"formationResult": map[string]any{
			"fullJson": json.RawMessage(formationJSON),
		},
	}
	b, _ := json.Marshal(trace)
	return b
}

// --- Hand-crafted formations matching the V4 product_card / product_detail shape.
// Widget uses atomsV2 + layout tree so GenericCardV2Template renders them as
// real cards (hero image → title → price/rating → brand badge).

const gridFormationJSON = `{
  "mode": "grid",
  "preset": "product_card",
  "grid": { "cols": 3 },
  "widgets": [
    {
      "id": "demo-w-1",
      "template": "product_card",
      "size": "medium",
      "entityRef": { "type": "product", "id": "demo-prod-1" },
      "atomsV2": [
        { "type": "image", "slot": "hero", "value": "https://images.unsplash.com/photo-1556228720-195a672e8a03?w=600" },
        { "type": "text", "slot": "title", "value": "Hydrating Moisturizer", "textStyle": { "fontSize": "lg", "fontWeight": "semibold" } },
        { "type": "text", "slot": "price", "value": "$24.00", "textStyle": { "fontSize": "md", "fontWeight": "semibold" } },
        { "type": "text", "slot": "rating", "value": "★ 4.8", "textStyle": { "color": "muted" } },
        { "type": "text", "slot": "badge", "value": "Glow Lab", "wrapper": { "type": "badge", "variant": "subtle" } }
      ],
      "layout": {
        "type": "column", "gap": "sm",
        "children": [
          { "atomIndex": 0 },
          { "node": { "type": "column", "gap": "xs", "children": [
            { "atomIndex": 1 },
            { "node": { "type": "row", "gap": "md", "children": [ { "atomIndex": 2 }, { "atomIndex": 3 } ] } },
            { "atomIndex": 4 }
          ] } }
        ]
      }
    },
    {
      "id": "demo-w-2",
      "template": "product_card",
      "size": "medium",
      "entityRef": { "type": "product", "id": "demo-prod-2" },
      "atomsV2": [
        { "type": "image", "slot": "hero", "value": "https://images.unsplash.com/photo-1620916566398-39f1143ab7be?w=600" },
        { "type": "text", "slot": "title", "value": "Vitamin C Serum", "textStyle": { "fontSize": "lg", "fontWeight": "semibold" } },
        { "type": "text", "slot": "price", "value": "$32.00", "textStyle": { "fontSize": "md", "fontWeight": "semibold" } },
        { "type": "text", "slot": "rating", "value": "★ 4.6", "textStyle": { "color": "muted" } },
        { "type": "text", "slot": "badge", "value": "Pure Skin", "wrapper": { "type": "badge", "variant": "subtle" } }
      ],
      "layout": {
        "type": "column", "gap": "sm",
        "children": [
          { "atomIndex": 0 },
          { "node": { "type": "column", "gap": "xs", "children": [
            { "atomIndex": 1 },
            { "node": { "type": "row", "gap": "md", "children": [ { "atomIndex": 2 }, { "atomIndex": 3 } ] } },
            { "atomIndex": 4 }
          ] } }
        ]
      }
    },
    {
      "id": "demo-w-3",
      "template": "product_card",
      "size": "medium",
      "entityRef": { "type": "product", "id": "demo-prod-3" },
      "atomsV2": [
        { "type": "image", "slot": "hero", "value": "https://images.unsplash.com/photo-1571781926291-c477ebfd024b?w=600" },
        { "type": "text", "slot": "title", "value": "Night Repair Cream", "textStyle": { "fontSize": "lg", "fontWeight": "semibold" } },
        { "type": "text", "slot": "price", "value": "$28.00", "textStyle": { "fontSize": "md", "fontWeight": "semibold" } },
        { "type": "text", "slot": "rating", "value": "★ 4.7", "textStyle": { "color": "muted" } },
        { "type": "text", "slot": "badge", "value": "Velvet", "wrapper": { "type": "badge", "variant": "subtle" } }
      ],
      "layout": {
        "type": "column", "gap": "sm",
        "children": [
          { "atomIndex": 0 },
          { "node": { "type": "column", "gap": "xs", "children": [
            { "atomIndex": 1 },
            { "node": { "type": "row", "gap": "md", "children": [ { "atomIndex": 2 }, { "atomIndex": 3 } ] } },
            { "atomIndex": 4 }
          ] } }
        ]
      }
    }
  ]
}`

const detailFormationJSON = `{
  "mode": "single",
  "preset": "product_detail",
  "widgets": [
    {
      "id": "demo-detail",
      "template": "product_detail",
      "size": "large",
      "entityRef": { "type": "product", "id": "demo-prod-2" },
      "atomsV2": [
        { "type": "image", "slot": "hero", "value": "https://images.unsplash.com/photo-1620916566398-39f1143ab7be?w=900" },
        { "type": "text", "slot": "title", "value": "Vitamin C Serum", "textStyle": { "fontSize": "2xl", "fontWeight": "bold" } },
        { "type": "text", "slot": "price", "value": "$32.00", "textStyle": { "fontSize": "xl", "fontWeight": "semibold" } },
        { "type": "text", "slot": "rating", "value": "★ 4.6 (214 reviews)", "textStyle": { "color": "muted" } },
        { "type": "text", "slot": "badge", "value": "Pure Skin", "wrapper": { "type": "badge" } },
        { "type": "text", "slot": "description", "value": "15% L-ascorbic acid serum for brightening and even tone. Lightweight, fast-absorbing, fragrance-free. Apply 3–4 drops morning and evening before moisturizer." }
      ],
      "layout": {
        "type": "column", "gap": "lg",
        "children": [
          { "atomIndex": 0 },
          { "node": { "type": "column", "gap": "md", "children": [
            { "atomIndex": 1 },
            { "node": { "type": "row", "gap": "md", "children": [ { "atomIndex": 2 }, { "atomIndex": 3 }, { "atomIndex": 4 } ] } },
            { "atomIndex": 5 }
          ] } }
        ]
      }
    }
  ]
}`
