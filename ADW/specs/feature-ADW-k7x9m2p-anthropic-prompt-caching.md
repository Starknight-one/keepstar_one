# Feature: ADW-k7x9m2p Anthropic Prompt Caching

## Feature Description

Внедрение Anthropic Prompt Caching для оптимизации стоимости и latency в двухагентном пайплайне. Кэширование позволит переиспользовать system prompts, tool definitions и историю разговора между запросами.

## Objective

1. **Экономия токенов** — кэшированные токены стоят 10% от базовой цены (вместо 100%)
2. **Поддержка истории разговора** — LLM понимает контекст предыдущих сообщений
3. **Снижение latency** — кэшированный контент не перечитывается заново
4. **Метрики** — отслеживание cache hit rate для оценки экономики

## Expertise Context

Expertise used:
- **backend-adapters**: Anthropic client реализован в `anthropic_client.go`, использует API версию `2023-06-01`. Текущие методы: `Chat`, `ChatWithUsage`, `ChatWithTools`. Ценообразование уже отслеживается в `LLMUsage`.
- **backend-ports**: `LLMPort` interface определяет контракт. `ChatWithTools` принимает `systemPrompt`, `messages`, `tools`.
- **backend-pipeline**: Agent1 использует `Agent1SystemPrompt` (~300 токенов), Agent2 использует `Agent2ToolSystemPrompt` (~400 токенов). Tools добавляют ~200-500 токенов.

## How Anthropic Prompt Caching Works

### Структура кэширования
```
┌─────────────────────────────────────────┐
│ tools[]  (определения инструментов)     │ ← кэшируется (cache_control)
├─────────────────────────────────────────┤
│ system   (system prompt)                │ ← кэшируется (cache_control)
├─────────────────────────────────────────┤
│ messages (история разговора)            │ ← кэшируется последний блок
├─────────────────────────────────────────┤
│ Новое сообщение пользователя            │ ← НЕ кэшируется
└─────────────────────────────────────────┘
```

### Pricing (Claude Haiku 4.5)
| Тип | Цена за MTok |
|-----|--------------|
| Base input | $1.00 |
| Cache write (5 min) | $1.25 (×1.25) |
| Cache write (1 hour) | $2.00 (×2.0) |
| Cache read | $0.10 (×0.1) |
| Output | $5.00 |

### Минимальный порог
- **Claude Haiku 4.5**: 4096 токенов минимум для кэширования
- **Claude Sonnet 4.5**: 1024 токена минимум
- **Claude Opus 4.5**: 4096 токенов минимум

### TTL
- **5 минут** — стандартный, обновляется при каждом использовании
- **1 час** — extended, стоит дороже (×2 вместо ×1.25)

## Relevant Files

### Existing Files
- `project/backend/internal/adapters/anthropic/anthropic_client.go` - текущая реализация Anthropic client
- `project/backend/internal/ports/llm_port.go` - LLMPort interface
- `project/backend/internal/domain/tool_entity.go` - LLMMessage, LLMUsage, ToolDefinition
- `project/backend/internal/usecases/agent1_execute.go` - Agent 1 вызывает ChatWithTools
- `project/backend/internal/usecases/agent2_execute.go` - Agent 2 вызывает ChatWithTools
- `project/backend/internal/prompts/prompt_analyze_query.go` - Agent1SystemPrompt
- `project/backend/internal/prompts/prompt_compose_widgets.go` - Agent2ToolSystemPrompt

### New Files
- `project/backend/internal/domain/cache_entity.go` - CacheUsage, CacheMetrics structs
- `project/backend/internal/adapters/anthropic/cache_types.go` - Anthropic cache-specific types
- `project/backend/internal/tools/mock_tools.go` - Padding tools для достижения порога кэширования

## Step by Step Tasks

### 0. Add cache padding tools (достижение порога 4096 токенов)

Создать mock tools для заполнения контекста до минимального порога кэширования Haiku (4096 токенов).

**Файл**: `project/backend/internal/tools/mock_tools.go`

