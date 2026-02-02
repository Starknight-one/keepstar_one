# Chat Feature

Чат с AI-ассистентом с персистентностью сессий.

## Файлы

- `chatModel.js` — Начальное состояние чата
- `useChatMessages.js` — Хук для управления состоянием
- `useChatSubmit.js` — Хук для отправки сообщений
- `ChatPanel.jsx` — Основной компонент чата
- `ChatInput.jsx` — Поле ввода
- `ChatHistory.jsx` — История сообщений
- `ChatPanel.css` — Стили

## Функции

### useChatMessages()
Возвращает состояние и методы для работы с сообщениями.
```js
const {
  sessionId,
  messages,
  isLoading,
  error,
  addMessage,
  setMessages,
  setLoading,
  setError,
  setSessionId
} = useChatMessages();
```

### useChatSubmit({ sessionId, addMessage, setLoading, setError, setSessionId })
Возвращает функцию для отправки сообщений.
```js
const { submit } = useChatSubmit({ ... });
await submit("Привет");
```

## Session Persistence

- `sessionId` сохраняется в localStorage (`chatSessionId`)
- При открытии чата загружается история через `getSession()`
- Если сессия истекла/не найдена — localStorage очищается

## Состояние

```js
{
  sessionId: string | null,
  messages: Message[],
  isLoading: boolean,
  error: string | null
}
```
