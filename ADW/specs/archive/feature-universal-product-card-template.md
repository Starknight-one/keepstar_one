# Feature: Universal Product Card Template

## Feature Description
Замена текущей hardcoded системы виджетов (PRODUCT_CARD, TEXT_BLOCK) на универсальную template-систему с динамическими слотами. Template определяет не конкретные поля, а правила отображения: какие атрибуты показывать, в каком порядке, какого размера. AI/backend определяет приоритеты атрибутов по категории товара.

## Objective
- Убрать hardcoded widget types для товаров
- Создать универсальный ProductCardTemplate с динамическими слотами
- Primary атрибуты (2-4) показываются сразу, secondary - по клику/expand
- Один template работает для любой категории (кроссовки, ноутбуки, телефоны)
- AI решает какие атрибуты primary для данной категории

## Expertise Context
Expertise used:
- **frontend-entities**: Текущая структура atom → widget → formation. Паттерн model.js + Renderer.jsx + .css
- **frontend-features**: Feature module pattern с hooks

## Relevant Files

### Existing Files
- `project/frontend/src/entities/widget/widgetModel.js` - текущие WidgetType, WidgetSize
- `project/frontend/src/entities/widget/WidgetRenderer.jsx` - switch по type, нужно заменить на template-based
- `project/frontend/src/entities/widget/Widget.css` - базовые стили виджетов
- `project/frontend/src/entities/atom/AtomRenderer.jsx` - рендеринг атомов (оставить как есть)
- `project/frontend/src/entities/atom/Atom.css` - стили атомов
- `project/frontend/src/entities/formation/FormationRenderer.jsx` - layout виджетов
- `project/frontend/src/entities/formation/Formation.css` - grid/carousel/list стили

### New Files
- `project/frontend/src/entities/widget/templates/ProductCardTemplate.jsx` - универсальный template
- `project/frontend/src/entities/widget/templates/ProductCardTemplate.css` - стили template
- `project/frontend/src/entities/widget/templates/index.js` - экспорт templates

## Data Structure

### Текущая (hardcoded):
```json
{
  "type": "product_card",
  "atoms": [
    { "type": "image", "value": "url" },
    { "type": "text", "value": "Nike" },
    { "type": "price", "value": 195 }
  ]
}
```

### Новая (template-based):
```json
{
  "template": "ProductCard",
  "size": "medium",
  "data": {
    "images": ["url1", "url2", "url3"],
    "title": "Zoom Soldier III",
    "badge": { "text": "Hit!!!", "variant": "success" },
    "primary": [
      { "key": "brand", "label": "Brand", "value": "Nike", "display": "chip" },
      { "key": "size", "label": "Size", "value": [42, 43, 44], "display": "selector" }
    ],
    "price": { "value": 195, "currency": "$" },
    "secondary": [
      { "key": "color", "label": "Color", "value": "Black/White" },
      { "key": "material", "label": "Material", "value": "Leather" }
    ]
  }
}
```

## Step by Step Tasks
IMPORTANT: Execute strictly in order.

### 1. Создать структуру папки templates
- Создать `project/frontend/src/entities/widget/templates/`
- Создать `project/frontend/src/entities/widget/templates/index.js`

### 2. Создать ProductCardTemplate.jsx
Компонент с секциями:
- **ImageSection**: carousel с точками навигации (если несколько images)
- **BadgeOverlay**: позиционированный badge в углу (опционально)
- **TitleSection**: заголовок товара
- **PrimaryAttributes**: chips/selectors для primary атрибутов
- **PriceSection**: цена на всю ширину
- **ExpandButton**: кнопка "Подробнее" если есть secondary

Props:
```jsx
function ProductCardTemplate({ data, size, onExpand, expanded }) {
  // data.images, data.title, data.badge, data.primary, data.price, data.secondary
}
```