```go
package tools

import "keepstar/internal/domain"

// CachePaddingEnabled controls whether padding tools are added
// TODO: Remove when real tools exceed 4096 tokens
var CachePaddingEnabled = true

// GetCachePaddingTools returns dummy tools to reach cache threshold
// These tools have detailed descriptions but will never be called
// because their names start with _internal_ prefix
func GetCachePaddingTools() []domain.ToolDefinition {
    if !CachePaddingEnabled {
        return nil
    }

    return []domain.ToolDefinition{
        {
            Name: "_internal_inventory_analytics",
            Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool generates comprehensive inventory analytics reports for administrative dashboards.
It processes warehouse data, calculates stock levels, predicts reorder points, and generates
trend analysis for inventory management. Supports multiple warehouse locations, SKU tracking,
and seasonal demand forecasting. Output includes JSON reports with detailed metrics.`,
            InputSchema: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "warehouse_id": map[string]interface{}{"type": "string", "description": "Internal warehouse identifier"},
                    "date_range": map[string]interface{}{"type": "string", "description": "Analysis period"},
                    "include_forecast": map[string]interface{}{"type": "boolean", "description": "Include demand forecast"},
                },
            },
        },
        {
            Name: "_internal_supplier_integration",
            Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool manages supplier integration workflows for the procurement system.
It handles purchase order generation, supplier communication protocols, delivery tracking,
and invoice reconciliation. Supports EDI formats, API integrations with major suppliers,
and automated reordering based on inventory thresholds. Includes supplier performance scoring.`,
            InputSchema: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "supplier_id": map[string]interface{}{"type": "string", "description": "Supplier identifier"},
                    "action": map[string]interface{}{"type": "string", "description": "Integration action type"},
                },
            },
        },
        {
            Name: "_internal_pricing_engine",
            Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool operates the dynamic pricing engine for competitive price optimization.
It analyzes competitor prices, demand elasticity, inventory levels, and margin targets
to suggest optimal pricing strategies. Supports A/B testing of price points, promotional
pricing rules, and geographic price differentiation. Includes machine learning models
for price sensitivity analysis and revenue optimization algorithms.`,
            InputSchema: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "product_ids": map[string]interface{}{"type": "array", "description": "Products to analyze"},
                    "strategy": map[string]interface{}{"type": "string", "description": "Pricing strategy"},
                },
            },
        },
        {
            Name: "_internal_customer_segmentation",
            Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool performs advanced customer segmentation for marketing analytics.
It clusters customers based on purchase history, browsing behavior, demographics,
and engagement metrics. Supports RFM analysis, cohort analysis, lifetime value prediction,
and churn risk scoring. Output includes segment definitions, customer assignments,
and recommended marketing strategies for each segment.`,
            InputSchema: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "segment_type": map[string]interface{}{"type": "string", "description": "Segmentation method"},
                    "include_recommendations": map[string]interface{}{"type": "boolean", "description": "Include marketing recs"},
                },
            },
        },
        {
            Name: "_internal_fraud_detection",
            Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool runs fraud detection algorithms on transaction data.
It analyzes payment patterns, device fingerprints, geographic anomalies,
and behavioral signals to identify potentially fraudulent orders. Supports
real-time scoring, rule-based detection, and machine learning models.
Includes integration with external fraud databases and chargeback prediction.`,
            InputSchema: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "transaction_id": map[string]interface{}{"type": "string", "description": "Transaction to analyze"},
                    "check_type": map[string]interface{}{"type": "string", "description": "Type of fraud check"},
                },
            },
        },
        {
            Name: "_internal_shipping_optimizer",
            Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool optimizes shipping routes and carrier selection for order fulfillment.
It calculates optimal shipping methods based on package dimensions, weight, destination,
delivery speed requirements, and cost constraints. Supports multi-carrier rate shopping,
zone skipping strategies, and consolidation opportunities. Includes carbon footprint
calculation and sustainable shipping options.`,
            InputSchema: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "order_ids": map[string]interface{}{"type": "array", "description": "Orders to optimize"},
                    "optimize_for": map[string]interface{}{"type": "string", "description": "cost/speed/carbon"},
                },
            },
        },
        {
            Name: "_internal_content_moderation",
            Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool performs content moderation on user-generated content including reviews,
questions, and uploaded images. It detects inappropriate content, spam, fake reviews,
and policy violations using NLP and computer vision models. Supports multiple languages,
sentiment analysis, and automated flagging workflows. Includes appeal handling and
moderation queue management.`,
            InputSchema: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "content_id": map[string]interface{}{"type": "string", "description": "Content to moderate"},
                    "content_type": map[string]interface{}{"type": "string", "description": "review/question/image"},
                },
            },
        },
        {
            Name: "_internal_recommendation_engine",
            Description: `INTERNAL SYSTEM TOOL - DO NOT USE.
This tool generates personalized product recommendations using collaborative filtering
and content-based algorithms. It analyzes user behavior, purchase history, product attributes,
and real-time context to suggest relevant products. Supports multiple recommendation types:
similar products, frequently bought together, personalized picks, and trending items.
Includes A/B testing framework and performance analytics.`,
            InputSchema: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "user_id": map[string]interface{}{"type": "string", "description": "User for recommendations"},
                    "context": map[string]interface{}{"type": "string", "description": "Recommendation context"},
                    "limit": map[string]interface{}{"type": "integer", "description": "Number of recommendations"},
                },
            },
        },
    }
}

