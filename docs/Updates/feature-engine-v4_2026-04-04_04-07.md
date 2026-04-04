# feature/engine-v4 — Ops-Only Engine: Deploy & Frontend Fix

**Branch**: `feature/engine-v4` (merged from `feature/engine-v4-pencil`)
**Date**: 2026-04-04, 01:00–04:07 UTC
**Commits**: `51acdd7` (merge) → `cc1a6da` (Dockerfile + landing) → `be0cfc9` (frontend fix)

---

## Summary

Полный деплой V4 ops-only движка: мердж дочерней ветки, настройка Railway, переключение лендинга и фикс фронтенда. Движок работает на проде, виджеты рендерятся.

---

## What was done

### 1. Merge feature/engine-v4-pencil → feature/engine-v4
- Чистый мердж без конфликтов (17 файлов, -575/+485 LOC)
- Все 3 теста pass: nesting, deep nesting, replication
- Go build без ошибок

Что вошло из pencil ветки:
- **Ops-only engine** — пресеты удалены, ops единственный механизм
- **Nesting fix** — `insertLayoutNode()` регистрирует pending ID в `idx.nodes`
- **Default ops** — `ProductCardGridOps()` (9 ops), `ProductDetailOps()` (13 ops)
- **Новый Agent2 промпт** — "You are Agent 2 — a UI builder" (ops-based)
- **Constraints упрощены** — удалены domain-specific правила (D5, W1, W2)
- **`/version` endpoint** — диагностика деплоя
- **ops_test.go** — 3 новых теста

### 2. Root cause: старый промпт в трейсах

**Проблема**: после мерджа и деплоя трейсы показывали старый Agent2 промпт.

**Причина**: `Keepstar_one_landing/index.html` хардкодил оба URL на старый Chat сервис:
```html
<!-- БЫЛО -->
<script src="https://chat-production-005e.up.railway.app/widget.js"
        data-api="https://chat-production-005e.up.railway.app/api/v1">

<!-- СТАЛО -->
<script src="https://v4-engine-production.up.railway.app/widget.js"
        data-api="https://v4-engine-production.up.railway.app/api/v1">
```

Все запросы с keepstar.one шли на старый бэкенд (main ветка), а не на v4-engine.

### 3. Full-stack Dockerfile

V4 Dockerfile переделан из backend-only в full-stack:
```dockerfile
# Stage 1: Build frontend (widget.js)
FROM node:22-alpine AS frontend
COPY project/frontend/ ./
ENV VITE_API_URL=/api/v1
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.24-alpine AS backend
COPY project_v4/backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server/

# Stage 3: Runtime
COPY --from=backend /build/server .
COPY --from=frontend /build/dist ./static
```

### 4. Railway config

JSON patch для v4-engine сервиса:
- `source.rootDirectory`: `/project_v4` → `/` (чтобы Dockerfile видел и `project/frontend/` и `project_v4/backend/`)
- `build.builder`: `RAILPACK` → `DOCKERFILE`
- `build.dockerfilePath`: `project_v4/Dockerfile`
- Порт 8080 на домене

keepstar_one (лендинг) переключен на ветку `feature/engine-v4`.

Ранее `--service-config` dot-path команды не применялись (тихо игнорировались). JSON patch через `railway environment edit --json` сработал.

### 5. Frontend fix — WidgetRenderer gate

**Проблема**: V4 виджеты приходят с `layout` + `atomsV2`, но БЕЗ поля `template`. Фронтенд (`WidgetRenderer.jsx:10`) гейтился на `if (widget.template)` — V4 виджеты проваливались в legacy path, который искал `widget.atoms` (а не `atomsV2`) → пустые div'ы.

**Фикс** (1 строка):
```jsx
// БЫЛО
if (widget.template) {

// СТАЛО
if (widget.template || widget.layout || widget.atomsV2) {
```

Виджеты с layout tree или atomsV2 теперь попадают в GenericCardV2Template → LayoutTreeRenderer.

---

## Architecture (deployed state)

```
keepstar.one (landing)
  └─ widget.js + data-api → v4-engine-production.up.railway.app
      ├─ Frontend: project/frontend/ (widget.js, Shadow DOM)
      ├─ Backend: project_v4/backend/ (Go, port 8080)
      │   ├─ Agent1 → catalog_search → state.data
      │   ├─ Agent2 → visual_assembly (ops) → formation
      │   └─ Engine: ops → replicate → bind → constraints → stamp IDs
      └─ Static: ./static (SPA file server)
```

### Services on Railway

