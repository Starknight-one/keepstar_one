# Widget

Составные виджеты — композиции атомов.

## Файлы

- `widgetModel.js` — Типы виджетов (WidgetType)
- `WidgetRenderer.jsx` — Рендерер любого виджета по типу
- `Widget.css` — Стили виджетов

## Типы виджетов

| Type | Описание |
|------|----------|
| PRODUCT_CARD | Карточка товара |
| TEXT_BLOCK | Текстовый блок |
| QUICK_REPLIES | Быстрые ответы (кнопки) |

## Использование

```jsx
<WidgetRenderer widget={{
  type: 'PRODUCT_CARD',
  size: 'medium',
  atoms: [
    { type: 'IMAGE', value: 'url', meta: { size: 'large' } },
    { type: 'TEXT', value: 'Product Name', meta: { style: 'bold' } },
    { type: 'PRICE', value: 1299, meta: { currency: '₽' } },
  ]
}} />
```
