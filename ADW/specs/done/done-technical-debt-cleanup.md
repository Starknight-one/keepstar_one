# Done: Technical Debt Cleanup

**Дата:** 2026-02-10
**Ветка:** `chore/technical-debt-cleanup`
**Статус:** Реализовано, `go build`, `go vet`, `npm run build` проходят

## Что сделано

### Фаза 1 — Надёжность

#### Task 1: pgx.ErrNoRows вместо строковых проверок
**Файл:** `project/backend/internal/adapters/postgres/postgres_catalog.go`
- Добавлены imports: `"errors"`, `"github.com/jackc/pgx/v5"`
- 4 замены `err.Error() == "no rows in result set"` → `errors.Is(err, pgx.ErrNoRows)`:
  - `GetTenantBySlug` — строка ~49
  - `GetMasterProduct` — строка ~116
  - `GetProduct` — строка ~368
  - `GetCatalogDigest` — строка ~865

#### Task 2: mergeProductWithMaster() хелпер
**Файл:** `project/backend/internal/adapters/postgres/postgres_catalog.go`
- Добавлен `masterProductRow` struct (7 полей: MasterProductID, Name, Description, Brand, CategoryName, ImagesJSON, AttributesJSON)
- Добавлена функция `mergeProductWithMaster(p *domain.Product, mp masterProductRow) error`
- Заменены дублированные merge-блоки в 3 методах:
  - `ListProducts` — ~25 строк → 10 строк вызова хелпера
  - `GetProduct` — ~25 строк → 10 строк вызова хелпера
  - `VectorSearch` — ~20 строк → 10 строк вызова хелпера

#### Task 3: Обработка ошибок json в postgres_state.go
**Файл:** `project/backend/internal/adapters/postgres/postgres_state.go`
- Добавлен import `"log/slog"`
- **Marshal (запись в БД) → return error** (16 мест):
  - `CreateState` — 4 marshal
  - `UpdateState` — 6 marshal
  - `AddDelta` — 3 marshal (action, result, template)
  - `UpdateData` — 2 marshal (data, meta)
  - `UpdateTemplate` — 1 marshal
  - `UpdateView` — 2 marshal (focused, stack)
  - `AppendConversation` — 1 marshal
  - `PopView` — 1 marshal (newStackJSON)
- **Unmarshal (чтение из БД) → slog.Warn + continue** (12 мест):
  - `GetState` — 7 unmarshal (data, meta, template, viewFocused, viewStack, conversationHistory)
  - `scanDeltas` — 3 unmarshal (action, result, template)
  - `PopView` — 1 unmarshal (viewStack)
  - `GetViewStack` — 1 unmarshal (viewStack)
- **Exec sync → slog.Warn:**
  - `AddDelta` step sync — `_, _ = Exec` → `if syncErr != nil { slog.Warn(...) }`

#### Дополнительно: postgres_catalog.go unmarshal
**Файл:** `project/backend/internal/adapters/postgres/postgres_catalog.go`
- Добавлен import `"log/slog"`
- `GetMasterProductsWithoutEmbedding` — `_ = json.Unmarshal` → `slog.Warn`
- `GetAllTenants` — `_ = json.Unmarshal` → `slog.Warn`

---

### Фаза 2 — Чистота

#### Task 4: Логирование VectorSearch
**Файл:** `project/backend/internal/tools/tool_catalog_search.go`
- `vectorProducts, _ = VectorSearch(...)` → `vectorProducts, vectorErr = VectorSearch(...)`
- При ошибке: `meta["vector_error"] = vectorErr.Error()` — не прерывает, fallback на keyword-only

#### Task 5: Унификация логирования
8 мест `log.Printf` → structured logger:

| Файл | Что изменено |
|------|-------------|
| `handlers/handler_chat.go` | Добавлено поле `log *logger.Logger`, конструктор принимает `*logger.Logger`, `log.Printf("Chat error: %v")` → `h.log.Error("chat_error", ...)` |
| `handlers/handler_catalog.go` | Добавлено поле `log *logger.Logger`, конструктор принимает `*logger.Logger`, `log.Printf("ListProducts error: %v")` → `h.log.Error("list_products_error", ...)` |
| `usecases/chat_send_message.go` | Добавлено поле `log *logger.Logger`, конструктор принимает `*logger.Logger`, 5× `log.Printf` → `uc.log.Warn(...)` |
| `adapters/anthropic/anthropic_client.go` | Import `"log"` → `"log/slog"`, `log.Printf("[WARN] slow LLM TTFB: ...")` → `slog.Warn("slow LLM TTFB", ...)` |
| `cmd/server/main.go` | `NewSendMessageUseCase(..., appLog)`, `NewChatHandler(..., appLog)`, `NewCatalogHandler(..., appLog)` |

#### Task 6: Вынос утилит шаблонов (FE)

