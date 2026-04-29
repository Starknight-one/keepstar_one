# main — Shopify install flow + magic-link auth

> **Branch**: main
> **Date (UTC)**: 2026-04-29 19:24
> **HEAD at write**: 978f10e
> **Parent (start of session)**: d008cc8
> **Mode**: live test passed end-to-end on `test2-mhjhejbs.myshopify.com`

---

## Context

Перед этой сессией: install из Shopify App Store у нас не делал ничего — App URL был настроен на корень фронта (`https://admin-production-4ae4.up.railway.app`), который ничего не знает про `?shop=&hmac=`. Пользователь видел нашу SPA, шёл на login и не понимал что произошло. Это блокирует self-serve onboarding и был 🔴 E1 в `docs/CATALOG_GAPS.md`.

Цель сессии — закрыть E1 на проде с реальным dev-store, плюс докрутить ещё несколько вещей из фазы 1 (G1 + A4) которые не требуют живого прогона для написания, но проверяются той же install-сессией.

Дополнительно: параллельная сессия в другом терминале вычистила `project/backend/` (legacy V1/V2) и подняла фундамент для переструктуры экспертов (фаза 2).

---

## Approach

### Backend — install flow

1. **Новый handler** `GET /admin/api/integrations/shopify/app` — публичный, HMAC-verified, это «App URL» в Partner Dashboard. Branches:
   - shop уже известен и `connected` → 302 на `/integrations?already_installed=1`
   - shop известен но `disconnected/error` → reinstall path: переиспользуем tenant_id, новый OAuth с `FlowKind=install`
   - shop неизвестен → авто-создание тенанта (slug из `<shop>.myshopify.com`), потом OAuth с `FlowKind=install`
2. **`OAuthState.FlowKind`** — новое поле + миграция `ALTER TABLE admin.oauth_states ADD COLUMN flow_kind TEXT DEFAULT 'connect'`. Существующие connect-флоу не трогаются.
3. **Install completion hook** — после `CompleteOAuth` для `FlowKind=install` фаер'ится горутиной хук, который дёргает `Shopify /shop.json` для email владельца → `MagicLinkUseCase.ProvisionShopOwner` (find/create user → grant membership → email magic-link).
4. **Magic-link infra**: `MagicLinkUseCase.Issue/Consume`, новый POST endpoint `/admin/api/auth/magic`.
5. **Race-fix** в `ProvisionShopOwner`: если новый юзер был создан, а `memberships.Add` упал — не выпускаем магик-линк (иначе юзер залогинится в пустой workspace picker, не сможет выбраться). Существующий юзер с другими membership'ами получает email как обычно.

### Frontend

1. `MagicLinkPage` (`/auth/magic`) — POST'ит код на бэк, adopt session, → `/auth/pick-workspace`.
2. `InstallCompletePage` (`/auth/install-complete?shop=...`) — отдельная страница после Shopify install. Заменила бесполезный редирект на `/auth/sign-in` (который показывал Google/Telegram кнопки нерелевантные мерчанту).

### Bug-fixes найденные на live-тесте

1. **SMTP зависание**. `net/smtp.SendMail` без таймаута молча зависал на Railway-egress (port 587 вероятно блокируется). Симптом: `install_provision_user_created` логировался, потом тишина, ни `magic_link_issued`, ни `magic_link_mail_failed`. Решение — Resend HTTP API адаптер (`api.resend.com/emails`) с 10s таймаутом. SMTP оставлен как fallback для self-hosted SMTP в будущем.
2. **Webhook context cancel**. `HandleWebhook` ack'ал Shopify сразу с 200, потом запускал `HandleWebhook` use-case в горутине через `r.Context()`. После того как HTTP handler возвращал ответ, `r.Context()` отменялся — все DB-roundtrip'ы из горутины падали с `context canceled`. Особенно болезненно для `app/uninstalled`: статус `tenant_integrations` залипал на `connected`, и reinstall попадал в already_connected ветку без нового OAuth. Решение: `context.WithTimeout(context.Background(), 5*time.Minute)` для горутины.

### G1 (V4-чат читает tier2)

`project_v4/backend/internal/adapters/postgres/postgres_catalog.go` — добавлено `mp.tier2` в SELECT всех трёх product-read путей (`ListProducts`, `GetProduct`, `VectorSearch`), скан в `mpTier2JSON []byte`, unmarshal в `domain.Product.Tier2 map[string]any` через `mergeProductWithMaster`. `tools/tool_visual_assembly.go` `ProductToMap` спрэдит Tier2 ПОСЛЕ Extra (typed > Extra > Tier2 на коллизии). 5 unit-тестов на precedence.

