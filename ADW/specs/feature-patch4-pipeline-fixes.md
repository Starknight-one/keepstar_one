# Feature: patch4 — Pipeline Critical Fixes

## Feature Description
Пакетное исправление 5 подтверждённых багов в двухагентном пайплайне: критичный баг поиска продуктов, отсутствие логирования Agent2, отсутствие conversation history у Agent2, дубль запросов из-за StrictMode, и обнаружение источника кнопки "показать больше".

## Objective
Устранить все 5 проблем, мешающих корректной работе и дебагу пайплайна. После фикса:
- Поиск по бренду + модели будет возвращать релевантные результаты (а не все 7 Nike)
- Agent2 будет полностью логироваться (дебаг станет возможен)
- Agent2 будет учитывать историю разговора (качество ответов вырастет)
- Дублирования запросов в dev-режиме не будет
- Кнопка "Show details" будет работать корректно (баг 5 — не баг, а фича)

## Expertise Context
Expertise used:
- **backend-pipeline**: tool_search_products.go — ProductFilter логика, tool_freestyle.go — атомы без кнопок
- **backend-usecases**: agent1_execute.go vs agent2_execute.go — паттерны логирования и ConversationHistory
- **backend-ports**: ProductFilter struct — Brand и Search поля, AND-комбинация в postgres
- **backend-adapters**: postgres_catalog.go — SQL-запрос с AND между Brand и Search условиями
- **frontend-features**: useChatSubmit.js, ChatInput.jsx, ChatPanel.jsx — submit flow
- **frontend-entities**: ProductCardTemplate.jsx — "Show details" кнопка в secondary slot

## Relevant Files

### Existing Files (модифицируются)
- `project/backend/internal/tools/tool_search_products.go` — Fix #1: убрать игнорирование query при brand
- `project/backend/internal/usecases/agent2_execute.go` — Fix #2 + #3: добавить логирование тулов + conversation history
- `project/frontend/src/features/chat/useChatSubmit.js` — Fix #4: защита от дублей
- `project/backend/internal/prompts/prompt_compose_widgets.go` — Fix #3 (опционально): расширить промпт контекстом

### Файлы только для чтения (не модифицируются)
- `project/backend/internal/usecases/agent1_execute.go` — паттерн логирования для копирования
- `project/backend/internal/logger/logger.go` — метод ToolExecuted уже существует
- `project/backend/internal/ports/catalog_port.go` — ProductFilter struct (Brand + Search = AND)
- `project/backend/internal/adapters/postgres/postgres_catalog.go` — SQL AND-комбинация (строки 203-207)
- `project/frontend/src/entities/widget/templates/ProductCardTemplate.jsx` — "Show details" кнопка (строки 92-115)
- `project/frontend/src/main.jsx` — StrictMode включён

### Новые файлы
Не требуются.

## Step by Step Tasks
IMPORTANT: Execute strictly in order.

### 1. Fix #1 — Search: query + brand комбинация (CRITICAL)

**Файл:** `project/backend/internal/tools/tool_search_products.go`

**Проблема:** Строки 84-95 — если `brand != ""`, то `filter.Search = query` не устанавливается. Query полностью игнорируется.

**Причина текущего кода:** Комментарий гласит "avoid AND conflict" — если query = "Nike Pegasus" и brand = "Nike", то query содержит бренд и создаёт двойную фильтрацию.

**Решение:** Вместо полного игнорирования query, вырезать brand из query перед установкой Search. Это сохраняет специфику (e.g. "Pegasus") при наличии brand фильтра.

**Конкретные изменения:**

**1a. Добавить import `"strings"`** — его НЕТ в текущих imports (строки 3-9: только "context", "fmt", domain, ports). Добавить:
```go
import (
	"context"
	"fmt"
	"strings"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)
```

