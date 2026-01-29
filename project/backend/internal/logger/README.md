# Logger

Структурированное логирование через методы.

## Файлы

- `logger.go` — Базовый логгер, уровни, формат

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