### A4 (extractTier2 honour Transform)

`project_admin/backend/internal/usecases/merge_apply.go`. Новый helper `applyTransform(val, transform, defaultUnit)` — switch на: `units.{weight|volume|length|count}` (через `units.Parse` → int canonical), `lowercase`, `shorten:N`, `split:DELIM`, неизвестные → 1:1 пасс-сру. 13 unit-тестов на applyTransform + extractTier2.

### DB hand-fix

В процессе тестирования integration test2 застряла в status=connected (из-за webhook bug который я ещё не пофиксил). Чтобы не создавать платный новый dev-store, сделал прямой UPDATE через `railway run` + `psql`:

```sql
UPDATE admin.tenant_integrations
SET status = 'disconnected', updated_at = NOW()
WHERE external_id = 'test2-mhjhejbs.myshopify.com';
```

После этого reinstall пошёл по правильной ветке.

---

## Files changed

| Path | What |
|---|---|
| `project_admin/backend/internal/handlers/handler_integrations_shopify.go` | `HandleAppEntry` (new). Webhook context fix. Callback redirect branch on FlowKind. |
| `project_admin/backend/internal/handlers/handler_auth_magic.go` | New — POST `/admin/api/auth/magic` consumer endpoint |
| `project_admin/backend/internal/usecases/shopify_v2.go` | `StartInstallEntry` (new), `SetInstallCompletionHook`, `CompleteOAuth` returns FlowKind, fires hook for install. Constructor takes `AdminCatalogPort`. |
| `project_admin/backend/internal/usecases/auth_magic_link.go` | New — `MagicLinkUseCase` (Issue / Consume / ProvisionShopOwner) |
| `project_admin/backend/internal/usecases/auth_magic_link_test.go` | New — 15 unit tests with in-memory port fakes |
| `project_admin/backend/internal/usecases/merge_apply.go` | `extractTier2` honours `target.Transform`. New `applyTransform` helper. |
| `project_admin/backend/internal/usecases/merge_apply_test.go` | +13 tests on applyTransform / extractTier2 |
| `project_admin/backend/internal/adapters/postgres/admin_migrations.go` | `ALTER TABLE admin.oauth_states ADD COLUMN flow_kind TEXT DEFAULT 'connect'` |
| `project_admin/backend/internal/adapters/postgres/integrations_adapter.go` | persist + read `flow_kind` |
| `project_admin/backend/internal/adapters/shopify/client.go` | `GetShopInfo` (new) — `/admin/api/<v>/shop.json` |
| `project_admin/backend/internal/adapters/smtp/mailer.go` + `templates/magic_link.html` | subject "magic_link" + new HTML template |
| `project_admin/backend/internal/adapters/resend/mailer.go` (+ 5 templates) | New — Resend HTTP API adapter |
| `project_admin/backend/internal/config/config.go` | `ResendAPIKey` + `HasResend()` |
| `project_admin/backend/cmd/server/main.go` | Mailer DI prefers Resend, install hook closure, magic-link UC + handler, route registrations |
| `project_admin/backend/cmd/sync-tenant-now/main.go` | Updated for new ShopifyV2UseCase signature |
| `project_admin/backend/internal/domain/integration.go` | `OAuthFlowConnect`/`OAuthFlowInstall` constants + `FlowKind` field |
| `project_v4/backend/internal/domain/product_entity.go` | `Tier2 map[string]any` |
| `project_v4/backend/internal/adapters/postgres/postgres_catalog.go` | tier2 in SELECT/scan/merge in 3 product reads |
| `project_v4/backend/internal/tools/tool_visual_assembly.go` | ProductToMap spreads Tier2 |
| `project_v4/backend/internal/tools/tool_visual_assembly_test.go` | +5 tests on Typed/Extra/Tier2 precedence |
| `project_admin/frontend/src/features/auth/api/authApi.js` | `magicConsume(code)` |
| `project_admin/frontend/src/features/auth/pages/MagicLinkPage.jsx` | New |
| `project_admin/frontend/src/features/auth/pages/InstallCompletePage.jsx` | New — post-install landing |
| `project_admin/frontend/src/App.jsx` | Routes for `/auth/magic` and `/auth/install-complete` |
| `docs/LAUNCH_ROADMAP.md` | Phase 1 partial (E1 closed live), session B (cleanup) merged, what's left to verify |