**1b. Строки 83-95 заменить на:**
```go
// Build filter
filter := ports.ProductFilter{
    Brand:    brand,
    MinPrice: minPrice,
    MaxPrice: maxPrice,
    Limit:    limit,
}

// Use query as search, stripping brand name to avoid double filtering
if query != "" {
    searchQuery := query
    if brand != "" {
        // Remove brand from query to avoid AND conflict with Brand filter
        // e.g. query="Nike Pegasus" brand="Nike" → search="Pegasus"
        searchQuery = strings.TrimSpace(removeSubstringIgnoreCase(query, brand))
    }
    if searchQuery != "" {
        filter.Search = searchQuery
    }
}
```

**1c. Добавить helper-функцию в конец файла (после extractProductFields):**
```go
// removeSubstringIgnoreCase removes first occurrence of substr from s, case-insensitive.
// Preserves original casing of remaining text.
func removeSubstringIgnoreCase(s, substr string) string {
    lower := strings.ToLower(s)
    idx := strings.Index(lower, strings.ToLower(substr))
    if idx < 0 {
        return s
    }
    return s[:idx] + s[idx+len(substr):]
}
```

**Верификация SQL:** В postgres_catalog.go условия Brand и Search уже AND-совместимы (строки 167-207). Brand фильтрует по `mp.brand ILIKE`, Search — по `p.name ILIKE OR mp.name ILIKE OR mp.brand ILIKE`. Комбинация корректна.

---

### 2. Fix #2 — Agent2: добавить логирование тулов (CRITICAL FOR DEBUG)

**Файл:** `project/backend/internal/usecases/agent2_execute.go`

**Проблема:** Строки 145-165 — цикл выполнения тулов Agent2 не содержит ни одной строки лога.

**Паттерн из Agent1 (agent1_execute.go:147-167):**
```go
uc.log.Debug("tool_call_received",
    "tool", toolCall.Name,
    "input", toolCall.Input,
    "session_id", req.SessionID,
)
toolStart := time.Now()
result, err := uc.toolRegistry.Execute(...)
toolDuration := time.Since(toolStart).Milliseconds()
...
uc.log.ToolExecuted(toolCall.Name, req.SessionID, result.Content, toolDuration)
```

**Конкретные изменения в agent2_execute.go, строки 145-165:**
Заменить текущий цикл на:
```go
// Execute tool calls — tools create deltas via zone-write internally
for _, toolCall := range llmResp.ToolCalls {
    response.ToolCalled = true
    response.ToolName = toolCall.Name

    uc.log.Debug("tool_call_received",
        "tool", toolCall.Name,
        "input", toolCall.Input,
        "session_id", req.SessionID,
        "actor", "agent2",
    )

    toolStart := time.Now()
    result, err := uc.toolRegistry.Execute(ctx, tools.ToolContext{
        SessionID: req.SessionID,
        TurnID:    req.TurnID,
        ActorID:   "agent2",
    }, toolCall)
    toolDuration := time.Since(toolStart).Milliseconds()

    if err != nil {
        uc.log.Error("tool_execution_failed", "error", err, "tool", toolCall.Name, "actor", "agent2")
        return nil, fmt.Errorf("execute tool %s: %w", toolCall.Name, err)
    }

    uc.log.ToolExecuted(toolCall.Name, req.SessionID, result.Content, toolDuration)

    response.RawResponse = result.Content

    // Tool writes formation to state
    if result.IsError {
        return nil, fmt.Errorf("tool error: %s", result.Content)
    }
}
```

**Проверки:**
- `uc.log` уже есть в struct (строка 45: `log *logger.Logger`) ✓
- `logger.ToolExecuted()` уже существует в logger.go:67-74 ✓
- `"time"` уже в imports (строка 7) ✓
- `"strings"` уже в imports (строка 6) ✓

---

### 3. Fix #3 — Agent2: добавить conversation history

**Файл:** `project/backend/internal/usecases/agent2_execute.go`

**Проблема:** Строки 100-102 — Agent2 создаёт messages из одного сообщения, без истории.

**Паттерн Agent1 (agent1_execute.go:89-94):**
```go
messages := state.ConversationHistory
messages = append(messages, domain.LLMMessage{
    Role:    "user",
    Content: req.Query,
})
```

