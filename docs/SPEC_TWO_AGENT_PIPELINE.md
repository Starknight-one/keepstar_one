# Two-Agent Pipeline

Микроспека двухагентной системы для chat.

## Концепция

```
User Query
    │
    ▼
┌─────────────────┐
│  Agent 1        │  Tool Router
│  (Query→Tools)  │
└────────┬────────┘
         │
         ├──→ tool_call ──→ [Tool] ──→ Session State
         │                     │
         │←── "ok" / "empty" ──┘
         │
         ▼
   "Я ок" + флаг для Agent 2
         │
         ▼
┌─────────────────┐
│  Agent 2        │  Compose Router
│  (State→Widget) │  Читает из Session State
└────────┬────────┘
         │
         ▼
    Widget JSON
```

## Agent 1: Tool Router

**Задача**: из свободного текста понять какие tools вызвать и с какими параметрами. Супер быстро.

**Ключевое**: агент НЕ видит данные из tools. Tools сами пишут в Session State. Агент получает только сигнал "ok" или "empty".

### Логика принятия решений

Агент помнит историю вызовов → знает что лежит в Session State:

1. **Нужны новые данные?** → tool_call(search_products, {...})
2. **Данные уже есть, нужна модификация?** → флаг Agent 2:
   - изменить layout (grid → carousel)
   - изменить стиль (шрифты, размеры)
   - убрать часть данных
   - отсортировать по-другому

### Input/Output

- Input: user query (свободная форма)
- Output: "Я ок" + флаг/инструкция для Agent 2
- Tools: search_products, get_product, filter, compare...
- Retry: до 3 попыток при фейле (validation hooks)

### Память

- История tool calls за сессию
- Знает что в кэше/state прямо сейчас
- Не хранит сами данные, только метаинфу

### Экономия токенов

- Минимальный system prompt
- Никаких рассуждений, только tool calls
- Не читает результаты tools
- Anthropic prompt caching

## Agent 2: Compose Router

**Задача**: собрать виджеты из Session State.

- Input: флаг/инструкция от Agent 1
- Читает: Session State (данные)
- Output: Widget JSON для frontend
- Роутит сборку: grid, carousel, comparison table...

## Session State

```
{
  products: [...],        // данные от tools
  filters_applied: {...}, // что применено
  layout_hint: "grid",    // текущий layout
  history: [...]          // история tool calls
}
```

## Кэширование

Anthropic prompt caching:
- System prompt кэшируется
- Длинные сессии = дешёвые
- Cache TTL учитывать

## TODO

- [ ] Определить tools для Agent 1
- [ ] Формат флагов Agent 1 → Agent 2
- [ ] Validation hooks
- [ ] Widget schema для Agent 2
- [ ] Session State структура в БД
