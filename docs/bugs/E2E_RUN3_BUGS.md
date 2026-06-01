> ⚠️ **HISTORICAL — superseded by docs/v5-known-gaps.md and /SESSION_HANDOFF_2026-05-30.md.** Records bugs fixed through 2026-03-31 (Run 3→Run 4). The demo is now proven end-to-end on prod (2026-05-30); the live gap tracker is docs/v5-known-gaps.md. Kept for reference on iteration cadence. (flagged 2026-06-01)

# E2E Run 3 → Run 4 — Статус багов (2026-03-31)

**Run 3**: 5 PASS / 20 FAIL из 25
**Run 4** (после фиксов Alpha 0.4.6): **20 PASS / 5 FAIL** из 25

---

## Решённые баги

### ✅ BUG-1: Фронтенд отстаёт на 1 шаг (P0) — FIXED

**Фикс**: Заменён `submittingRef` guard на `requestIdRef` (monotonic counter) + `AbortController` в `useChatSubmit.js`. Новый запрос отменяет предыдущий, устаревшие ответы отбрасываются.

**Файлы**: `project/frontend/src/features/chat/useChatSubmit.js`, `project/frontend/src/shared/api/apiClient.js`

**Результат**: 10+ тестов починено. Фронтенд больше не показывает результат предыдущего запроса.

---

### ✅ BUG-4: Layout switch на list не работает (P1) — FIXED

**Фикс**: Убран `layoutKeywords` regex guard в `tool_visual_assembly_v1.go` который откатывал layout если user query не содержал ключевых слов.

**Результат**: Тест #2 "Покажи их списком" — PASS.

---

### ✅ BUG-6: Limit не работает (P1) — FIXED (был следствием BUG-1)

Backend всегда возвращал правильный count. Фронтенд показывал старые данные из-за race condition.

**Результат**: Тест #17 "Покажи топ-5" — PASS.

---

### ✅ BUG-7: Пропущенные запросы (P1) — FIXED

**Причина**: `submittingRef` guard дропал запросы пока предыдущий в полёте.
**Фикс**: Тот же что BUG-1 — AbortController + новые запросы всегда отправляются.

**Дополнительно**: E2E тест-раннер переведён на фиксированную паузу 5 сек между запросами вместо ненадёжного wait_for_response.

---

### ✅ BUG-8: Comparison → grid не переключается (P1) — FIXED (следствие BUG-1)

---

### ✅ BUG-9: Visual modifiers (shadow, accent) не работают (P3) — FIXED

**Фикс**: Добавлена поддержка `shadow` и `borderRadius` из layout node в `GenericCardV2Template.jsx`.

**Результат**: Тест #24 "С тенями и акцентным цветом" — PASS.

---

### ✅ BUG-11: slots=[] всегда пустой (Test framework) — FIXED

**Фикс**: Добавлены `data-slot` и `data-field` атрибуты в `AtomV2Renderer.jsx`.

**Результат**: Все field-ассерты (fields_present, fields_absent, price_before_title) теперь работают.

---

## Оставшиеся баги (5 FAIL)

### 🔴 BUG-3: Show/Hide не редактирует текущую карточку (P0) — OPEN

**Тесты**: #4, #5, #6

