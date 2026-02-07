# Design System Spec

## Цель
Создать гибкую систему атомов с переключаемыми дизайн-библиотеками.

---

## Ключевая модель

### Три уровня

| Уровень | Отвечает на | Варианты |
|---------|-------------|----------|
| **Атом** | Какие данные? | text, number, image, icon, video, audio |
| **Виджет** | Что показать для сущности? | preset / custom atoms |
| **Формация** | Как расположить? | grid / carousel / list / custom layout |

### Агентная архитектура

```
Agent1: запрос → поиск → АТОМЫ в стейт (что нашли)

Agent2: стейт (атомы) + запрос →
  ├── Атомы → Виджет + Формация   (пресет или кастомный виджет)
  ├── Атомы → Атомы + Формация    (фристайл, без виджетов)
  └── Атомы → Виджет + Атомы + Формация (гибрид, редко)
```

---

## Атом — единица данных

Чистые данные без визуала. **6 типов**, каждый с подтипами.

### Типы и подтипы

**text** — строковые данные
| Subtype | Пример | Автоформат |
|---------|--------|------------|
| `string` | "Nike Air Max" | — |
| `date` | "2026-02-05" | → "5 фев 2026" |
| `time` | "14:30" | → "14:30" |
| `datetime` | "2026-02-05T14:30" | → "5 фев, 14:30" |
| `url` | "https://..." | → кликабельная ссылка |
| `email` | "test@mail.com" | → mailto: |
| `phone` | "+79991234567" | → tel: |

**number** — числовые данные
| Subtype | Пример | Автоформат |
|---------|--------|------------|
| `int` | 42 | "42" |
| `float` | 4.5 | "4.5" |
| `currency` | 99.00 + currency:"USD" | → "$99.00" |
| `percent` | 85 | → "85%" |
| `rating` | 4.5 (0-5) | → ★★★★☆ |

**image** — изображения
| Subtype | Пример |
|---------|--------|
| `url` | "https://cdn.com/img.jpg" |
| `base64` | "data:image/png;base64,..." |

**video** — видео
| Subtype | Пример |
|---------|--------|
| `url` | "https://cdn.com/video.mp4" |
| `embed` | YouTube/Vimeo embed URL |

**audio** — аудио
| Subtype | Пример |
|---------|--------|
| `url` | "https://cdn.com/audio.mp3" |

**icon** — иконки
| Subtype | Пример |
|---------|--------|
| `name` | "heart", "cart", "star" (из библиотеки) |
| `emoji` | "❤️", "🛒" |
| `svg` | inline SVG |

### Структура атома

```typescript
type Atom = {
  type: 'text' | 'number' | 'image' | 'icon' | 'video' | 'audio'
  subtype: string       // string, currency, url, rating...
  value: string | number
  meta?: {              // доп. данные для форматирования
    currency?: string   // USD, RUB, EUR
    locale?: string     // en-US, ru-RU
    min?: number        // для rating: 0
    max?: number        // для rating: 5
  }
  display?: string      // h1, price, badge...
  action?: { ... }
}
```

### Совместимость subtype → display

| Subtype | Допустимые display |
|---------|-------------------|
| `string` | h1, h2, h3, h4, body, body-lg, body-sm, caption, badge, tag |
| `date`, `datetime` | body, caption, badge |
| `currency` | price, price-lg, price-old |
| `rating` | rating, rating-text |
| `percent` | percent, progress |
| `int`, `float` | body, h1-h4, badge |
| `url` (image) | image, avatar, thumbnail, gallery |

### Display — как показать атом

| Display | База | Пример |
|---------|------|--------|
| `h1`, `h2`, `h3`, `h4` | text | Заголовки |
| `body`, `body-lg`, `body-sm` | text | Текст |
| `caption` | text | Подпись |
| `price`, `price-lg`, `price-old` | number | $99, ~~$129~~ |
| `rating`, `rating-text` | number | ★★★★☆ 4.5 |
| `percent`, `progress` | number | 85%, [████░] |
| `badge`, `badge-success`, `badge-error` | text | [SALE], [NEW] |
| `tag` | text | #категория |
| `avatar`, `avatar-sm`, `avatar-lg` | image | Круглое фото |
| `thumbnail`, `gallery` | image | Превью, карусель |
| `icon`, `icon-sm`, `icon-lg` | icon | Иконка |

