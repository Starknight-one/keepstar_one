# Atom

Атомарные UI элементы — базовые строительные блоки.

## Файлы

- `atomModel.js` — AtomType, AtomSubtype, AtomDisplay enums + legacy mapping (LEGACY_TYPE_TO_DISPLAY)
- `AtomRenderer.jsx` — Рендерер по display (с legacy fallback)
- `Atom.css` — Стили атомов (display-based)

## Система типов

### AtomType (базовые типы)

| Type | Описание |
|------|----------|
| text | Текст |
| number | Число |
| image | Изображение |
| icon | Иконка |
| video | Видео |
| audio | Аудио |

### AtomSubtype (форматы данных)

- **text**: string, date, datetime, url, email, phone
- **number**: int, float, currency, percent, rating
- **image**: url, base64
- **icon**: name, emoji, svg

### AtomDisplay (визуальные форматы)

- **text**: h1, h2, h3, h4, body-lg, body, body-sm, caption, badge-*, tag-*
- **number**: price, price-lg, price-old, rating, rating-text, rating-compact, percent, progress
- **image**: image, image-cover, avatar-*, thumbnail, gallery
- **icon**: icon, icon-sm, icon-lg
- **interactive**: button-primary, button-secondary, button-outline, button-ghost
- **layout**: divider, spacer

### Legacy Mapping

`LEGACY_TYPE_TO_DISPLAY` — карта старых типов (price, badge, rating, button, divider, progress, selector) на display-значения.

## Использование

```jsx
<AtomRenderer atom={{ type: 'text', display: 'h2', value: 'Hello' }} />
<AtomRenderer atom={{ type: 'number', subtype: 'currency', display: 'price', value: 1299 }} />
<AtomRenderer atom={{ type: 'number', subtype: 'rating', display: 'rating', value: 4.5 }} />
```
