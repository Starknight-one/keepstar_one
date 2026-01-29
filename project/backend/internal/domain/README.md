# Domain

Чистые бизнес-сущности. НОЛЬ зависимостей из проекта.

## Файлы

- `atom_entity.go` — Atom, AtomType (базовый элемент UI)
- `widget_entity.go` — Widget, WidgetType (композиция атомов)
- `formation_entity.go` — Formation (layout виджетов)
- `message_entity.go` — Message (сообщение в чате)
- `session_entity.go` — Session (сессия пользователя)
- `product_entity.go` — Product (товар/услуга)
- `domain_errors.go` — Доменные ошибки

## Правила

- Никаких импортов из `internal/`
- Только стандартная библиотека Go
- Структуры + конструкторы + методы валидации