### Action — интерактивность (опционально)

| Action | Применимо к | Что делает |
|--------|-------------|------------|
| `button` | text, icon | onClick → действие |
| `link` | text, image | onClick → переход |
| `input` | text | onChange → ввод |
| `selector` | text[] | выбор варианта |

```typescript
type Atom = {
  type: 'text' | 'number' | 'image' | 'icon' | 'video' | 'audio'
  value: string | number
  display?: string    // h1, price, badge...
  action?: {
    type: string      // click, change
    handler: string   // add_to_cart, navigate
  }
}
```

---

## Виджет — контейнер атомов для одной сущности

**Что показать** для товара/услуги. Пресетный или кастомный.

### Пресетный виджет
```json
{ "preset": "product-card", "entity": "product-123" }
```
Бэкенд знает: product-card = image + title + price + rating

### Кастомный виджет
```json
{ "entity": "product-123", "atoms": ["image", "price", "size", "delivery"] }
```
Пользователь выбрал конкретные атомы

### Слоты (для пресетов)
```
hero      — главное изображение
badge     — бейджи (SALE, NEW)
title     — название
rating    — рейтинг
price     — цена
meta      — доп. инфо (размеры, цвета)
actions   — кнопки
secondary — второстепенное
```

---

## Формация — как расположить

Layout для виджетов или атомов.

### Стандартные
| Mode | Описание |
|------|----------|
| `grid` | Сетка (cols: 2/3/4/auto) |
| `carousel` | Горизонтальный скролл |
| `list` | Вертикальный список |
| `single` | Один элемент |

### Кастомные (фристайл)
Любой layout: `circle`, `infinity`, `comparison-table`, etc.

---

## Примеры

### Пресет
```
Запрос: "покажи кроссовки Nike"

Agent1 → атомы в стейт
Agent2 → preset:product-card + formation:grid

┌─────────┐ ┌─────────┐ ┌─────────┐
│ [img]   │ │ [img]   │ │ [img]   │
│ Nike 1  │ │ Nike 2  │ │ Nike 3  │
│ $99     │ │ $129    │ │ $89     │
└─────────┘ └─────────┘ └─────────┘
```

### Кастомный виджет
```
Запрос: "покажи Nike с ценами, размерами и доставкой"

Agent2 → custom atoms + formation:list

┌───────────────────────────────────┐
│ [img] Nike Air  $99  42-45  5 фев │
└───────────────────────────────────┘
```

### Фристайл
```
Запрос: "только фотки в круг"

Agent2 → atoms:image[] + formation:circle

      [img1]
   [img4]  [img2]
      [img3]
```

---

---

## Маппинг: Pencil → Модель

### Text displays (8)
| Pencil компонент | → | display |
|------------------|---|---------|
| Atom/H1 | → | `h1` |
| Atom/H2 | → | `h2` |
| Atom/H3 | → | `h3` |
| Atom/H4 | → | `h4` |
| Atom/Body Large | → | `body-lg` |
| Atom/Body | → | `body` |
| Atom/Body Small | → | `body-sm` |
| Atom/Caption | → | `caption` |

### Number displays — Price (4)
| Pencil компонент | → | display | subtype |
|------------------|---|---------|---------|
| Atom/Price Large | → | `price-lg` | currency |
| Atom/Price | → | `price` | currency |
| Atom/Price Old | → | `price-old` | currency |
| Atom/Price Discount | → | `price-discount` | currency |

### Number displays — Rating (3)
| Pencil компонент | → | display | subtype |
|------------------|---|---------|---------|
| Atom/Rating 5 Stars | → | `rating` | rating |
| Atom/Rating With Text | → | `rating-text` | rating |
| Atom/Rating Compact | → | `rating-compact` | rating |

