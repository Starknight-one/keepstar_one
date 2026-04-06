# V2 Engine — Known Issues & Future Work

Обновлено: 2026-03-20. Три раунда: тестирование + полный аудит движка.

> **Аудит V2 движка (2026-03-20)**: сравнение спеки с реализацией выявило что из 41 операции спеки реализовано ~13, tool schema — V1 интерфейс с костылём-конвертером, 7 из 18 параметров не работают в V2. Подробности: `docs/UPDATES.md` → "Аудит V2 Engine".

---

## ИСПРАВЛЕНО (Alpha 0.0.6)

### 1. TenantSlug → tenant_id mismatch (fieldDefCount=0)
`field_definition_adapter.go` делал `WHERE tenant_id = $1` со slug строкой, а `tenant_id` — UUID. Теперь JOIN на `catalog.tenants`, матчит slug или UUID. **fieldDefCount=13 на проде.**

### 2. Agent2 промпт: show vs hide семантика
"Только X и Y" теперь корректно маппится на hide (убирает всё кроме), а не show. Добавлены примеры и CRITICAL блок в оба промпта (v1/v2).

### 3. Двойные hero-картинки
GenericCardV2Template рендерил hero ImageCarousel + LayoutTreeRenderer рендерил hero из layout tree. Теперь ImageCarousel только в fallback (без layout tree), LayoutTreeRenderer единственный источник hero.

### 4. C1 Rule удаляла явно запрошенные show-поля
show-поля теперь protected от cross-widget normalization (C1 70% threshold).

---

## ИСПРАВЛЕНО (Стабилизация — фаза 1, uncommitted)

### P1. Agent2 тянет контекст из предыдущих запросов — FIXED
**Было**: Agent2 тащил show/hide/layout из предыдущих запросов через conversation history.
**Фикс**: Заменена источник истории. Теперь `state.Agent2History` — отдельное JSONB поле с tool_use/tool_result самого Agent2 (не user-сообщения из Agent1). Лимит: 4 сообщения (2 турна). Agent2 видит свои прошлые вызовы и делает точные дельты.
**Файлы**: `domain/state_entity.go`, `ports/state_port.go`, `adapters/postgres/postgres_state.go`, `adapters/postgres/state_migrations.go`, `usecases/agent2_execute.go`

### P2. Description + пустые строки в данных — FIXED
**Было**: пустые строки от отсутствующих полей ломали рендер.
**Фикс**: engine_v2.go пропускает поля с пустыми строками. Добавлен LineClamp в `DisplayToTextStyleWrapper` для длинных текстов (h1/h2→2, h3/h4→3, body-sm→4). AtomV2.css — `display: block` + `word-break: break-word`.
**Файлы**: `engine/engine_v2.go`, `entities/atom/AtomV2.css`

### P3. List layout — фотки на весь экран — FIXED
**Было**: list mode = карточки одна под одной, images 100% ширины.
**Фикс**: CSS — list карточки `flex-direction: row`, images 120×120px (thumbnail), layout spans фиксированы на 120px.
**Файл**: `entities/formation/Formation.css`

### P4. Single/detail карточка не full-width — FIXED
**Было**: size=large карточка ограничена max-width контейнера.
**Фикс**: `.formation-single > .size-large { max-width: 100% }`.
**Файл**: `entities/formation/Formation.css`

---

## ТРЕБУЕТ ПРОРАБОТКИ (не блокеры для демо)

### A. MaxFields — ограничение количества полей
MaxFields жёстко режет количество атомов (3 для small, 5 для medium). Show-поля теперь поднимают MaxFields, но сама концепция грубая.

**Идеи**: MaxArea вместо MaxFields, adaptive по контенту, cap только для auto-выбора.

### B. Agent2 интерпретация (помимо P1)
- "Крупными карточками" → до фикса давал comparison. После фикса size=large работает, но контекст портит.
- Позиционный выбор ("первый и два последних") не поддерживается — limit/offset не позволяет.
- Compose ("карточка + карусель рядом") — Agent2 не использует параметр compose.

### C. Comparison preset выглядит плохо
Отображается как обычные карточки рядом. Нет: общих строк, выделения отличий, табличной структуры.

### D. Table layout некрасивый
Работает, но визуально "всратый" (цитата из тестирования).

### E. Agent2 не имеет своей conversation history — SOLVED (P1 fix)
Решено в рамках P1: `Agent2History` добавлена в state, накапливает tool_use + tool_result.

### F. Нет полного LLM request в трейсах (observability)
Трейсы записывают `promptSent`, `toolInput`, `toolResult` — но НЕ записывают полный LLM request: все messages (включая 4 user-сообщения из истории), system prompt, tool definitions, raw response. Приходится реверс-инжинирить из кода что агент видит.

**Решение**: в `agent1_execute.go` и `agent2_execute.go` перед вызовом LLM логировать полный request (messages array, system prompt, tools). Сохранять в trace как `fullLLMRequest`. Тогда в админке видно ровно то что видит агент — никаких догадок.

### G. Длинные названия продуктов — PARTIALLY SOLVED (P2 fix)
LineClamp добавлен в `DisplayToTextStyleWrapper` (h1→2 строки, h3→3, body-sm→4). CSS `text-overflow: ellipsis` + `word-break: break-word` в AtomV2.css. Для полного решения может потребоваться truncate на уровне движка (не только CSS).

---

## Тестовые сессии

| Сессия | Кол-во | Дата | Контекст |
|--------|--------|------|----------|
| e4a5b7d9 | 3 | 2026-03-18 | Первый тест, до enriched traces |
| 3c23736c | 12 | 2026-03-18 | Основной тест Alpha 0.0.5 |
| ef229e2b | 4 | 2026-03-18 | Второй тест Alpha 0.0.5 |
| e160151c | 17 | 2026-03-18 | Тест Alpha 0.0.6 (после 4 фиксов) |

Трейсы: https://admin-production-4ae4.up.railway.app/traces
