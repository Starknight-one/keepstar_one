# V5 — Chunk 14: Railway deploy + readyz

## Context

V5 chat engine (chunks 1–13) полностью готов локально. Item 14 + 15 из
`docs/v5-engine-plan.md` — поднять V5 как самостоятельный сервис на
Railway, чтобы Vlad начал реальное тестирование на проде. Без deploy
chunks 15 (V4 vs V5 smoke comparison) и 16 (frontend route swap) не
могут стартовать — обоим нужен реальный prod URL.

## Locked-in decisions

- **Service**: `v5-engine` (mirror `v4-engine` в том же Railway-проекте
  `selfless-tranquility / production`). Domain
  `https://v5-engine-production.up.railway.app`.
- **Auto-deploy**: GitHub repo привяжем через Railway dashboard после
  первого CLI-deploy (CLI flow для repo-link требовал интерактивной
  GitHub OAuth). После привязки каждый push в `v5` ветку будет
  автодеплоиться.
- **Same Neon**: тот же DATABASE_URL что у V4 / Admin / Curator. V5
  миграции (state, preset, component, trace) идемпотентны и создают
  таблицы в namespace `v5_*` — никаких конфликтов.
- **`/readyz`**: пингует только Postgres (1-секундный timeout). Anthropic
  outages surface как 5xx на `/api/v1/pipeline`, для readiness достаточно
  DB.
- **`/healthz`**: остаётся process-alive — это что Railway ставит в
  healthcheck path.
- **Single Docker, V4 pattern**: один image отдаёт и Go-binary, и
  widget IIFE bundle как static на `/`. Mirror `project_v4/Dockerfile`,
  3 stage'а (vite build → go build → alpine runtime).

## Approach

### 1. Backend — `/readyz` endpoint

Новый файл `handler_readyz.go`:
```go
func ReadyzHandler(pool *pgxpool.Pool) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
        defer cancel()
        if err := pool.Ping(ctx); err != nil {
            writeJSON(w, http.StatusServiceUnavailable, ...)
            return
        }
        writeJSON(w, http.StatusOK, map[string]string{"status":"ready"})
    }
}
```

Регистрируется в `routes.go` рядом с `/healthz`, ДО tenant-middleware
(тенант-резолвер не нужен — это infra-probe).

### 2. Backend — static fileserver на `/`

В `RegisterRoutes` добавлен параметр `staticDir string`. Когда он
непустой и директория существует, регистрируется catch-all `GET /`,
который mirror'ит V4 `cmd/server/main.go:347-357`:

```go
mux.HandleFunc("GET /", func(w, r) {
    path := filepath.Join(staticDir, r.URL.Path)
    if st, err := os.Stat(path); err == nil && !st.IsDir() {
        fs.ServeHTTP(w, r); return
    }
    if fileExists(filepath.Join(staticDir, "index.html")) {
        http.ServeFile(w, r, ...); return
    }
    http.NotFound(w, r)
})
```

`config.go` теперь читает `STATIC_DIR` env (default `./static`) и
прокидывает в RegisterRoutes. Локально без вите-билда переменная
указывает на несуществующую папку → fileserver просто не регистрируется.

V5 widget.jsx (line 30) уже умеет автодетектить `apiBaseUrl` из
`script.src.origin`, поэтому same-origin раздача `widget.js`
автоматически делает «embed везде» работающим без `data-api`.

### 3. `project_v5/Dockerfile`

Mirror `project_v4/Dockerfile`:
- Stage 1: `node:22-alpine` → `npm ci` + `npm run build` → IIFE `widget.js`
- Stage 2: `golang:1.24-alpine` → `go build -o server ./cmd/server/`
- Stage 3: `alpine:3.21` + ca-certificates → `COPY` server + frontend
  dist as `./static` → `EXPOSE 8084` (formality) → `CMD ["./server"]`