**Создан:** `project/frontend/src/entities/widget/templates/templateUtils.js`
- `groupAtomsBySlot(atoms)` — группировка атомов по слотам
- `normalizeImages(value)` — нормализация значения в массив URL

**Создан:** `project/frontend/src/entities/widget/templates/ImageCarousel.jsx`
- Единый компонент карусели для card-шаблонов
- `e.stopPropagation()` на dots — безопасный default (предотвращает всплытие клика на карточку)
- Клик по картинке → переход к следующей

**Обновлены 4 файла:**

| Файл | Удалено | Добавлено |
|------|---------|-----------|
| `ProductCardTemplate.jsx` | Локальные `groupAtomsBySlot`, `normalizeImages`, `ImageCarousel` | Импорт из `templateUtils` + `ImageCarousel` |
| `ServiceCardTemplate.jsx` | Локальные `groupAtomsBySlot`, `normalizeImages`, `ImageCarousel` | Импорт из `templateUtils` + `ImageCarousel` |
| `ProductDetailTemplate.jsx` | Локальные `groupAtomsBySlot`, `normalizeImages` | Импорт из `templateUtils` (ImageGallery с thumbnails — оставлен локальным) |
| `ServiceDetailTemplate.jsx` | Локальные `groupAtomsBySlot`, `normalizeImages` | Импорт из `templateUtils` (ImageGallery с thumbnails — оставлен локальным) |

---

### Фаза 3 — Гигиена

#### Task 7: Мёртвый код BE

| Что | Действие |
|-----|----------|
| `tools/mock_tools.go` | Удалён файл (414 строк padding tools) |
| `tool_registry.go:71-72` | Удалена строка `defs = append(defs, GetCachePaddingTools()...)` |
| `adapters/json_store/` | Удалена директория (json_product_store.go + README.md) |
| `chat_send_message.go:13-15` | Удалена константа `DefaultSessionTTL` (deprecated, заменена `domain.SessionTTL`) |

#### Task 8: Мёртвый код FE

| Что | Действие |
|-----|----------|
| `src/app/` (App.jsx + App.css) | Удалена директория |
| `src/styles/` | Удалена директория (была пустая) |
| `src/entities/atom/atoms/` | Удалена директория (была пустая) |
| `shared/ui,hooks,lib,logger/` | **Оставлены** (scaffold для будущей разработки) |

---

## Верификация

### Backend
```
$ cd project/backend && go build ./...   # PASS
$ go vet ./...                           # PASS
```

### Grep-проверки (все возвращают 0 совпадений в коде)
```
grep -rn 'err.Error() == "no rows'  internal/         → 0
grep -rn '_ = json\.\|_, _ ='      internal/adapters/ → 0
grep -rn 'log\.Printf'             internal/          → 0
grep -rn 'CachePadding'            internal/          → 0 (только README)
grep -rn 'DefaultSessionTTL'       internal/          → 0
```

### Frontend
```
$ cd project/frontend && npm run build   # PASS (492ms, 71 modules)
```

## Затронутые файлы

### Backend (11 файлов изменено, 3 удалено)
- `internal/adapters/postgres/postgres_catalog.go` — ErrNoRows, merge helper, slog unmarshal
- `internal/adapters/postgres/postgres_state.go` — marshal/unmarshal error handling
- `internal/adapters/anthropic/anthropic_client.go` — slog.Warn вместо log.Printf
- `internal/tools/tool_catalog_search.go` — vector error logging
- `internal/tools/tool_registry.go` — удаление CachePadding
- `internal/handlers/handler_chat.go` — structured logger
- `internal/handlers/handler_catalog.go` — structured logger
- `internal/usecases/chat_send_message.go` — structured logger, удаление DefaultSessionTTL
- `cmd/server/main.go` — обновление конструкторов
- ~~`internal/tools/mock_tools.go`~~ — удалён
- ~~`internal/adapters/json_store/`~~ — удалена директория

### Frontend (6 файлов изменено/создано, 3 директории удалены)
- `src/entities/widget/templates/templateUtils.js` — NEW
- `src/entities/widget/templates/ImageCarousel.jsx` — NEW
- `src/entities/widget/templates/ProductCardTemplate.jsx` — рефакторинг импортов
- `src/entities/widget/templates/ServiceCardTemplate.jsx` — рефакторинг импортов
- `src/entities/widget/templates/ProductDetailTemplate.jsx` — рефакторинг импортов
- `src/entities/widget/templates/ServiceDetailTemplate.jsx` — рефакторинг импортов
- ~~`src/app/`~~ — удалена
- ~~`src/styles/`~~ — удалена
- ~~`src/entities/atom/atoms/`~~ — удалена

## API Changes
Нет. Все изменения внутренние — рефакторинг, error handling, logging. Публичный API не затронут.