**Симптом**: На detail view (#3) запрос "покажи только название и цены" (#4) возвращает grid с 23 товарами вместо редактирования текущей single карточки.

**Рут-кауз**: Промпт. Agent2 получает screen_state.mode="single", но запрос в множественном числе ("название и цены") заставляет LLM думать что пользователь хочет вернуться к гриду. Rule #7 в промпте недостаточно сильный.

**Дополнительно**: На шаге #5 "Добавь рейтинг" — LLM добавляет ещё и фотки (hero), хотя не просили. show:["rating"] не должен тянуть за собой другие поля.

**Что уже сделано**: Добавлены примеры detail card в промпт (V1+V2), усилен Rule #7, engine сохраняет currentMode. Но LLM всё равно часто шлёт layout:"grid".

**Что нужно**: Глубже прокачивать промпт Agent2 — приоритет всегда на том что уже на экране.

---

### 🟡 BUG-12: Small size не даёт 4+ колонки (P2) — FIXING

**Тест**: #12 "Покажи снова сеткой, маленькими" — cols=3 вместо 4+

**Рут-кауз**: `CalcGridConfig` в `defaults.go` для `WidgetSizeSmall` возвращал 3 колонки.

**Фикс применён** (незакоммичен): `WidgetSizeSmall` → 4 колонки для 10+ items.

---

### 🟡 BUG-13: Horizontal карточки — фото мелкие (P2) — FIXING

**Тест**: #13 "Покажи горизонтальными карточками"

**Симптом**: Фотка 140px вместо половины карточки.

**Фикс применён** (незакоммичен): CSS `.generic-card-horizontal .generic-card-media` — `width: 50%` вместо `140px`.

---

### 🟡 BUG-14: Таблица — прозрачный фон (P2) — FIXING

**Тест**: #11 визуально, тест проходит но таблица выглядит плохо.

**Рут-кауз**: `.comparison-product-column` в CSS не имел background — колонки прозрачные.

**Фикс применён** (незакоммичен): Добавлен `background: #FFFFFF` + `border: 1px solid #F1F5F9`.

---

### 🔴 BUG-5v2: state_filter — данные отстают на 1 шаг (P1) — OPEN

**Тест**: #14 "Покажи только COSRX" — показывает 12 виджетов вместо 3.

**Рут-кауз**: Stale read из Neon PostgreSQL. Цепочка:
1. `state_filter` пишет 3 продукта в БД (`pool.Exec(UPDATE)`)
2. Agent1 Go код делает `GetState()` → `pool.Query(SELECT)` может уйти на другой connection в pool → читает старые 23 продукта
3. Agent1 возвращает `ProductsFound = 23`
4. Pipeline строит `microcontext = "filtered: 23 items"`
5. Agent2 Go код делает свой `GetState()` → тоже может прочитать stale 23
6. Agent2 рендерит 23 виджетов

**Проблема не в LLM** — Go код передаёт неправильные числа в промпт. Два `GetState()` подряд на одном turn'е оба читают до того как Neon PostgreSQL propagated write через connection pool.

**Файлы**:
- `project/backend/internal/usecases/agent1_execute.go:287` — GetState после tool
- `project/backend/internal/usecases/agent2_execute.go:113` — GetState в начале
- `project/backend/internal/adapters/postgres/postgres_state.go:420` — zone write без транзакции

**Варианты фикса**:
1. Передать count через Metadata из tool → Agent1 → microcontext (обходит stale read в Agent1)
2. Передать отфильтрованные Products напрямую из pipeline в Agent2 без round-trip в БД
3. Обернуть zone-write + read в одну транзакцию
4. Построить карту state flow чтобы увидеть все точки stale read

---

### 🟡 Preset persistence — не персистится кастомный order/style (P1) — OPEN

**Симптом**: Шаг #8 меняет order (цена первой), шаг #9 просит "крупными" — order сбрасывается к дефолту.

**Рут-кауз**: Каждый turn Agent2 получает свежий AutoResolve без учёта предыдущих кастомизаций. `RenderConfig` хранит fields/mode/size, но V2 engine его не читает как базу — использует только `currentFields` из предыдущей формации.

**Что нужно**: Session-scoped preset — каждое изменение обновляет "рабочий пресет" сессии, последующие запросы строятся поверх него.

---

## Приоритизация (обновлённая)

| Приоритет | Баг | Тесты | Статус |
|-----------|-----|-------|--------|
| **P0** | BUG-3: Detail card show/hide | #4, #5, #6 | OPEN — промпт |
| **P1** | BUG-5v2: Stale state read | #14 | OPEN — архитектура |
| **P1** | Preset persistence | #9 (order) | OPEN — фича |
| **P2** | BUG-12: Small cols | #12 | Фикс применён |
| **P2** | BUG-13: Horizontal photos | #13 | Фикс применён |
| **P2** | BUG-14: Table background | #11 | Фикс применён |
