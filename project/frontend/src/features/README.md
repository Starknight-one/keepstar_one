# Features

Фичи приложения.

## Папки

- `chat/` — Чат панель, ввод, история
- `catalog/` — Каталог товаров, ProductGrid
- `navigation/` — Навигация: BackButton для drill-down. Интегрирован в App.jsx через navState (canGoBack, onExpand, onBack)
- `canvas/` — Канвас виджетов (будущее)
- `overlay/` — Fullscreen overlay

## App.jsx Integration

App.jsx управляет навигацией через state `navState`:
- `navState.canGoBack` — показывает/скрывает BackButton
- `navState.onExpand` — передаётся в FormationRenderer как `onWidgetClick`
- `navState.onBack` — вызывается по клику на BackButton

## Правила

- Импорты из `shared/` и `entities/`
- Каждая фича: model + hooks + components
