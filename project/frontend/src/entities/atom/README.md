# Atom

Атомарные UI элементы — базовые строительные блоки.

## Файлы

- `atomModel.js` — Типы атомов (AtomType)
- `AtomRenderer.jsx` — Рендерер любого атома по типу
- `Atom.css` — Стили атомов

## Типы атомов

| Type | Описание | Meta |
|------|----------|------|
| TEXT | Текст | style |
| NUMBER | Число | format (currency, percent, compact) |
| PRICE | Цена | currency |
| IMAGE | Изображение | size, label |
| RATING | Звёздный рейтинг | - |
| BADGE | Бейдж | variant |
| BUTTON | Кнопка | action |
| ICON | Иконка | - |
| DIVIDER | Разделитель | - |
| PROGRESS | Прогресс-бар | - |

## Использование

```jsx
<AtomRenderer atom={{ type: 'TEXT', value: 'Hello', meta: { style: 'bold' } }} />
<AtomRenderer atom={{ type: 'PRICE', value: 1299, meta: { currency: '₽' } }} />
<AtomRenderer atom={{ type: 'RATING', value: 4.5 }} />
```
