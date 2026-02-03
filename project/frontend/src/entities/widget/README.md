# Widget

Составные виджеты — композиции атомов.

## Файлы

- `widgetModel.js` — WidgetType, WidgetTemplate, WidgetSize
- `WidgetRenderer.jsx` — Рендерер (template-based или legacy)
- `Widget.css` — Стили виджетов
- `templates/index.js` — Экспорт шаблонов
- `templates/ProductCardTemplate.jsx` — Slot-based карточка товара
- `templates/ProductCardTemplate.css` — Стили ProductCard
- `templates/ServiceCardTemplate.jsx` — Slot-based карточка услуги
- `templates/ServiceCardTemplate.css` — Стили ServiceCard

## Шаблоны (Templates)

| Template | Описание |
|----------|----------|
| ProductCard | Карточка товара со слотами |
| ServiceCard | Карточка услуги (duration, provider) |

## Слоты

| Slot | Назначение |
|------|------------|
| hero | Изображение/карусель |
| badge | Overlay badge |
| title | Заголовок |
| primary | Основные атрибуты (chips) |
| price | Цена |
| secondary | Раскрываемые детали |

## Использование

```jsx
<WidgetRenderer widget={{
  template: 'ProductCard',
  size: 'medium',
  atoms: [
    { type: 'image', value: ['url1', 'url2'], slot: 'hero' },
    { type: 'text', value: 'Product Name', slot: 'title' },
    { type: 'text', value: 'Brand', slot: 'primary' },
    { type: 'price', value: 1299, slot: 'price', meta: { currency: '₽' } },
  ]
}} />
```