| Service | Branch | Root | Purpose |
|---------|--------|------|---------|
| v4-engine | feature/engine-v4 | / | V4 backend + frontend |
| Chat | main | /project | Old V1/V2 backend (unused by landing now) |
| Admin | main | /project_admin | Admin dashboard + traces |
| keepstar_one | feature/engine-v4 | /Keepstar_one_landing | Landing page |

---

## Verification

- `/version` → `{"build":"v4-nesting-fix-1c8634d","prompt":"ui-builder","date":"2026-04-04"}`
- `/widget.js` → 200 OK, `text/javascript`
- "покажи крема" → 50 виджетов, grid 3 cols, layout tree + 9 atomsV2 each
- Трейсы: Agent2 prompt = "You are Agent 2 — a UI builder", tool = visual_assembly, 563 output tokens
- Фронтенд рендерит карточки через LayoutTreeRenderer

---

## Key learnings

1. **Railway `--service-config` dot-path** может тихо не применяться. JSON patch через `--json` надёжнее. Всегда верифицировать через `railway environment config --json`.

2. **`data-api` в landing HTML** — хардкод, не связан с переменными админки (`WIDGET_BASE_URL`). При смене бэкенда нужно менять и HTML.

3. **Frontend/backend contract**: если бэкенд меняет формат виджета (убирает поле `template`), фронтенд нужно обновлять. V4 Widget = `{layout, atomsV2}` без `template`.

4. **Два сервиса = два бэкенда**: Chat (main) и v4-engine (feature/engine-v4) — разный код, одна БД. Трейсы показывают промпт того бэкенда, который обработал запрос.

---

## Testing results (live on keepstar.one)

### "покажи крема" — grid карточек
- Agent1: catalog_search → 50 products
- Agent2: 563 tokens, visual_assembly → grid 3 cols, 50 widgets x 9 atoms
- Рендер: **работает после фронтенд-фикса** (WidgetRenderer gate)

### "лендинг из первых трёх" — replication bug
- Agent1: правильно не вызвал tool (данные есть)
- Agent2: построил 1 красивый виджет (заголовок + row из 3 product columns)
- **Баг**: движок реплицировал шаблон на все 50 продуктов → 51 виджет, мусор на экране
- **Причина**: V4 не поддерживает `limit` — репликация всегда берёт ВСЕ данные
- В V1/V2 Agent2 мог указать `limit: 3` в tool input → data slice перед engine

### "freestyle виджет + карточка рядом" — partial success
- Agent2 попытался: update existing formation (2 cols) + добавить text widget + product card
- Результат: 2 product cards (2 cols) + старый "Сравнение топ-3" виджет + новый "Привет, это крутой движок" + пустая карточка
- **Проблемы**: state carry-over от прошлых запросов, нет true freestyle (текстовый виджет без data binding)

---

## Known gaps (next session)

### 1. Нет limit/data slicing
- V4 visual_assembly tool не имеет `limit`/`offset` параметров
- Репликация в engine.go всегда берёт ВСЕ `input.Data`
- **Fix**: добавить `limit` в tool schema + slice `input.Data` перед replication
- **Reference**: `project/backend/internal/tools/tool_visual_assembly.go:145-152` (V1/V2 реализация)

### 2. Нет default ops (пресеты-заменители)
- Пресеты удалены, default_ops.go содержит только `ProductCardGridOps()` и `ProductDetailOps()`
- Пресеты экономили токены (Agent2 мог сказать "preset=X" вместо 10+ ops)
- **Нужно**: механизм "named ops bundles" — Agent2 вызывает по имени, не тратит токены на описание каждого op

### 3. Нет multi-widget creation
- Agent2 может создать 1 шаблон → реплицируется. Или update existing
- Не может создать 2+ РАЗНЫХ виджета в одном запросе (напр. hero + grid + text block)
- В V1/V2 это делалось через sections в build mode

### 4. State carry-over
- Existing formation не очищается между turn'ами если Agent2 не делает `delete formation`
- Виджеты от прошлых запросов "протекают" в текущий рендер

### 5. Freestyle widgets (no data binding)
- Agent2 может создать виджет с `value: "text"` (литерал), но движок всё равно пытается bind/replicate
- Нужен флаг "этот виджет — freestyle, не привязывать к данным"

---

## Files changed (this session)

| Action | File | Change |
|--------|------|--------|
| MERGE | 17 files from feature/engine-v4-pencil | Ops-only engine (see previous spec) |
| EDIT | `project_v4/Dockerfile` | Backend-only → full-stack (frontend + backend) |
| EDIT | `Keepstar_one_landing/index.html` | URLs → v4-engine-production |
| EDIT | `project/frontend/src/entities/widget/WidgetRenderer.jsx` | Gate: template → template OR layout OR atomsV2 |
