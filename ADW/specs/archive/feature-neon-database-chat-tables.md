# Feature: Подключение Neon PostgreSQL и создание таблиц чата

## Status: COMPLETED ✅

## Feature Description

Подключение к удалённой PostgreSQL базе данных Neon и создание набора таблиц для хранения данных чата: сессии, сообщения, пользователи, и аналитика взаимодействий.

## Objective

1. ✅ Настроить подключение к Neon PostgreSQL через connection string
2. ✅ Создать миграции для таблиц чата
3. ✅ Реализовать PostgreSQL адаптер для CachePort
4. ✅ Обеспечить хранение истории сообщений с метаданными
5. ✅ Интегрировать в SendMessageUseCase
6. ✅ Добавить sliding TTL для сессий (10 минут)
7. ✅ Создать endpoint для получения истории сессии
8. ✅ Обновить фронтенд для сохранения и загрузки истории

## Expertise Context

Expertise used:
- **backend**: Hexagonal architecture с портами и адаптерами. Существующие entities `Session` и `Message` в domain layer. `CachePort` интерфейс для сессий.
- **frontend**: React hooks, localStorage для persistence

## Database Schema Design

### Таблицы

```sql
-- 1. Пользователи/посетители чата
CREATE TABLE chat_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id VARCHAR(255),
    tenant_id VARCHAR(255) NOT NULL,
    fingerprint VARCHAR(255),
    ip_address INET,
    user_agent TEXT,
    metadata JSONB DEFAULT '{}',
    first_seen_at TIMESTAMPTZ DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 2. Сессии чата
CREATE TABLE chat_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES chat_users(id),
    tenant_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) DEFAULT 'active',
    started_at TIMESTAMPTZ DEFAULT NOW(),
    ended_at TIMESTAMPTZ,
    last_activity_at TIMESTAMPTZ DEFAULT NOW(),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 3. Сообщения
CREATE TABLE chat_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL,
    content TEXT NOT NULL,
    widgets JSONB,
    formation JSONB,
    tokens_used INT,
    model_used VARCHAR(100),
    latency_ms INT,
    sent_at TIMESTAMPTZ DEFAULT NOW(),
    received_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 4. События чата (аналитика)
CREATE TABLE chat_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID REFERENCES chat_sessions(id) ON DELETE CASCADE,
    user_id UUID REFERENCES chat_users(id),
    event_type VARCHAR(100) NOT NULL,
    event_data JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Индексы
CREATE INDEX idx_chat_users_tenant ON chat_users(tenant_id);
CREATE INDEX idx_chat_sessions_user ON chat_sessions(user_id);
CREATE INDEX idx_chat_sessions_tenant ON chat_sessions(tenant_id);
CREATE INDEX idx_chat_sessions_status ON chat_sessions(status);
CREATE INDEX idx_chat_messages_session ON chat_messages(session_id);
CREATE INDEX idx_chat_messages_sent_at ON chat_messages(sent_at);
CREATE INDEX idx_chat_events_session ON chat_events(session_id);
CREATE INDEX idx_chat_events_type ON chat_events(event_type);
CREATE INDEX idx_chat_events_created ON chat_events(created_at);
```

### Типы событий (chat_events.event_type)

| Event Type | Description |
|------------|-------------|
| `chat_opened` | Пользователь открыл чат |
| `message_sent` | Пользователь отправил сообщение |
| `message_received` | Пользователь получил ответ |
| `chat_closed` | Пользователь закрыл чат |
| `widget_clicked` | Клик по виджету |
| `session_timeout` | Сессия завершена по таймауту |

## API Endpoints

### POST /api/v1/chat
Отправка сообщения (существующий, обновлён)

Request:
```json
{
  "sessionId": "uuid (optional)",
  "tenantId": "string (optional, default: 'default')",
  "message": "string"
}
```

Response:
```json
{
  "sessionId": "uuid",
  "response": "string",
  "latencyMs": 1234
}
```

### GET /api/v1/session/{id}
Получение истории сессии (новый)

Response:
```json
{
  "id": "uuid",
  "status": "active|closed|archived",
  "messages": [
    {
      "id": "uuid",
      "role": "user|assistant",
      "content": "string",
      "sentAt": "timestamp",
      "latencyMs": 1234
    }
  ],
  "startedAt": "timestamp",
  "lastActivityAt": "timestamp"
}
```

## Session TTL

- **Default TTL**: 10 минут (sliding window)
- При каждом сообщении `lastActivityAt` обновляется
- Если 10 минут без активности → сессия помечается `closed`
- При попытке продолжить expired сессию → создаётся новая

## Files Changed

### Backend - New Files
- `project/backend/internal/adapters/postgres/postgres_client.go` - Connection pool
- `project/backend/internal/adapters/postgres/postgres_cache.go` - CachePort implementation
- `project/backend/internal/adapters/postgres/postgres_events.go` - EventPort implementation
- `project/backend/internal/adapters/postgres/migrations.go` - Auto-migrations
- `project/backend/internal/ports/event_port.go` - EventPort interface
- `project/backend/internal/domain/user_entity.go` - ChatUser entity
- `project/backend/internal/domain/event_entity.go` - ChatEvent entity
- `project/backend/internal/handlers/handler_session.go` - Session history endpoint

### Backend - Modified Files
- `project/backend/go.mod` - Added pgx/v5, uuid dependencies
- `project/backend/internal/config/config.go` - Added DATABASE_URL
- `project/backend/internal/domain/session_entity.go` - Added status, metadata, timestamps
- `project/backend/internal/domain/message_entity.go` - Added sessionId, latency, model fields
- `project/backend/internal/usecases/chat_send_message.go` - Integrated cache/events, added TTL
- `project/backend/internal/handlers/handler_chat.go` - Added sessionId support
- `project/backend/internal/handlers/routes.go` - Added session route
- `project/backend/cmd/server/main.go` - PostgreSQL DI, graceful shutdown

### Frontend - Modified Files
- `project/frontend/src/shared/api/apiClient.js` - Added getSession(), sessionId param
- `project/frontend/src/features/chat/useChatMessages.js` - Added setMessages()
- `project/frontend/src/features/chat/useChatSubmit.js` - localStorage persistence
- `project/frontend/src/features/chat/ChatPanel.jsx` - Load history on mount

## Acceptance Criteria

- [x] Приложение подключается к Neon PostgreSQL при запуске
- [x] Таблицы создаются автоматически при первом запуске (миграции)
- [x] Сессии чата сохраняются в БД
- [x] Сообщения сохраняются с timestamps и метаданными
- [x] События трекаются: открытие чата, отправка/получение сообщений
- [x] Connection pool корректно управляет соединениями
- [x] Graceful shutdown закрывает соединения с БД
- [x] API `/api/v1/chat` работает с PostgreSQL backend
- [x] Session TTL 10 минут (sliding)
- [x] API `/api/v1/session/{id}` возвращает историю
- [x] Фронтенд сохраняет sessionId в localStorage
- [x] Фронтенд загружает историю при открытии чата

## Environment Variables

```env
DATABASE_URL=postgresql://user:pass@host/db?sslmode=require&channel_binding=require
```

## Security Notes

- Connection string хранится только в env variables
- SSL mode `require` для Neon
- Connection string не логируется
- `.env` в `.gitignore`
