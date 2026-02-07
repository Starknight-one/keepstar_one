# Feature: ADW-r4w8n3k Activate Prompt Caching

## Feature Description

Финальная доводка Anthropic prompt caching для достижения `cache_read_input_tokens > 0`. Фаза 1 (ADW-k7x9m2p) добавила инфраструктуру: `ChatWithToolsCached`, `CacheConfig`, cache types, padding tools. Но кэш не активируется из-за трёх проблем: ConversationHistory не персистится в postgres, input tokens ниже порога 4096, и Go рандомизирует порядок ключей в map — что ломает cache matching.

**Важное открытие**: Beta header `anthropic-beta: prompt-caching-2024-07-31` НЕ нужен. Prompt caching стал GA в декабре 2024. Достаточно `cache_control` в запросе.

## Objective

1. **ConversationHistory в postgres** — персистенция истории между запросами
2. **Padding tools до 4096+** — гарантированное превышение минимального порога для Haiku
3. **Стабильный порядок ключей** — Go map ordering ломает cache, нужен детерминированный JSON
4. **Валидация** — `cache_read_input_tokens > 0` при повторных запросах

## Expertise Context

Expertise used:
- **backend-adapters**: Anthropic client `ChatWithToolsCached`, postgres `StateAdapter` с JSONB колонками. ConversationHistory не персистится (только in-memory через `UpdateState` blob)
- **backend-ports**: `StatePort.UpdateState(ctx, state)` — blob update. `AddDelta` auto-increment step
- **backend-domain**: `SessionState.ConversationHistory []LLMMessage` — поле есть, но не в postgres schema
- **backend-pipeline**: `mock_tools.go` — 8 padding tools (~3200 tokens). С реальными tools ~4000, нужно 4096+

## Relevant Files

### Existing Files (modify)
- `project/backend/internal/adapters/postgres/state_migrations.go` — добавить миграцию для `conversation_history` колонки
- `project/backend/internal/adapters/postgres/postgres_state.go` — CreateState, GetState, UpdateState: чтение/запись conversation_history
- `project/backend/internal/adapters/anthropic/anthropic_client.go` — убрать TODO про beta header (не нужен), исправить стабильность JSON ключей
- `project/backend/internal/tools/mock_tools.go` — расширить padding tools до гарантированных 4096+ tokens
- `project/backend/internal/usecases/cache_test.go` — обновить assertions: проверить cache_read > 0

### New Files
- Нет

## Step by Step Tasks

### 1. ConversationHistory колонка в postgres

Добавить миграцию в `state_migrations.go`:

```sql
ALTER TABLE chat_session_state
    ADD COLUMN IF NOT EXISTS conversation_history JSONB DEFAULT '[]';
```

Добавить в `RunStateMigrations` как новый migration string.

Файл: `project/backend/internal/adapters/postgres/state_migrations.go`

### 2. Персистенция ConversationHistory в StateAdapter

**CreateState**: инициализировать `conversation_history` как `'[]'`.

**GetState**: добавить `conversation_history` в SELECT, добавить переменную `conversationHistoryJSON []byte`, unmarshal в `state.ConversationHistory`.

**UpdateState**: добавить `conversation_history` в UPDATE SET, marshal `state.ConversationHistory`.

Файл: `project/backend/internal/adapters/postgres/postgres_state.go`

Паттерн (по аналогии с `view_stack`):
```go
// В GetState SELECT:
SELECT id, session_id, current_data, current_meta, current_template,
       view_mode, view_focused, view_stack, conversation_history, step, created_at, updated_at

// Добавить переменную:
var conversationHistoryJSON []byte

// После scan:
if len(conversationHistoryJSON) > 0 {
    json.Unmarshal(conversationHistoryJSON, &state.ConversationHistory)
}

// В UpdateState:
conversationHistoryJSON, _ := json.Marshal(state.ConversationHistory)
// Добавить в UPDATE SET и параметры
```

### 3. Стабильный порядок ключей в tool definitions

**Проблема**: Anthropic docs: _"Verify that the keys in your tool_use content blocks have stable ordering as some languages (e.g. Swift, Go) randomize key order during JSON conversion, breaking caches"_.

`ToolDefinition.InputSchema` — это `map[string]interface{}`. Go рандомизирует порядок ключей при `json.Marshal`. Каждый запрос может генерировать разный JSON для одних и тех же tools → cache miss.

**Решение**: В `ChatWithToolsCached` перед отправкой — сериализовать `InputSchema` через sorted keys. Использовать `json.Marshal` + `json.Unmarshal` в `json.RawMessage` не поможет, map всё равно рандомный.