**Решение:** Добавить последние N сообщений из ConversationHistory для контекста Agent2. НЕ ВСЮ историю (Agent2 работает с мета-данными, ему не нужна вся цепочка).

**Конкретные изменения в agent2_execute.go, строки 97-102:**
```go
// Build user message with view context, user query, and data delta
userPrompt := prompts.BuildAgent2ToolPrompt(state.Current.Meta, state.View, req.UserQuery, dataDelta)

// Include recent user queries from conversation history for context (last 4 user messages max).
// IMPORTANT: Only take user messages with Content (skip assistant, tool_use, tool_result).
// Agent1 ConversationHistory contains tool_use/tool_result blocks from Agent1's tools,
// which would confuse Agent2 (it has different tools: render_* and freestyle).
var messages []domain.LLMMessage
if len(state.ConversationHistory) > 0 {
    var userMessages []domain.LLMMessage
    for _, msg := range state.ConversationHistory {
        if msg.Role == "user" && msg.Content != "" && msg.ToolResult == nil {
            userMessages = append(userMessages, msg)
        }
    }
    historyLimit := 4
    start := len(userMessages) - historyLimit
    if start < 0 {
        start = 0
    }
    messages = append(messages, userMessages[start:]...)
}
messages = append(messages, domain.LLMMessage{
    Role:    "user",
    Content: userPrompt,
})
```

**Примечание:** Лимит в 4 user-сообщения — баланс между контекстом и расходом токенов (Agent2 использует Haiku). Agent2 увидит последние user-запросы, что даст контекст для restyle-запросов вроде "сделай крупнее" после "покажи Nike".

**Gotcha (CRITICAL):** `state.ConversationHistory` содержит tool_use (assistant) и tool_result (user) сообщения от Agent1. Agent2 имеет ДРУГОЙ набор tools (render_*, freestyle). Передача tool_use/tool_result от Agent1-tools вызовет confusion или ошибку Anthropic API. Поэтому фильтруем ТОЛЬКО user messages с текстовым Content.

**Тип совместимости:** `state.ConversationHistory` = `[]domain.LLMMessage`, `ChatWithToolsCached` принимает `[]domain.LLMMessage` — тип совпадает.

---

### 4. Fix #4 — Frontend: защита от дубля запроса (StrictMode)

**Файл:** `project/frontend/src/features/chat/useChatSubmit.js`

**Проблема:** React StrictMode в dev-режиме может вызывать двойное выполнение эффектов. При этом submit вызывается пользователем напрямую (не из эффекта), поэтому StrictMode сам по себе не дублирует submit. Но при двойном маунте ChatPanel submit-callback пересоздаётся.

**Решение:** Добавить `useRef`-based guard для предотвращения concurrent запросов.

**Конкретные изменения в useChatSubmit.js:**
Добавить ref для блокировки:
```javascript
// После строки 19 (sessionIdRef)
const submittingRef = useRef(false);
```

Добавить guard в начало submit:
```javascript
const submit = useCallback(async (text) => {
    if (!text.trim()) return;
    if (submittingRef.current) return; // Prevent duplicate requests
    submittingRef.current = true;

    // ... existing code ...

    } finally {
      setLoading(false);
      submittingRef.current = false; // Release lock
    }
}, [addMessage, setLoading, setError, setSessionId, onFormationReceived]);
```

**Примечание:** `isLoading` state тоже блокирует кнопку через `ChatInput disabled={isLoading}`, но state update асинхронный и может не успеть до повторного вызова. `useRef` — синхронный и моментальный.

---

### 5. Fix #5 — Кнопка "показать больше": НЕ БАГ, ДОКУМЕНТАЦИЯ

**Результат расследования:**
- **Кнопка найдена:** `ProductCardTemplate.jsx:92-115` и `ServiceCardTemplate.jsx:82-105`
- **Текст:** "Show details" / "Hide details" (не "показать больше")
- **Поведение:** Toggle secondary atoms (description и прочие атомы в slot=secondary)
- **Источник:** Фронтенд шаблоны, НЕ бэкенд/LLM

