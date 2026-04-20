---
Branch: main
Date (UTC): 2026-04-20 22:51
Parent commit: dbb00dd fix(admin-auth): auth shell fills viewport width
Plan: /Users/starknight/.claude/plans/wondrous-meandering-dragonfly.md
---

# Admin redesign — Chats / Catalog / Import (Phase 1)

Ports the Pencil-redesigned admin into React: black-sidebar shell, new design tokens (Inter, #4285F4 accent, #1A1A1A text, soft borders, pill controls), then rebuilt three sections — Chats list + detail, Catalog list + detail, Import — to match `Admin.pen` frames `NOjF5`, `OsxPh`, `xvh1E`, `m0Gbe`, `IAlhb`. Widget / Canvas / Settings remain on the old shell pending a follow-up pass.

## Context

The admin frontend was the last surface still on the old blue-on-white styling that predated the auth/onboarding visual language landed in `feature/admin-auth-screens` and `feature/admin-onboarding-mvp`. The user redesigned five admin frames in Pencil; this session ports them into the live React app while leaving the rest of the admin functional.

## Approach

- **Token overhaul** in `src/index.css`: introduced `--bg / --surface / --border / --text* / --accent / --radius-* / --sidebar-*` and kept `--color-*` aliases mapped to the new vars so untouched pages don't break. Loaded Inter from Google Fonts.
- **Shell** (`features/layout/`): black sidebar (#0F0F0F, 240px), white logo, white-on-black active state with `rgba(255,255,255,0.1)` background; reordered nav to match the design (Chats / Catalog / Import / Widget / Canvas / Settings) and dropped the standalone Integrations + Traces links since `/import` is now the source-card hub.
- **Shared primitives**: added `pill` modifier to `Button` (radius-pill, height 36, dark-on-white primary) and a new `ChipGroup` for filter rails. Pulled out reusable header/search/card/status-badge styles into `shared/ui/ui.css`.
- **Chats** (`features/conversations/`):
  - List: chip filters (All / Online / Idle / Completed) with counts, pill search, Export pill button. Rows grouped by derived status (`<2 min` → online, `<30 min` → idle, else completed). New columns: USER (with avatar) / FIRST MESSAGE / STARTED / ACTIVE TIME / COST / STATUS pill.
  - Detail: `All chats ←` breadcrumb + user identity + right-aligned cost/active/status meta. Two-column body: transcript card with sticky read-only composer; right rail with `WidgetThumb` list (extracted from formation traces) + `SessionInfoCard` (preset, layout, items shown, ops, cart shown) + "Open in Canvas" pill.
- **Catalog** (`features/catalog/`):
  - List: 240px `CategoryTree` (static skincare hierarchy as planned — backend categories punt) + breadcrumb + new table (Product cell with thumb + name + brand / SKU / Price / Stock / Status pill via `stockStatus`). Reuses `Table`, `Pagination`.
  - Detail: breadcrumb + right-aligned Save changes pill. Two-column layout — left: hero card (image + name + tags + price + description) → product details form → "Additional information" stub block. Right rail: Variants card (placeholder), Performance card (placeholder metrics), Quick actions (Preview/Duplicate/Archive/Delete ghost buttons).
- **Import** (`features/import/`): three source cards (Shopify → `/integrations/shopify`, CSV → `/integrations/csv`, Google Sheets disabled) styled like the auth source cards; Recent jobs table reads `/catalog/imports?limit=20` via existing `useJobPolling`; legacy raw-JSON drop is hidden behind an "Advanced — JSON upload" disclosure so the wiring keeps working without dominating the page.

## Files changed

| Scope | Files |
|---|---|
| Tokens / shell | `src/index.css`, `src/features/layout/{DashboardLayout.jsx,layout.css}` |
| Shared UI | `src/shared/ui/{Button.jsx,ui.css}`, `src/shared/ui/ChipGroup.jsx` (new) |
| Chats | `src/features/conversations/{ConversationsPage.jsx,ConversationDetailPage.jsx,conversations.css}`, `src/features/conversations/{SessionInfoCard.jsx,WidgetThumb.jsx}` (new) |
| Catalog | `src/features/catalog/{ProductsPage.jsx,ProductDetailPage.jsx,catalog.css}`, `src/features/catalog/{CategoryTree.jsx,productDetail.css}` (new) |
| Import | `src/features/import/{ImportPage.jsx,import.css}` |

## Verification

- `cd project_admin/frontend && npm run build` → clean (`✓ built in 3.75s`).
- Manual smoke-test plan (run with `npm run dev`, port 5174):
  1. Login → land on `/catalog`. Sidebar should be black with white "Catalog" highlighted. Inter font everywhere.
  2. `/conversations`: chip filters switch visible group; search filters by user/message; status pills (Live / Idle / Done) render with dot indicator.
  3. `/conversations/:id`: transcript on left, right rail shows widget list + session info + "Open in Canvas".
  4. `/catalog`: category tree visible on left, "Skincare → Moisturizers → Face creams" expandable; selecting a leaf filters the table client-side; status badges (Active/Low stock/Out) render correctly.
  5. `/catalog/:id`: 2-col layout, Save changes pill in top-right works (hits `PUT /products/:id`), right rail visible.
  6. `/import`: three source cards; Shopify/CSV cards link to `/integrations/shopify` and `/integrations/csv`; Recent jobs table populates; "Advanced — JSON upload" expands to legacy drop.

## Known gaps / caveats

- **Widget / Canvas / Settings** still use the old styling — black sidebar applies but the page contents weren't touched. Next phase.
- **Category tree is hard-coded skincare hierarchy** with client-side substring filter on `category` text. Backend-driven categories tree is a follow-up; a non-cosmetics tenant will see an irrelevant tree.
- **Export button on Chats list is visual-only** — no CSV writer behind it.
- **"Open in Canvas" navigates to `/canvas` generically** — deep-linking to a specific session replay isn't wired.
- **Performance metrics + Variants on product detail are placeholders** — the backend doesn't surface those numbers yet.
- **"Additional information" fields (social links, gallery, stories, reviews)** are disabled inputs — UI shell only.
- **Sidebar dropped the Integrations link** since `/import` now hosts source cards. `/integrations`-* routes still resolve directly (used by source-card links) but are no longer in the nav.
- All UI copy is English per the user-facing-text-English-only memory.
