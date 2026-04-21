# Billing & Subscription page (admin) — implementation

- **Branch:** main
- **Date (UTC):** 2026-04-21 00:05
- **Parent commit:** bce6c8cfc1f69f235108155194af5d5516f27a2d
- **Commit sha:** (not yet committed; this log describes the working-tree change set)

## Context

The admin frontend was missing the Billing & Usage page (Pencil frame `UAI4l`, "7. Billing & Usage") — the sidebar had no entry, and there was no API or DB support behind it either. The goal of this session was to ship the full UI matching the Pencil design, backed by real data (plan, subscription, token usage aggregated from traces, recent invoices) and togglable preferences (alert threshold, Slack alerts, overdraft protection).

Explicit constraints from the user:
- Payment provider integration (Stripe etc.) is out of scope — display + toggles only.
- Pricing rule: the admin shows `api_tokens × multiplier` (default multiplier = 5, because Keepstar sells chat at ~5× API cost). Multiplier is stored per tenant so it can later vary; every tenant seeds with 5.00.
- New sidebar item for Billing (not nested under Settings).
- Real wiring — tokens aggregated from `pipeline_traces`, plan/preferences/invoices persisted in DB.

## Approach

### Backend — hexagonal layers

1. **Domain (`internal/domain/billing.go`)** — single source of truth for the plan catalog. `PlanCatalog` is a hardcoded `map[string]Plan` with three tiers (starter / growth / enterprise). No `plans` DB table; catalog lives in code so marketing copy changes don't need a migration. `PlanOrder` drives the Available Plans order.
2. **Port (`internal/ports/billing_port.go`)** — `BillingPort` interface with five methods: `GetSubscription`, `UpdatePlan`, `UpdatePreferences`, `ListInvoices`, `AggregateTokensForCycle`.
3. **Adapter (`internal/adapters/postgres/billing_adapter.go`)** — Postgres implementation.
   - `GetSubscription` auto-seeds a row via `INSERT ... ON CONFLICT DO NOTHING` then `SELECT`, so the first hit for any tenant never 404s.
   - `AggregateTokensForCycle` sums `agent1/agent2 inputTokens+outputTokens` from `pipeline_traces.trace_data` JSONB, with a JOIN through `catalog.tenants` to bridge the UUID ↔ slug gap (`chat_sessions.tenant_id` is the slug string, while admin rows reference `catalog.tenants(id)` UUID).
4. **Usecase (`internal/usecases/billing.go`)** — composes the `/overview` response. Applies the multiplier once: `tokensDisplayed := int64(float64(apiTokens) * multiplier)`; both values are returned so the frontend can show the multiplier explicitly later.
5. **Handler (`internal/handlers/handler_billing.go`)** — four routes, all protected by the existing auth middleware, tenant pulled from `TenantID(ctx)`:
   - `GET  /admin/api/billing/overview`
   - `GET  /admin/api/billing/invoices?limit=20`
   - `POST /admin/api/billing/plan`
   - `POST /admin/api/billing/preferences`
6. **Migrations** — appended to the existing `admin_migrations.go` array, so they run on server startup (idempotent `IF NOT EXISTS`). Two tables: `admin.billing_subscriptions` and `admin.billing_invoices`, both referencing `catalog.tenants(id)` with `ON DELETE CASCADE`.
7. **DI in `cmd/server/main.go`** — 2 init lines (adapter + usecase), 1 handler line, 4 `protected.HandleFunc` + 4 `mux.Handle` (authMW) registrations.
8. **Seed (`cmd/seed-demo-chat/main.go`)** — appended a block that resolves the demo tenant slug to a UUID and inserts: one subscription (growth, multiplier 5.00) and three invoices with deterministic IDs (`INV-YYYY-MM`) — current month upcoming, prior two months paid at $49 each. `ON CONFLICT (id) DO NOTHING` keeps the seed idempotent.

### Frontend — Feature-Sliced module

New feature folder `src/features/billing/`:
- `BillingPage.jsx` — fetches `/billing/overview` and `/billing/invoices?limit=20` in parallel, renders the four sections. `changePlan` POSTs and reloads; `savePrefs` POSTs with optimistic state update.
- `CurrentPlanCard.jsx` — eyebrow + plan name + "N tokens / month · renews {date}" + status pill + usage bar + 4-column Conversations/Products/Seats/Support grid.
- `AvailablePlans.jsx` — three `.plan-row` rows. Current plan expands with the same detail grid; others show a single Upgrade/Downgrade pill button.
- `PreferencesCards.jsx` — two cards with four toggles total (alert threshold, Slack alerts, overdraft enabled, overdraft notify).
- `InvoicesTable.jsx` — uses the existing `shared/ui/Table` primitive; columns: Invoice / Period / Tokens used / Status / action.
- `plans.js` — helpers (`actionForPlan`, `formatNumber`, `formatDate`, `formatPeriod`).
- `billing.css` — co-located styles, responsive collapse to 2-col / 1-col at `max-width: 900px`.

