# API

HTTP клиент для работы с backend.

## Файлы

- `apiClient.js` — HTTP клиент

## Функции

### sendChatMessage(message, sessionId)
Отправка сообщения в чат.

```js
const response = await sendChatMessage("Привет", sessionId);
// { sessionId: "uuid", response: "...", latencyMs: 1234 }
```

### getSession(sessionId)
Получение истории сессии.

```js
const session = await getSession(sessionId);
// { id, status, messages[], startedAt, lastActivityAt }
// Returns null if session not found (404)
```

## API Base

```
http://localhost:8080/api/v1
```