**Действие:** Это не баг. Кнопка появляется когда freestyle или preset создают атомы с `slot: "secondary"` (description). Это штатное поведение ProductCardTemplate.

**Никаких изменений кода не требуется для этого пункта.**

---

### 6. Test: добавить тест на brand + query комбинацию

**Файл:** `project/backend/internal/tools/tool_search_products_test.go`

**Проблема:** Существующие 4 теста (строки 117-264) передают только query без brand. Новая логика removeSubstringIgnoreCase остаётся непротестированной.

**Добавить тест в конец файла:**
```go
func TestSearchProducts_BrandPlusQuery(t *testing.T) {
	state := &domain.SessionState{
		ID: "s1", SessionID: "sess-1",
		Current: domain.StateCurrent{
			Meta: domain.StateMeta{Aliases: map[string]string{"tenant_slug": "nike"}},
		},
	}
	sp := newMockStatePort(state)

	var capturedFilter ports.ProductFilter
	cp := &mockCatalogPortWithCapture{
		products: []domain.Product{
			{ID: "p1", Name: "Nike Pegasus 41", Price: 15990, Brand: "Nike"},
		},
		total:         1,
		captureFilter: &capturedFilter,
	}
	tool := tools.NewSearchProductsTool(sp, cp)

	ctx := context.Background()
	toolCtx := tools.ToolContext{SessionID: "sess-1", TurnID: "turn-1", ActorID: "agent1"}

	// Test: brand + query → query should have brand stripped
	_, err := tool.Execute(ctx, toolCtx, map[string]interface{}{
		"query": "Nike Pegasus",
		"brand": "Nike",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedFilter.Brand != "Nike" {
		t.Errorf("expected Brand=Nike, got %s", capturedFilter.Brand)
	}
	if capturedFilter.Search != "Pegasus" {
		t.Errorf("expected Search=Pegasus, got %q", capturedFilter.Search)
	}
}

func TestSearchProducts_BrandEqualsQuery(t *testing.T) {
	state := &domain.SessionState{
		ID: "s1", SessionID: "sess-1",
		Current: domain.StateCurrent{
			Meta: domain.StateMeta{Aliases: map[string]string{"tenant_slug": "nike"}},
		},
	}
	sp := newMockStatePort(state)

	var capturedFilter ports.ProductFilter
	cp := &mockCatalogPortWithCapture{
		products: []domain.Product{
			{ID: "p1", Name: "Nike Air Max", Price: 12990, Brand: "Nike"},
		},
		total:         1,
		captureFilter: &capturedFilter,
	}
	tool := tools.NewSearchProductsTool(sp, cp)

	ctx := context.Background()
	toolCtx := tools.ToolContext{SessionID: "sess-1", TurnID: "turn-1", ActorID: "agent1"}

	// Test: brand == query → Search should be empty (no redundant filtering)
	_, err := tool.Execute(ctx, toolCtx, map[string]interface{}{
		"query": "Nike",
		"brand": "Nike",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedFilter.Brand != "Nike" {
		t.Errorf("expected Brand=Nike, got %s", capturedFilter.Brand)
	}
	if capturedFilter.Search != "" {
		t.Errorf("expected Search empty, got %q", capturedFilter.Search)
	}
}
```

**Также добавить mockCatalogPortWithCapture** (рядом с существующим mockCatalogPort):
```go
// mockCatalogPortWithCapture captures the filter passed to ListProducts
type mockCatalogPortWithCapture struct {
	products      []domain.Product
	total         int
	captureFilter *ports.ProductFilter
}

func (m *mockCatalogPortWithCapture) GetTenantBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	return &domain.Tenant{ID: "t1", Slug: slug}, nil
}
func (m *mockCatalogPortWithCapture) GetCategories(ctx context.Context) ([]domain.Category, error) {
	return nil, nil
}
func (m *mockCatalogPortWithCapture) GetMasterProduct(ctx context.Context, id string) (*domain.MasterProduct, error) {
	return nil, nil
}
func (m *mockCatalogPortWithCapture) ListProducts(ctx context.Context, tenantID string, filter ports.ProductFilter) ([]domain.Product, int, error) {
	if m.captureFilter != nil {
		*m.captureFilter = filter
	}
	return m.products, m.total, nil
}
func (m *mockCatalogPortWithCapture) GetProduct(ctx context.Context, tenantID string, productID string) (*domain.Product, error) {
	return nil, nil
}
```

