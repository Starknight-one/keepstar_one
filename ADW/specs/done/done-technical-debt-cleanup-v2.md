# Chore: Technical Debt Cleanup

## ADW ID: chore-technical-debt-cleanup

## Контекст

Аудит кодовой базы выявил ряд повторяющихся паттернов и хрупких мест. Ничего критичного для пользователя, но накапливается debt который замедлит дальнейшую разработку.

Сиды БД НЕ входят в скоуп — будут убраны при реализации Admin Catalog Import API.

---

## Задачи

### 1. Backend: Исправить string-based error check на pgx.ErrNoRows

**Файл:** `project/backend/internal/adapters/postgres/postgres_catalog.go`

**Проблема:** В 4 местах проверка ошибки "no rows" идёт через строковое сравнение `err.Error() == "no rows in result set"`, тогда как в `postgres_state.go` и `postgres_cache.go` используется корректная проверка через `errors.Is(err, pgx.ErrNoRows)`. При обновлении pgx или изменении текста ошибки — сломается молча.

**Где:**
- `:49` — `GetTenantBySlug()`
- `:116` — `GetMasterProduct()`
- `:368` — `GetProduct()`
- `:865` — `GetCatalogDigest()`

**Решение:** Заменить на `errors.Is(err, pgx.ErrNoRows)`, добавить `"errors"` в imports.

**Оценка:** ~5 минут

---

### 2. Backend: Вынести mergeProductWithMaster() в приватный хелпер

**Файл:** `project/backend/internal/adapters/postgres/postgres_catalog.go`

**Проблема:** Идентичный блок логики мёрджа продукта с мастер-продуктом скопирован в 3 методах:
- `ListProducts()` — строки 284-328
- `GetProduct()` — строки 381-412
- `VectorSearch()` — строки 510-534

Логика: парсинг картинок продукта → проверка master_product_id → fallback name/description из мастера → бренд из мастера → категория → fallback картинок → парсинг атрибутов → formatPrice.

При изменении схемы мастер-продукта (а это будет при админке) — нужно менять в 3 местах.

**Решение:** Создать приватную структуру `masterProductRow` для scan-данных и хелпер `mergeWithMaster(p *domain.Product, row masterProductRow) error` в том же файле.

**Будущий контекст (Admin API):** Эта merge-логика станет частью публичного поведения системы. Через админку тенант сможет "дополнять" продукт — переопределять name, description, images поверх мастера. Текущий merge (fallback на мастер если поле пустое) — это по сути премиум-функция "автозаполнение из мастер-каталога", которую можно будет включить/выключить на уровне тенанта.

Поэтому при выносе хелпера стоит учесть:
- Хелпер должен быть чистым и тестируемым — без завязки на SQL scan
- Входные данные: `*domain.Product` + данные мастера (структура `masterProductRow`)
- В будущем хелпер может быть переиспользован из Admin API handler-а (при импорте/preview каталога)
- Когда появится toggle "использовать мастер-данные" в tenant.Settings — условие добавится в одном месте внутри этого хелпера

**Оценка:** ~15 минут

---

### 3. Backend: Обработать проглоченные ошибки в postgres_state.go

**Файл:** `project/backend/internal/adapters/postgres/postgres_state.go`

**Проблема:** 8+ мест где `json.Marshal` / `json.Unmarshal` / `Exec` вызываются с `_ =` или `_, _ =`. Если JSON сериализация сломается (невалидный UTF-8, циклические ссылки) — в БД запишутся невалидные данные без какого-либо сигнала.

**Где (основные):**
- `:44-47` — `CreateState()` — 4 маршала
- `:117-122` — `UpdateState()` — 6 маршалов
- `:145-149` — `AddDelta()` — маршалы action/result/template
- `:186-189` — `Exec` для sync update
- `:199-234` — `UpdateData`, `UpdateTemplate`, `UpdateView`, `AppendConversation`
- `:330-333` — `scanDeltas()` — unmarshals
- `:381-425` — `PopView`, `GetViewStack` — unmarshals

**Решение:** Добавить `if err != nil { return fmt.Errorf("marshal ...: %w", err) }`. Для unmarshals в scan-методах — логировать ошибку + продолжать (graceful degradation), не роняя весь запрос.

**Оценка:** ~10 минут

---

### 4. Backend: Обработать ошибку VectorSearch в tool_catalog_search.go

**Файл:** `project/backend/internal/tools/tool_catalog_search.go`

**Проблема:** Строка 240 — `vectorProducts, _ = t.catalogPort.VectorSearch(...)`. Если vector search упал (pgvector extension недоступен, embedding NULL) — ошибка теряется, fallback на пустой результат. Пользователь получает только keyword results без понимания что часть поиска не работает.

**Решение:** Логировать ошибку через logger, но не прерывать выполнение (vector search — optional enhancement).

**Оценка:** ~3 минуты

---

### 5. Backend: Унифицировать логирование

**Проблема:** Mix `log.Printf()` (stdlib) и structured logger в нескольких местах:
- `handler_chat.go:66`
- `handler_catalog.go:107`
- `chat_send_message.go:77, 153, 165, 175, 187`

**Решение:** Заменить все `log.Printf()` на `logger.Info()` / `logger.Error()` с structured fields.

**Оценка:** ~5 минут

---

### 6. Frontend: Вынести общие утилиты шаблонов

**Файлы:**
- `src/entities/widget/templates/ProductCardTemplate.jsx`
- `src/entities/widget/templates/ServiceCardTemplate.jsx`
- `src/entities/widget/templates/ProductDetailTemplate.jsx`
- `src/entities/widget/templates/ServiceDetailTemplate.jsx`