Shared primitives:
- New `src/shared/ui/Toggle.jsx` + matching CSS block in `ui.css` (no Toggle primitive existed before; extracting one keeps the four rows clean).

Routing & nav:
- `src/App.jsx` — `<Route path="billing" element={<BillingPage />} />` inside the protected DashboardLayout group.
- `DashboardLayout.jsx` — new sidebar entry with `CreditCard` icon, placed right after Settings so the trailing group reads Settings → Billing.

## Files changed

| Scope | File | New/Edit |
|---|---|---|
| Migration | `project_admin/backend/internal/adapters/postgres/admin_migrations.go` | Edit |
| Domain | `project_admin/backend/internal/domain/billing.go` | New |
| Port | `project_admin/backend/internal/ports/billing_port.go` | New |
| Adapter | `project_admin/backend/internal/adapters/postgres/billing_adapter.go` | New |
| Usecase | `project_admin/backend/internal/usecases/billing.go` | New |
| Handler | `project_admin/backend/internal/handlers/handler_billing.go` | New |
| DI | `project_admin/backend/cmd/server/main.go` | Edit |
| Seed | `project_admin/backend/cmd/seed-demo-chat/main.go` | Edit |
| Router | `project_admin/frontend/src/App.jsx` | Edit |
| Sidebar | `project_admin/frontend/src/features/layout/DashboardLayout.jsx` | Edit |
| Page | `project_admin/frontend/src/features/billing/BillingPage.jsx` | New |
| Page parts | `.../CurrentPlanCard.jsx`, `AvailablePlans.jsx`, `PreferencesCards.jsx`, `InvoicesTable.jsx` | New |
| Helpers | `project_admin/frontend/src/features/billing/plans.js` | New |
| Styles | `project_admin/frontend/src/features/billing/billing.css` | New |
| Toggle primitive | `project_admin/frontend/src/shared/ui/Toggle.jsx` | New |
| Toggle CSS | `project_admin/frontend/src/shared/ui/ui.css` | Edit |

## Verification

- `cd project_admin/backend && go build ./... && go vet ./...` → clean.
- `cd project_admin/frontend && npm run build` → clean (Vite 7.3.1, 2784 modules, 3.33s).
- Migration is idempotent (`IF NOT EXISTS`), so starting the server with an already-migrated DB is a no-op.
- Seed is idempotent (`ON CONFLICT DO NOTHING` on both the subscription and the three `INV-YYYY-MM` invoice IDs).
- What to smoke-test locally once server restarts:
  1. Sidebar highlights Billing when navigated to `/billing`.
  2. `curl -H 'Cookie: …' localhost:8081/admin/api/billing/overview` returns JSON with `usage.tokensUsed` = `usage.apiTokensActual × multiplier`.
  3. Current Plan card shows Growth with the renewal date and the usage bar filled proportionally.
  4. Available Plans renders three rows, Growth expanded; clicking Upgrade/Downgrade on another row changes the active plan.
  5. Toggling any preference persists — refresh preserves state.
  6. Invoices table shows three rows (Upcoming / Paid / Paid) after re-running `go run ./cmd/seed-demo-chat`.

## Known gaps / caveats

- **No Stripe / payment collection.** `UpdatePlan` just flips the DB row; there's no checkout, no webhook, no payment method storage.
- **`View` / `PDF` actions on invoices are visual only** — no PDF generator and no detail modal. `View` on the Upcoming row is a `#view` anchor; `PDF` becomes a muted non-link when `pdf_url` is NULL.
- **"Upgrade plan" in the header is not wired** — the Pencil design shows a top-right pill, but for now the primary plan-change path lives inside the Available Plans rows. Wiring the header button to the next tier is a one-liner when we decide the UX.
- **Per-tenant multiplier override UI is out of scope.** The `token_multiplier` column exists and is honored; operators edit it via SQL for now.
- **Cycle rollover isn't scheduled.** `cycle_start` / `cycle_end` are set once at seed/auto-seed time; a monthly rollover job is a later task.
- **Real invoice generation isn't scheduled.** The three seeded invoices are static; we don't yet have a monthly cron writing a new `INV-YYYY-MM` row.
- **Aggregation join assumes `chat_sessions.tenant_id` is the slug** (VARCHAR), bridged through `catalog.tenants`. If that column is ever migrated to UUID, the JOIN in `AggregateTokensForCycle` needs to drop the `catalog.tenants` hop.
- The commit for this work has not been created yet — this log captures the working-tree state.