`npm ci` (а не `npm install`) — детерминированно из package-lock.json.
`CGO_ENABLED=0` — статический Go-binary на alpine.

### 4. Railway service setup

```bash
railway link --project selfless-tranquility
railway add --service v5-engine
railway service v5-engine
railway variable set DATABASE_URL=...        --service v5-engine --skip-deploys
railway variable set ANTHROPIC_API_KEY=...   --service v5-engine --skip-deploys
railway variable set OPENAI_API_KEY=...      --service v5-engine --skip-deploys
railway variable set TENANT_SLUG=hey-babes-cosmetics --service v5-engine --skip-deploys
railway variable set LLM_MODEL=claude-haiku-4-5-20251001 --service v5-engine --skip-deploys
railway variable set LOG_LEVEL=info          --service v5-engine --skip-deploys
railway variable set RAILWAY_DOCKERFILE_PATH=project_v5/Dockerfile --service v5-engine --skip-deploys
railway up --service v5-engine --ci -m "chunk 14 — first deploy"
railway domain --service v5-engine
```

`RAILWAY_DOCKERFILE_PATH=project_v5/Dockerfile` — важно, иначе Railway
по умолчанию ищет Dockerfile в корне репо и попадёт на `project_v4/`.

`PORT` Railway проставляет сам; `config.go:33` уже читает его.

### 5. Smoke verification

Pre-deploy: `go build ./...`, `go test ./...`, `npm test` — все green.
Локального docker'а нет, build верификация через Railway-side.

Post-deploy:
- `GET /healthz` → 200 `{"status":"ok"}` ✅
- `GET /readyz` → 200 `{"status":"ready"}` ✅
- `HEAD /widget.js` → 200, content-type `text/javascript` ✅
- `POST /api/v1/session/init` → sessionId UUID ✅
- `POST /api/v1/pipeline {query:"покажи 3 продукта"}` → document с
  product_card preset replicate=3, 17 spans, cache_read=7546 tokens,
  $0.006 cost, 7.2s latency ✅

### 6. Документы

- Frozen plan: этот файл
- Session log: `docs/Updates/v5/v5_2026-05-03_<HH-MM>_chunk-14.md`
- `docs/v5-engine-plan.md` — items 14 + 15 → ✅, snapshot block
  «chunks 1–14 closed», status table row для chunk 14, секция «What's
  still NOT shippable» — снято «Not deployed»
- `docs/v5-known-gaps.md` — закрыта строка «Latency baseline / Railway
  deploy»
- `CLAUDE.md` — V5 status block, dev-servers таблица
- `docs/Updates/v5/README.md` — index entry для chunk-14

### 7. Commit

`feat(v5): railway deploy + readyz (chunk 14)`

## Critical files

| File | Change |
|---|---|
| `project_v5/Dockerfile` | NEW (3-stage, mirrors V4) |
| `project_v5/backend/cmd/server/main.go` | MODIFY (pass pool + StaticDir to RegisterRoutes) |
| `project_v5/backend/internal/handlers/routes.go` | MODIFY (accept pool + staticDir; register /readyz; static fileserver) |
| `project_v5/backend/internal/handlers/handler_readyz.go` | NEW (DB-ping readiness probe) |
| `project_v5/backend/internal/config/config.go` | MODIFY (StaticDir field, env STATIC_DIR) |

## Known gaps (deferred)

- **GitHub repo auto-deploy**: Railway dashboard → v5-engine → Settings → Source: connect GitHub repo + branch `v5`. После этого каждый push в `v5` будет триггерить deploy. CLI flow для этого требует интерактивной OAuth — отдаётся Vlad'у.
- **No staging environment**: deploy сразу в production (Vlad: «приложением пока никто не пользуется»).
- **No CDN for widget.js**: Railway serves directly. Acceptable для текущей фазы тестирования.
- **`/readyz` only DB**: Anthropic outages surface как 5xx на pipeline endpoint, не как readiness flips.