**Проблема:** 3 функции дублируются во всех 4 файлах:
1. `groupAtomsBySlot(atoms)` — группировка атомов по слотам (4 копии)
2. `normalizeImages(value)` — нормализация картинок в массив (4 копии)
3. `SLOTS` — объект с именами слотов (4 копии)

Также `ImageCarousel` — почти идентичный компонент в ProductCard и ServiceCard (разница: в ServiceCard есть промежуточная функция `handleDotClick`, в ProductCard — инлайн `e.stopPropagation()`).

**Решение:**
1. Создать `src/entities/widget/templates/templateUtils.js` — экспорт `SLOTS`, `groupAtomsBySlot`, `normalizeImages`
2. Создать `src/entities/widget/templates/ImageCarousel.jsx` — единый компонент карусели
3. Импортировать во всех 4 шаблонах

**Оценка:** ~20 минут

---

### 7. Backend: Удалить мёртвый код

**Что удалить:**
- `chat_send_message.go:13-15` — deprecated `DefaultSessionTTL` (не используется, есть `domain.SessionTTL`)
- `tools/mock_tools.go` — 20 фейковых тулов для паддинга порога кэша Anthropic (реальных тулов уже достаточно, контекст вырос)
- `adapters/json_store/json_product_store.go` — ранняя MVP-заглушка "каталог из JSON-файла", заменена на `postgres.CatalogAdapter`, нигде не подключена

**НЕ удалять:**
- `adapters/memory/memory_cache.go` — заглушка под будущую фичу instant navigation (см. "Не входит в скоуп")

**Решение:** Удалить файлы/код.

**Оценка:** ~5 минут

---

### 8. Frontend: Удалить мёртвый код

**Что удалить:**
- `src/app/App.jsx` + `src/app/App.css` — дублирует `src/App.jsx`, никем не импортируется (`main.jsx` ссылается на `./App.jsx` — корневой)
- `src/styles/` — пустая, глобальные стили живут в `index.css` + `shared/theme/`
- `src/entities/atom/atoms/` — пустая, задумывалась под декомпозицию AtomRenderer, не реализована

**Пустые директории — оставить (scaffold под будущие фичи):**
- `shared/ui/` — сменные design kits для тенантов (выбор/генерация дизайн-системы)
- `shared/hooks/` — переиспользуемые React-хуки (когда появятся)
- `shared/lib/` — утилиты не привязанные к React (форматирование, валидация, i18n)
- `shared/logger/` — структурированный фронтенд-логгер для production monitoring

**Оценка:** ~3 минуты

---

## Порядок выполнения

**Фаза 1 — Надёжность (High, ~30 мин):**
1. Задача #1 — pgx.ErrNoRows → устраняет хрупкость, 4 правки в одном файле
2. Задача #3 — ошибки json в state → без этого данные могут молча портиться
3. Задача #2 — mergeProductWithMaster() хелпер → блокирует будущую работу по Admin API

**Фаза 2 — Чистота (Medium, ~28 мин):**
4. Задача #4 — логирование VectorSearch → быстро, 1 строка
5. Задача #5 — унификация log.Printf → можно сделать вместе с #4 за один проход
6. Задача #6 — вынос утилит шаблонов на фронте → самая объёмная, но изолированная

**Фаза 3 — Гигиена (Low, ~8 мин):**
7. Задача #7 — удаление мёртвого кода BE
8. Задача #8 — удаление мёртвого кода FE

Фазы 1 и 2 можно делать параллельно (BE и FE независимы). Фазу 3 — в конце, чтобы не мешать диффам.

## Сводка

| # | Слой | Описание | Приоритет | Оценка |
|---|------|----------|-----------|--------|
| 1 | BE | pgx.ErrNoRows вместо строк | High | ~5 мин |
| 2 | BE | mergeProductWithMaster() хелпер | High | ~15 мин |
| 3 | BE | Обработка ошибок json в state | High | ~10 мин |
| 4 | BE | Логирование ошибки VectorSearch | Medium | ~3 мин |
| 5 | BE | Унификация логирования | Medium | ~5 мин |
| 6 | FE | Вынос утилит шаблонов | Medium | ~20 мин |
| 7 | BE | Удаление мёртвого кода | Low | ~5 мин |
| 8 | FE | Удаление мёртвого кода | Low | ~3 мин |

**Итого:** ~66 минут, 8 задач, 0 новых фич, 0 изменений API.

---

## Не входит в скоуп

- DB seeds (будут убраны при Admin Catalog Import API)
- CI/CD pipeline
- TypeScript миграция
- Фронтенд-тесты
- i18n (русские строки)
- Barrel exports (index.js)

### Instant Navigation (отдельная фича, требует спеку)

`adapters/memory/memory_cache.go` — заглушка под механизм мгновенной навигации по карточкам. Сейчас при клике на товар в гриде → поход в БД → Agent2 рендер → 1-2+ секунды. Целевое поведение:

**Вперёд (grid → detail):** Когда Agent2 отрендерил грид, бэкенд сразу пре-рендерит detail-карточки для каждого товара в фоне и кладёт в in-memory кэш. При клике на карточку — отдаём готовый рендер мгновенно без LLM и без похода в БД. Если пользователь попросил через чат показать конкретный товар в специфичном формате — тогда Agent2 отрабатывает как обычно.

**Назад (detail → grid):** Когда пользователь жмёт "назад" — отдаём закэшированный рендер грида из предыдущего turn-а, без пересборки. Если пользователь через чат попросил "покажи обратно но иначе" — Agent2 отрабатывает.

**Суть:** Кэш хранит готовые рендеры (widgets + formations) привязанные к session + turn. CachePort уже описывает контракт (GetSession/SaveSession + CacheProducts/GetCachedProducts), но потребуется расширить под рендер-кэш (не только products, но и готовые widget trees).