// EstimatedPaddingTokens is approximate token count for padding tools
// 8 tools × ~400 tokens each ≈ 3200 tokens
// Combined with real tools (~800) = ~4000 tokens, close to 4096 threshold
const EstimatedPaddingTokens = 3200
```

**Интеграция в Registry** (`project/backend/internal/tools/tool_registry.go`):

```go
// GetDefinitions returns all tool definitions including padding if enabled
func (r *Registry) GetDefinitions() []domain.ToolDefinition {
    defs := make([]domain.ToolDefinition, 0)

    // Add real tools
    for _, executor := range r.executors {
        defs = append(defs, executor.Definition())
    }

    // Add padding tools for cache threshold (temporary)
    defs = append(defs, GetCachePaddingTools()...)

    return defs
}
```

**Почему это работает**:
- Tools с префиксом `_internal_` и описанием "DO NOT USE" не будут вызваны LLM
- ~3200 токенов padding + ~800 токенов real = ~4000 токенов
- Близко к порогу 4096, история добавит недостающее
- Легко отключить через `CachePaddingEnabled = false`

### 1. Extend domain types for cache metrics

Добавить поля для отслеживания кэша в `LLMUsage`:

```go
// In domain/tool_entity.go (extend LLMUsage)
type LLMUsage struct {
    InputTokens              int     `json:"input_tokens"`
    OutputTokens             int     `json:"output_tokens"`
    TotalTokens              int     `json:"total_tokens"`
    Model                    string  `json:"model"`
    CostUSD                  float64 `json:"cost_usd"`
    // NEW: Cache metrics
    CacheCreationInputTokens int     `json:"cache_creation_input_tokens,omitempty"`
    CacheReadInputTokens     int     `json:"cache_read_input_tokens,omitempty"`
}
```

Обновить `CalculateCost()` для учёта кэш-токенов.

### 2. Update Anthropic API types

В `anthropic_client.go` добавить поддержку `cache_control`:

```go
// Content block with cache control
type contentBlockWithCache struct {
    Type         string                 `json:"type"`
    Text         string                 `json:"text,omitempty"`
    CacheControl *cacheControl          `json:"cache_control,omitempty"`
    // ... other fields
}

type cacheControl struct {
    Type string `json:"type"` // "ephemeral"
    TTL  string `json:"ttl,omitempty"` // "5m" or "1h"
}

