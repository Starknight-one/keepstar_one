# Entities

Бизнес-сущности UI.

## Папки

- `atom/` — Атомарные UI элементы (Text, Image, Price, Rating...)
- `widget/` — Составные виджеты (ProductCard, ProductDetail, ServiceCard, ServiceDetail, TextBlock, QuickReplies)
- `formation/` — Layout виджетов (grid, carousel, single, list)
- `message/` — Сообщения чата

## Правила

- Импорты только из `shared/`
- Каждая сущность: model + renderer