### Text displays — Badge (4)
| Pencil компонент | → | display |
|------------------|---|---------|
| Atom/Badge Sale | → | `badge-error` (красный) |
| Atom/Badge New | → | `badge-success` (зелёный) |
| Atom/Badge Bestseller | → | `badge-warning` (оранж) |
| Atom/Badge Free Shipping | → | `badge-info` (синий) |

### Text displays — Tag (2)
| Pencil компонент | → | display |
|------------------|---|---------|
| Atom/Tag Category | → | `tag` |
| Atom/Tag Active | → | `tag-active` |

### Image displays — Avatar (4)
| Pencil компонент | → | display |
|------------------|---|---------|
| Atom/Avatar Large | → | `avatar-lg` |
| Atom/Avatar Medium | → | `avatar` |
| Atom/Avatar Small | → | `avatar-sm` |
| Atom/Avatar With Badge | → | `avatar-badge` |

### Icon library (12 иконок)
| Pencil | → | icon name |
|--------|---|-----------|
| Atom/Icon Home | → | `home` |
| Atom/Icon Search | → | `search` |
| Atom/Icon Cart | → | `cart` |
| Atom/Icon Heart | → | `heart` |
| Atom/Icon User | → | `user` |
| Atom/Icon Star | → | `star` |
| Atom/Icon Plus | → | `plus` |
| Atom/Icon Minus | → | `minus` |
| Atom/Icon Trash | → | `trash` |
| Atom/Icon Chevron Right | → | `chevron-right` |
| Atom/Icon Filter | → | `filter` |
| Atom/Icon Package | → | `package` |

### Interactive — Button (7)
| Pencil компонент | → | action + display |
|------------------|---|------------------|
| Atom/Button Primary | → | `button-primary` |
| Atom/Button Secondary | → | `button-secondary` |
| Atom/Button Outline | → | `button-outline` |
| Atom/Button Ghost | → | `button-ghost` |
| Atom/Button Icon | → | `button-icon` |
| Atom/Button Icon Small | → | `button-icon-sm` |
| Atom/Button Danger | → | `button-danger` |

### Interactive — Input (2)
| Pencil компонент | → | action + display |
|------------------|---|------------------|
| Atom/Input | → | `input` |
| Atom/Input With Icon | → | `input-icon` |

### Widget presets (8)
| Pencil компонент | → | preset |
|------------------|---|--------|
| Widget/Product Card | → | `product-card` |
| Widget/Product Card Horizontal | → | `product-card-horizontal` |
| Widget/Category Card | → | `category-card` |
| Widget/Category Card Compact | → | `category-card-compact` |
| Widget/Cart Item | → | `cart-item` |
| Widget/Search Bar | → | `search-bar` |
| Widget/Header | → | `header` |
| Widget/Tab Bar | → | `tab-bar` |

---

## Дизайн-библиотека = CSS для displays

Тема определяет **как выглядят displays**, данные не меняются.

### CSS Variables
```css
:root {
  --accent-primary: #8B5CF6;
  --bg-page: #FFFFFF;
  --bg-card: #F4F4F5;
  --text-primary: #18181B;
  --text-secondary: #71717A;
  --error: #EF4444;
  --success: #22C55E;

  --font-display: 'Plus Jakarta Sans', sans-serif;
  --font-body: 'Inter', sans-serif;

  --radius-sm: 8px;
  --radius-md: 12px;
  --radius-lg: 16px;
  --radius-pill: 100px;
}
```

### Переключение
```html
<div class="theme-marketplace">...</div>
<div class="theme-minimal">...</div>
```

---

## TODO

- [ ] Обновить atomModel.js — 6 чистых типов + display
- [ ] Обновить AtomRenderer.jsx — рендер по display
- [ ] Экспортировать CSS из Pencil как первую тему
- [ ] Механизм переключения тем
- [ ] Пресеты виджетов на бэке