// Tool with cache control
type anthropicToolWithCache struct {
    Name         string                 `json:"name"`
    Description  string                 `json:"description"`
    InputSchema  map[string]interface{} `json:"input_schema"`
    CacheControl *cacheControl          `json:"cache_control,omitempty"`
}
```

### 3. Implement cache-aware request building

Создать новый метод `ChatWithToolsCached`:

```go
// ChatWithToolsCached sends messages with prompt caching enabled
func (c *Client) ChatWithToolsCached(
    ctx context.Context,
    systemPrompt string,
    messages []domain.LLMMessage,
    tools []domain.ToolDefinition,
    cacheConfig *CacheConfig,
) (*domain.LLMResponse, error)
```

Где `CacheConfig`:
```go
type CacheConfig struct {
    CacheTools  bool   // cache tool definitions
    CacheSystem bool   // cache system prompt
    CacheConversation bool // cache conversation history
    TTL         string // "5m" or "1h"
}
```

### 4. Modify request structure for caching

System prompt должен быть массивом для cache_control:

```go
type anthropicCachedRequest struct {
    Model     string                    `json:"model"`
    MaxTokens int                       `json:"max_tokens"`
    System    []contentBlockWithCache   `json:"system"` // Array for cache control
    Messages  []anthropicToolMsg        `json:"messages"`
    Tools     []anthropicToolWithCache  `json:"tools,omitempty"`
}
```

### 5. Parse cache metrics from response

Обновить парсинг response для извлечения кэш-метрик:

```go
type anthropicUsageResponse struct {
    InputTokens              int `json:"input_tokens"`
    OutputTokens             int `json:"output_tokens"`
    CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
    CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}
```

### 6. Update LLMPort interface

Добавить новый метод в порт:

```go
// In ports/llm_port.go
type LLMPort interface {
    Chat(ctx context.Context, message string) (string, error)
    ChatWithTools(...) (*domain.LLMResponse, error)
    ChatWithUsage(...) (*ChatResponse, error)
    // NEW
    ChatWithToolsCached(
        ctx context.Context,
        systemPrompt string,
        messages []domain.LLMMessage,
        tools []domain.ToolDefinition,
        cacheConfig *CacheConfig,
    ) (*domain.LLMResponse, error)
}

type CacheConfig struct {
    CacheTools        bool
    CacheSystem       bool
    CacheConversation bool
    TTL               string // "5m" or "1h"
}
```

### 7. Store conversation history in state

Расширить `SessionState` для хранения истории:

```go
// In domain/state_entity.go
type SessionState struct {
    // ... existing fields
    ConversationHistory []domain.LLMMessage `json:"conversation_history,omitempty"`
}
```

### 8. Update Agent1 to use caching

В `agent1_execute.go`:

```go
// Build messages with conversation history
messages := state.ConversationHistory
messages = append(messages, domain.LLMMessage{
    Role:    "user",
    Content: req.Query,
})

// Call LLM with caching
llmResp, err := uc.llm.ChatWithToolsCached(
    ctx,
    prompts.Agent1SystemPrompt,
    messages,
    toolDefs,
    &ports.CacheConfig{
        CacheTools:  true,
        CacheSystem: true,
        CacheConversation: len(messages) > 1, // cache if history exists
        TTL: "5m",
    },
)