Commits this session (chronological):
- `8a3357d` feat(v4-catalog): read master_products.tier2 in chat reads
- `0fdaf61` fix(magic-link): block email when newly-created user has no memberships
- `4e1c8d1` feat(catalog): apply Transform in extractTier2 (units/lowercase/shorten/split)
- `fc3c1e6` test: magic-link usecase + ProductToMap tier2 precedence
- `dab0629` fix(mailer): Resend HTTP API adapter (SMTP hangs silently on Railway)
- `0b3e0fb` fix(shopify-webhook): detach goroutine from request context
- `978f10e` feat(install-flow): dedicated post-install landing page

Phase 2 cleanup commits (от параллельной сессии, смёрджены в main):
- `5d30f64` chore(cleanup): delete legacy project/backend, point dev to V4
- `3e8cc3b` docs(updates): log phase2-cleanup session — legacy backend deletion
- `066c959` chore(cleanup): delete legacy project/Dockerfile

---

## Verification

**Live на проде (Railway, Admin сервис):**

1. ✅ **Install** на `test2-mhjhejbs.myshopify.com` через Partner Dashboard
2. ✅ Backend logs показали полную последовательность:
   - `shopify_install_entry_tenant_provisioned` (или `shopify_install_entry_already_connected` после фликания статуса)
   - `http_request method=GET path=/admin/api/integrations/shopify/app status=302`
   - `http_request method=GET path=/admin/api/integrations/shopify/callback status=302`
   - `shopify_v2_initial_ingest_started`
   - `install_provision_user_created`
   - `shopify_v2_dump_to_staging_completed products=17`
   - `harvester_lite_run_completed products_written=17`
3. ✅ Email пришёл на `starknight@keepstar.one` от `noreply@updates.keepstar.one` через Resend
4. ✅ Клик по ссылке → `/auth/magic` → POST `/admin/api/auth/magic` → adopt session → редирект в админку
5. ✅ Каталог показывает 17 snowboard'ов (это тестовый seed Shopify dev-store'а)

**Что НЕ протестировано живьём — для следующей сессии:**

1. **G1**: discovery → apply → V4-чат на этом тенанте должен видеть tier2 поля в виджете. Сейчас просто закоммичено, не подтверждено.
2. **A4**: discovery агент должен эмитнуть `units.weight`/`units.volume` для размеров → проверить в БД что в `master_products.tier2.weight_g` лежит int (а не string `"2.5 kg"`).
3. **Webhook regression**: uninstall test2 → должен дёрнуть наш `app/uninstalled` без ошибки → integration → status=disconnected. Reinstall → пройдёт через reinstall-ветку.
4. **Edge cases install**: bad HMAC → 401, magic-link reuse → expired error на втором клике, magic-link expired (24h+).

**Локально:**

```bash
cd project_admin/backend && go build ./... && go vet ./... && go test ./internal/usecases/... -run "TestApplyTransform|TestExtractTier2|TestLookupPath|TestMagicLink|TestProvisionShopOwner"
# all green

cd project_v4/backend && go build ./... && go vet ./... && go test ./internal/tools/... -run "TestProductToMap"
# all green
```

33 unit-теста за сессию, все зелёные.

---

## Known gaps / caveats

1. **Live-тесты на G1, A4, webhook regression — не сделаны.** В новой сессии после компакта надо начать с этого. Пока не подтверждено живьём — formally закрыты только E1 + race-fix логика.
2. **DB hand-fix** оставил `tenant_integrations.status = disconnected` для test2. На повторном install статус снова станет `connected` через нормальный OAuth путь — никакой остаточной грязи.
3. **Resend sandbox vs verified domain.** Сейчас работает через верифицированный домен `updates.keepstar.one`. Email пойдёт на любой адрес, не только на адрес владельца Resend-аккаунта.
4. **API ключ Resend был выложен в чате во время отладки** (`re_X5F7f9Qu_8nMBNw49JJ95keazJpHWtYyY`). Vlad — после стабилизации флоу нужно его ротейтнуть в Resend Dashboard и обновить env в Railway.
5. **Email-templates дублированы** в `adapters/smtp/templates/` и `adapters/resend/templates/`. Можно зафакторить в общий пакет — но это refactor, не fix. Отложено.
6. **Партнёрская конфигурация в Shopify Partner Dashboard:** App URL = `https://admin-production-4ae4.up.railway.app/admin/api/integrations/shopify/app`. Без этого install из Shopify не попадает в наш handler. Vlad настроил руками во время сессии.
7. **`Use legacy install flow: false`** — Vlad держит включённым managed install. Текущий код работает с обоими режимами потому что мы всё равно проходим через redirect_uri callback.
8. **Phase 2 параллельно**. Cleanup `project/backend/` смёржен в main. Эксперты — в работе в параллельной сессии. Между нашими зонами конфликтов нет (см. `docs/LAUNCH_ROADMAP.md` секции 6.1 + 6.2).