### 3. Создать ProductCardTemplate.css
Стили для:
- `.product-card-template` - контейнер
- `.product-card-images` - image carousel с dots
- `.product-card-badge` - абсолютно позиционированный badge
- `.product-card-title` - заголовок
- `.product-card-primary` - row с chips
- `.product-card-chip` - отдельный chip атрибута
- `.product-card-selector` - selector для вариантов (размеры)
- `.product-card-price` - блок цены
- `.product-card-secondary` - expandable секция
- Size модификаторы: `.size-small`, `.size-medium`, `.size-large`

### 4. Создать компонент ImageCarousel
В том же файле или отдельно:
- Принимает массив images
- Показывает dots навигации
- Свайп или клик для переключения

### 5. Создать компонент AttributeChip
Универсальный chip для атрибута:
- `display: "chip"` - простой текст в обводке
- `display: "selector"` - варианты выбора (размеры)
- `display: "text"` - просто текст без обводки

### 6. Обновить widgetModel.js
Добавить:
```js
export const WidgetTemplate = {
  PRODUCT_CARD: 'ProductCard',
  // Будущие templates
};
```

### 7. Обновить WidgetRenderer.jsx
Добавить template-based рендеринг:
```jsx
import { ProductCardTemplate } from './templates';

export function WidgetRenderer({ widget }) {
  // Если есть template - использовать template-based рендеринг
  if (widget.template) {
    return renderTemplate(widget);
  }

  // Fallback на старую логику для обратной совместимости
  switch (widget.type) {
    // ... старый код
  }
}

function renderTemplate(widget) {
  switch (widget.template) {
    case 'ProductCard':
      return <ProductCardTemplate data={widget.data} size={widget.size} />;
    default:
      return <DefaultWidget widget={widget} />;
  }
}
```

### 8. Обновить Formation.css для auto-fill grid
Заменить фиксированные cols на responsive:
```css
.formation-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 20px;
}
```

### 9. Добавить expanded state для карточек
В ProductCardTemplate:
- Локальный state `expanded`
- При expanded=true показывать secondary атрибуты
- Анимация expand/collapse

### 10. Validation
- Запустить `npm run build` в project/frontend
- Запустить `npm run lint` в project/frontend

## Validation Commands
```bash
cd project/frontend && npm run build
cd project/frontend && npm run lint
```

## Acceptance Criteria
- [ ] ProductCardTemplate рендерит карточку товара
- [ ] Images отображаются как carousel с dots
- [ ] Badge позиционируется в углу с поворотом
- [ ] Primary атрибуты отображаются как chips
- [ ] Атрибуты с массивом значений (размеры) отображаются как selector
- [ ] Price занимает всю ширину
- [ ] Кнопка expand показывает secondary атрибуты
- [ ] Template работает с разными size (small/medium/large)
- [ ] Grid автоматически распределяет карточки (auto-fill)
- [ ] Frontend собирается без ошибок
- [ ] Lint проходит без ошибок

## Notes
- Обратная совместимость: старый формат `{ type: "product_card", atoms: [...] }` продолжает работать
- Backend должен будет обновить формат response в pipeline (отдельная задача)
- Для selector (размеры) нужен onClick handler - пока заглушка, потом добавим action dispatch
- Secondary атрибуты могут быть undefined - template должен это обрабатывать
- ImageCarousel может быть выделен в отдельный entity позже

## Architecture Diagram
```
Formation
├── WidgetRenderer
│   ├── template? → renderTemplate()
│   │   └── ProductCardTemplate
│   │       ├── ImageCarousel
│   │       ├── BadgeOverlay
│   │       ├── TitleSection
│   │       ├── PrimaryAttributes
│   │       │   └── AttributeChip (chip | selector | text)
│   │       ├── PriceSection
│   │       └── SecondaryAttributes (expandable)
│   │
│   └── type? → switch (old logic)
│       ├── ProductCard (deprecated)
│       ├── TextBlock
│       └── QuickReplies
```
