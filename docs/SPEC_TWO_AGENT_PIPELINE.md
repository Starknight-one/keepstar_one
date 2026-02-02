# Two-Agent Pipeline

Микроспека двухагентной системы для chat.

## Концепция

```
User Query
    │
    ▼
┌─────────────────┐
│  Agent 1        │  Tool Router
│  (Query→Tools)  │  Вызывает нужные tools
└────────┬────────┘
         │ "Я ок" + данные
         ▼
┌─────────────────┐
│  Agent 2        │  Compose Router
│  (Data→Widgets) │  Собирает ответ
└────────┬────────┘
         │
         ▼
    Widget JSON
```

## Agent 1: Tool Router

**Задача**: понять intent, вызвать tools, вернуть данные.

- Input: user query (свободная форма)
- Output: "Я ок" + результат tools
- Tools: search_products, get_product, filter, compare...
- Retry: до 3 попыток при фейле
- Память: session state (что искали, что показали)

**Экономия токенов**:
- Минимальный system prompt
- Никаких рассуждений, только tool calls
- Anthropic cache для повторяющихся промптов

## Agent 2: Compose Router

**Задача**: собрать виджеты из данных.

- Input: данные от Agent 1
- Output: Widget JSON для frontend
- Роутит сборку: grid, carousel, comparison table...

## Кэширование

Anthropic prompt caching:
- System prompt кэшируется
- Длинные сессии = дешёвые
- Cache TTL учитывать

## TODO

- [ ] Определить tools для Agent 1
- [ ] Формат "Я ок" + данные
- [ ] Validation hooks
- [ ] Widget schema для Agent 2
- [ ] Session state структура