---

### 7. Validation

Выполнить все валидационные команды:
```bash
cd project/backend && go build ./...
cd project/backend && go test ./...
cd project/frontend && npm run build
cd project/frontend && npm run lint
```

## Validation Commands
```bash
# Backend build (required)
cd project/backend && go build ./...

# Backend tests
cd project/backend && go test ./...

# Frontend build (required)
cd project/frontend && npm run build

# Frontend lint
cd project/frontend && npm run lint
```

## Acceptance Criteria
- [ ] `tool_search_products.go`: query "Nike Pegasus" + brand "Nike" → filter.Search = "Pegasus", filter.Brand = "Nike"
- [ ] `tool_search_products.go`: query "кроссовки" + brand "" → filter.Search = "кроссовки" (поведение не изменилось)
- [ ] `tool_search_products.go`: query "Nike" + brand "Nike" → filter.Search = "" (бренд вырезан, нет дубликата)
- [ ] `agent2_execute.go`: tool_call_received лог появляется перед вызовом тула Agent2
- [ ] `agent2_execute.go`: tool_executed лог появляется после вызова тула Agent2 с timing
- [ ] `agent2_execute.go`: tool_execution_failed лог появляется при ошибке тула
- [ ] `agent2_execute.go`: messages содержат последние 4 user-only сообщения из ConversationHistory + текущий prompt
- [ ] `agent2_execute.go`: tool_use/tool_result сообщения из ConversationHistory НЕ попадают в Agent2 messages
- [ ] `tool_search_products_test.go`: TestSearchProducts_BrandPlusQuery проходит (Search="Pegasus")
- [ ] `tool_search_products_test.go`: TestSearchProducts_BrandEqualsQuery проходит (Search="")
- [ ] `useChatSubmit.js`: двойной быстрый клик не вызывает 2 запроса
- [ ] `go build ./...` — проходит без ошибок
- [ ] `go test ./...` — проходит без ошибок
- [ ] `npm run build` — проходит без ошибок
- [ ] `npm run lint` — проходит без ошибок

## Notes

### Gotcha: Search AND-конфликт
Текущий комментарий в коде ("avoid AND conflict") имел смысл — если query="Nike" и brand="Nike", то AND фильтрация давала бы `(name ILIKE '%Nike%' OR brand ILIKE '%Nike%') AND brand ILIKE '%Nike%'`, что избыточно но не ломает результат. Настоящая проблема в том что query="Nike Pegasus" полностью теряла "Pegasus". Fix #1 решает это аккуратно.

### Gotcha: Agent2 ConversationHistory и prompt caching
Agent2 использует `ChatWithToolsCached` с `CacheConversation: false` (строка ~130). Добавление ConversationHistory может повлиять на cache hit rate. Лимит в 4 сообщения минимизирует этот эффект. Если нужно — можно включить `CacheConversation: true` для Agent2 тоже.

### Gotcha: Fix #4 — только dev-режим
Дубль запросов из StrictMode происходит ТОЛЬКО в dev-режиме. В production StrictMode отключён. Тем не менее, `submittingRef` guard — хорошая практика и защищает от edge cases (double-click, rapid submit).

### Gotcha: Fix #5 — не баг
Кнопка "Show details" — это **штатная фича** ProductCardTemplate/ServiceCardTemplate. Она показывает/скрывает secondary-слот атомов. Если нужно локализовать текст на русский — это отдельная задача (i18n).
