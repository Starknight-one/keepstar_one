# Use Cases

Бизнес-логика. Один файл = один use case.

## Файлы

- `chat_send_message.go` — Отправка сообщения с сохранением в БД

## SendMessageUseCase

Основной use case для чата:
- Получает/создаёт сессию
- Проверяет TTL (10 мин sliding window)
- Отправляет сообщение в LLM
- Сохраняет сообщения в БД
- Трекает события (chat_opened, message_sent, message_received)

```go
type SendMessageUseCase struct {
    llm    ports.LLMPort
    cache  ports.CachePort
    events ports.EventPort
    sessionTTL time.Duration
}

func (uc *SendMessageUseCase) Execute(ctx, req) (*SendMessageResponse, error)
func (uc *SendMessageUseCase) WithSessionTTL(ttl) *SendMessageUseCase
```

## Правила

- Импорты только из `domain/` и `ports/`
- Нельзя импортировать `adapters/` напрямую (только через порты)
- Каждый use case — структура с методом Execute()
- Graceful degradation если БД не настроена