// Update conversation history in state
state.ConversationHistory = append(state.ConversationHistory,
    domain.LLMMessage{Role: "user", Content: req.Query},
)
if len(llmResp.ToolCalls) > 0 {
    state.ConversationHistory = append(state.ConversationHistory,
        domain.LLMMessage{Role: "assistant", ToolCalls: llmResp.ToolCalls},
    )
}
```

### 9. Update Agent2 similarly

В `agent2_execute.go` применить аналогичные изменения.

### 10. Update cost calculation

```go
func (u *LLMUsage) CalculateCost() float64 {
    pricing, ok := LLMPricing[u.Model]
    if !ok {
        pricing = LLMPricing["claude-haiku-4-5-20251001"]
    }

    // Regular input (after cache breakpoint)
    inputCost := float64(u.InputTokens) * pricing.InputPerMillion / 1_000_000

    // Cache write (1.25x for 5min TTL)
    cacheWriteCost := float64(u.CacheCreationInputTokens) * pricing.InputPerMillion * 1.25 / 1_000_000

    // Cache read (0.1x)
    cacheReadCost := float64(u.CacheReadInputTokens) * pricing.InputPerMillion * 0.1 / 1_000_000

    // Output
    outputCost := float64(u.OutputTokens) * pricing.OutputPerMillion / 1_000_000

    return inputCost + cacheWriteCost + cacheReadCost + outputCost
}
```

### 11. Add logging for cache metrics

В `logger/logger.go` добавить:

```go
func (l *Logger) LLMUsageWithCache(
    agent string,
    model string,
    inputTokens, outputTokens int,
    cacheCreated, cacheRead int,
    costUSD float64,
    durationMs int64,
) {
    l.Info("llm_usage",
        "agent", agent,
        "model", model,
        "input_tokens", inputTokens,
        "output_tokens", outputTokens,
        "cache_created", cacheCreated,
        "cache_read", cacheRead,
        "cache_hit_rate", calculateHitRate(cacheRead, inputTokens+cacheCreated+cacheRead),
        "cost_usd", costUSD,
        "duration_ms", durationMs,
    )
}
```

### 12. Validation

- Run `go build ./...` in project/backend
- Run `go test ./...` in project/backend
- Manual test:
  1. Send query "покажи кроссовки", check logs for `cache_creation_input_tokens > 4000`
  2. Send same query within 5 minutes, check `cache_read_input_tokens > 4000`
  3. Verify mock tools (`_internal_*`) are NOT in tool call logs

## Validation Commands

```bash
cd project/backend && go build ./...
cd project/backend && go test ./...
```

## Acceptance Criteria

- [ ] Cache padding tools добавлены (~3200 токенов)
- [ ] Mock tools НЕ вызываются при обычных запросах (проверить в логах)
- [ ] `LLMUsage` содержит `cache_creation_input_tokens` и `cache_read_input_tokens`
- [ ] `ChatWithToolsCached` метод реализован в Anthropic adapter
- [ ] System prompt передаётся как массив с `cache_control`
- [ ] Tool definitions имеют `cache_control` на последнем элементе
- [ ] Conversation history кэшируется при последующих запросах
- [ ] Cost calculation учитывает кэш-токены (write ×1.25, read ×0.1)
- [ ] Логи показывают cache hit rate
- [ ] При повторном запросе в течение 5 минут `cache_read_input_tokens > 0`
- [ ] `CachePaddingEnabled` флаг позволяет отключить mock tools

## Notes

### Минимальный порог токенов
Для Haiku 4.5 минимум 4096 токенов для кэширования. Agent1SystemPrompt (~300 токенов) + real tools (~500 токенов) = ~800 токенов — **недостаточно для кэширования**.

**Решение**: Cache padding tools (шаг 0)
- 8 mock tools с детальными описаниями добавляют ~3200 токенов
- Итого: ~800 (real) + ~3200 (padding) = ~4000 токенов
- С первым сообщением (~100 токенов) достигаем порога 4096
- Mock tools имеют префикс `_internal_` и описание "DO NOT USE" — LLM их не вызовет
- Когда реальных tools станет достаточно — отключаем padding через `CachePaddingEnabled = false`

### Порядок cache breakpoints
Важно: `tools` → `system` → `messages`. Cache control должен быть на последнем элементе каждой группы.

### TTL выбор
Рекомендация: начать с 5 минут (стандартный, дешевле). Если пользователь отвечает реже чем раз в 5 минут — рассмотреть 1 час (дороже на write, но сохранит кэш).

### Backward compatibility
`ChatWithTools` остаётся без изменений. `ChatWithToolsCached` — новый метод. Миграция постепенная.

### Экономика (пример расчёта)

**Без кэша** (10 сообщений в сессии, Haiku):
- Каждое сообщение: ~1000 input tokens × $1/MTok = $0.001
- 10 сообщений: 1000 + 2000 + 3000 + ... + 10000 = 55000 tokens = $0.055

**С кэшем** (те же 10 сообщений):
- Первое: 1000 cache write × $1.25/MTok = $0.00125
- Остальные 9: 9000 cache read × $0.10/MTok = $0.0009
- Новые токены: 9 × 100 × $1/MTok = $0.0009
- Итого: $0.00125 + $0.0009 + $0.0009 = $0.00305

**Экономия: ~94%** для длинных сессий.
