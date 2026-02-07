# Chat Feature

Чат с AI-ассистентом с персистентностью сессий.

## Файлы

- `chatModel.js` — Начальное состояние чата
- `useChatMessages.js` — Хук для управления состоянием
- `useChatSubmit.js` — Хук для отправки сообщений
- `sessionCache.js` — localStorage кеш сессии (save/load/clear, TTL 30 мин)
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

## ChatPanel Props

- `onClose` — закрытие чата
- `onFormationReceived` — callback при получении formation
- `onNavigationStateChange` — callback с навигационным состоянием (canGoBack, onExpand, onBack)
- `hideFormation` — скрыть виджеты в сообщениях (рендерятся отдельно)

## Session Persistence

- `sessionId` сохраняется в localStorage (`chatSessionId`)
- Кеш сессии: `chatSessionCache` в localStorage (messages + formation, TTL 30 мин)
- При открытии: мгновенный restore из кеша, затем фоновая валидация через `getSession()`
- Если сессия истекла/не найдена — кеш очищается через `clearSessionCache()`

## Состояние

```js
{
  sessionId: string | null,
  messages: Message[],
  isLoading: boolean,
  error: string | null
}
```