Варианты:
1. Заменить `InputSchema map[string]interface{}` на `InputSchema json.RawMessage` в domain — слишком инвазивно.
2. **Рекомендуемый**: В `anthropic_client.go` добавить helper `stableJSONMarshal` который сортирует ключи рекурсивно. Использовать его при marshal tool definitions в `ChatWithToolsCached`.

```go
// stableJSONMarshal serializes a value with sorted map keys for cache stability
func stableJSONMarshal(v interface{}) ([]byte, error) {
    // json.Marshal in Go already sorts map keys by default since Go 1.12
    // But nested maps inside interface{} may not be sorted if they're
    // different types. Let's verify this is sufficient.
    return json.Marshal(v)
}
```

**Важно**: На самом деле `encoding/json` в Go **уже сортирует ключи map** при Marshal (начиная с Go 1.12). Проблема может быть в другом — если `InputSchema` заполняется из `map[string]interface{}` literal в Go, порядок при итерации будет рандомный, но при `json.Marshal` ключи будут отсортированы. Нужно проверить, что мы не итерируем map для построения request body и не полагаемся на порядок.

**Действие**: Проверить что `json.Marshal` используется для tool definitions (а не ручная конкатенация). Если да — Go автоматически сортирует ключи и проблемы нет. Добавить комментарий-предупреждение.

Файл: `project/backend/internal/adapters/anthropic/anthropic_client.go`

### 4. Расширить padding tools до 4096+

Текущее состояние: 8 padding tools × ~400 tokens = ~3200 tokens. С реальными tools (~800) = ~4000. Порог для Haiku — 4096.

**Действие**: Добавить 2 дополнительных padding tool или расширить описания существующих, чтобы гарантированно превысить 4096. Целевой padding: ~3600 tokens (+ ~800 real = ~4400, с запасом).

Обновить `EstimatedPaddingTokens` const.

Файл: `project/backend/internal/tools/mock_tools.go`

### 5. Удалить неактуальный TODO про beta header

Prompt caching стал GA в декабре 2024. Beta header `anthropic-beta: prompt-caching-2024-07-31` НЕ нужен.

**Действие**: Убрать TODO комментарий из кода если есть. Убрать шаг 20 из оригинальной спеки как неактуальный.

Файлы: `project/backend/internal/adapters/anthropic/anthropic_client.go`, при необходимости спека `ADW-k7x9m2p`

### 6. Обновить cache_test.go

Обновить assertions в `TestPromptCaching_Chain`:
- После fix шагов 1-4, `cache_read_input_tokens > 0` должен быть на запросах 2+
- Сделать `t.Error` вместо `t.Logf("WARNING")` для `cacheHits == 0`
- Добавить assertion что `len(finalState.ConversationHistory) > 0` (уже есть, проверить что работает с persistence)

Файл: `project/backend/internal/usecases/cache_test.go`

### 7. Validation

```bash
cd project/backend && go build ./...
cd project/backend && go test -v -run TestPromptCaching_Chain -timeout 300s ./internal/usecases/
```

Ожидаемый результат:
- Request 1: `cache_creation_input_tokens > 0`, `cache_read_input_tokens = 0`
- Request 2+: `cache_read_input_tokens > 0` (cache hit на tools + system)
- `conversation_history` в postgres сохраняется между запросами
- Финальный `len(state.ConversationHistory) > 0`

## Validation Commands

```bash
cd project/backend && go build ./...
cd project/backend && go test -v -run TestPromptCaching_Chain -timeout 300s ./internal/usecases/
```

## Acceptance Criteria

- [ ] `conversation_history JSONB` колонка в `chat_session_state`
- [ ] `GetState` возвращает `ConversationHistory` из postgres
- [ ] `UpdateState` сохраняет `ConversationHistory` в postgres
- [ ] Padding tools дают суммарно 4096+ input tokens
- [ ] `cache_creation_input_tokens > 0` на первом запросе
- [ ] `cache_read_input_tokens > 0` на повторных запросах
- [ ] `TestPromptCaching_Chain` проходит с cache hits
- [ ] Go map key ordering не ломает cache (verified)

## Notes

- **Beta header НЕ нужен**: Prompt caching стал GA в декабре 2024. Источник: [Anthropic API Release Notes](https://docs.anthropic.com/en/release-notes/api), [GitHub Issue #175](https://github.com/anthropics/claude-cookbooks/issues/175)
- **Go map key sorting**: `encoding/json.Marshal` в Go ≥1.12 автоматически сортирует ключи map. Но нужно убедиться что мы не конструируем JSON вручную.
- **Минимальный порог Haiku 4.5**: 4096 tokens. Текущий total ~4000 — на грани. Нужен запас.
- **Cache TTL**: 5 минут по умолчанию. Тест отправляет запросы с паузой 500ms — достаточно.
- **Порядок кэширования**: tools → system → messages. Изменение tools invalidирует весь кэш.
