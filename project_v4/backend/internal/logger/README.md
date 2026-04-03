# Logger

Структурированное логирование через методы.

## Файлы

- `logger.go` — Базовый логгер, уровни, формат

## Методы

- `ChatMessageReceived(sessionID, message)` — incoming chat message
- `LLMRequestStarted(stage)` — LLM request start
- `LLMResponseReceived(stage, tokens, durationMs)` — LLM response
- `LLMUsage(stage, model, inputTokens, outputTokens, costUSD, durationMs)` — token usage
- `LLMUsageWithCache(stage, model, inputTokens, outputTokens, cacheCreated, cacheRead, costUSD, durationMs)` — token usage with cache metrics (hit rate computed)
- `ToolExecuted(toolName, sessionID, result, durationMs)` — tool execution
- `Agent1Completed(sessionID, toolCalled, productsFound, totalTokens, costUSD, durationMs)` — agent1 summary

## Правила

- Логи — это методы, не inline код
- Структурированные логи (key-value)
- Никогда не логировать PII

## Паттерн

```go
func (l *Logger) ChatMessageReceived(sessionID, message string) {
    l.Info("chat_message_received",
        "session_id", sessionID,
        "message_length", len(message),
    )
}
```
