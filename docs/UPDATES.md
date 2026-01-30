# Updates

Лог изменений проекта Keepstar.

---

## 2025-01-29

### Архитектура
- Создана hexagonal структура backend (internal/domain, ports, adapters, usecases, handlers, prompts)
- Создана feature-sliced структура frontend (shared, entities, features, app)
- Старый рабочий код сохранён (main.go, App.jsx, Chat.jsx)
- Новая структура в stubs, готова к миграции

### Expert System
- Заполнены expertise.yaml для backend и frontend
- Обновлены self-improve.md с валидацией YAML и лимитами строк
- Обновлены question.md с примерами и контекстом
- Добавлен README для expert system (ACT → LEARN → REUSE)

### Инструменты
- Добавлен dev-inspector для отладки UI элементов

### Документация
- Создан драфт Product Manifesto (AI_docs/Manifesto)

---
