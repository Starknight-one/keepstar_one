# Fix: Agent1 State Context — Stop firing on style requests

## ADW ID: fix-agent1-state-context

## Контекст

**BUG-2 из `bug-agent2-smart-render-qa`:** Agent1 (data retrieval) вызывает `catalog_search` на style-запросы вроде "покажи крупнее с рейтингом", потому что не знает что данные уже загружены. Agent1 получал только `ConversationHistory` + raw query — ни `ProductCount`, ни `Fields`, ни `RenderConfig`.

**Результат бага:** лишний LLM tool call + 3-6 секунд задержки + лишние деньги.

## Решение

Передать Agent1 краткую сводку текущего стейта в user message (паттерн из Agent2 `BuildAgent2ToolPrompt`). Enriched message эфемерен — в conversation history сохраняется raw query.

## Что сделано

### 1. `prompt_analyze_query.go` — новая функция + обновлённый промпт

**`BuildAgent1ContextPrompt(meta, currentConfig, userQuery)`** — обогащает запрос блоком `<state>`:
- `loaded_products` / `loaded_services` — сколько данных загружено
- `available_fields` — какие поля доступны (rating, price, images...)
- `current_display` — текущий пресет/режим/размер (если formation есть)
- `displayed_fields` — какие поля сейчас отображаются

Если данных нет (ProductCount=0, ServiceCount=0) — возвращает raw query без `<state>`.

**Agent1SystemPrompt** — добавлены правила 9-10:
- Если `<state>` есть и запрос про поля из `available_fields` → style request, не вызывать tool
- Если `<state>` отсутствует → новый запрос данных
- Примеры: "покажи крупнее с рейтингом" + state has rating → DO NOT call; "покажи Adidas" + state has Nike → catalog_search

### 2. `agent1_execute.go` — enriched message для LLM

Между tenant context и messages добавлено:
- Обновление `meta.ProductCount` / `meta.ServiceCount` из фактических данных
- Извлечение `currentConfig` из formation (паттерн из `agent2_execute.go`)
- Построение `enrichedQuery` через `BuildAgent1ContextPrompt`
- В LLM отправляется enriched message, в history сохраняется raw `req.Query`

### 3. Debug trace — отображение enriched query

- `Agent1ExecuteResponse.EnrichedQuery` — новое поле
- `AgentTrace.EnrichedQuery` — прокидывается через trace entity
- `pipeline_execute.go` — маппинг response → trace
- `handler_trace.go` — expandable "Enriched Query (with state)" на debug-странице

## Файлы

| Файл | Что изменено |
|------|-------------|
| `prompts/prompt_analyze_query.go` | `BuildAgent1ContextPrompt()`, правила 9-10 в системном промпте |
| `usecases/agent1_execute.go` | Enriched message, meta counts, currentConfig extraction |
| `domain/trace_entity.go` | `EnrichedQuery` поле в `AgentTrace` |
| `usecases/pipeline_execute.go` | Маппинг `EnrichedQuery` в trace |
| `handlers/handler_trace.go` | UI отображение enriched query |

## Edge cases

| Кейс | Поведение |
|------|-----------|
| Первое сообщение (нет данных) | `<state>` не добавляется, raw query as-is |
| Данные есть, но formation нет | `current_display` отсутствует, но counts и fields видны |
| State после DB read (map[string]interface{}) | Type assertion к `*FormationWithData` = false, `currentConfig = nil` |
| Запрос на новые данные ("покажи Adidas") | LLM видит что загружены Nike → вызывает catalog_search |

## Кэширование

- System prompt = `const` → `CacheSystem: true` работает
- History хранит raw queries → `CacheConversation` работает
- Enriched message всегда последний → вне кэша
- Overhead: ~100-200 tokens на `<state>` блок

## Результат тестирования

- "Покажи кроссовки Найк" → `catalog_search` (корректно, первый запрос)
- "Покажи крупнее с рейтингом" → `NONE (stop=end_turn)` — Agent1 не вызвал tool
- "Покажи списком" → `NONE (stop=end_turn)` — Agent1 не вызвал tool
- "А теперь покажи Адидас" → `catalog_search` (корректно, новые данные)

Agent1 LLM call на style-запросах: ~1.3 секунды (только think, без tool). Ранее: ~5-6 секунд (think + catalog_search).
