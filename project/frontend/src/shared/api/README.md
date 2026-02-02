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

### getProducts(tenantSlug, filters)
Получение списка товаров тенанта.

```js
const data = await getProducts("nike", { search: "air max", limit: 10 });
// { products: [...], total: 8 }
```

Фильтры: `category`, `brand`, `search`, `minPrice`, `maxPrice`, `limit`, `offset`

### getProduct(tenantSlug, productId)
Получение одного товара.

```js
const product = await getProduct("nike", "uuid");
// { id, name, price, priceFormatted, images, ... }
// Returns null if not found (404)
```

## API Base

```
http://localhost:8080/api/v1
```
