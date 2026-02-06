# Feature: Design System Integration

**ADW-ID**: DSI-001
**Complexity**: complex
**Layers**: backend, frontend

---

## Implementation Status

| Phase | Task | Status |
|-------|------|--------|
| 1.1 | atom_entity.go — 6 типов + Subtype + Display | ✅ Done |
| 1.2 | preset_entity.go — Display в FieldConfig | ✅ Done |
| 1.3 | display_entity.go — создать | ✅ Done |
| 1.4 | product_presets.go — display mapping | ✅ Done |
| 1.5 | service_presets.go — display mapping | ✅ Done |
| 1.6 | tool_render_preset.go — atom.Display | ✅ Done |
| 1.7 | tool_freestyle.go — создать | ✅ Done |
| 1.8 | prompt_compose_widgets.go — обновить промпты | ✅ Done |
| 1.9 | template_apply.go — новые типы | ✅ Done (fixed build error) |
| 1.10 | agent2_execute_test.go — тесты | ✅ Done (fixed build error) |
| 2.1 | atomModel.js — 6 типов + enums | ✅ Done |
| 2.2 | AtomRenderer.jsx — рендер по display | ✅ Done |
| 2.3 | Atom.css — стили | ✅ Done → 🔧 Fixed (Pencil values) |
| 3.1 | ThemeProvider.jsx | ✅ Done |
| 3.2 | Pencil MCP extraction | ❌ FORGOTTEN → 🔧 Fixed later |
| 3.3 | App.jsx integration | ✅ Done |
| — | Widget templates use AtomRenderer | ❌ FORGOTTEN → 🔧 Fixed later |
| — | ProductCardTemplate.css | ❌ FORGOTTEN → 🔧 Fixed (Pencil design) |
| — | ServiceCardTemplate.css | ❌ FORGOTTEN → 🔧 Fixed (Pencil design) |
| — | ProductDetailTemplate.css | ❌ FORGOTTEN → 🔧 Fixed later |
| — | ServiceDetailTemplate.css | ❌ FORGOTTEN → 🔧 Fixed later |
| — | index.html — Google Fonts | ❌ FORGOTTEN → 🔧 Fixed |
| — | lucide-react dependency | ✅ Done |
| — | Formation.css | ❓ Not checked |
| — | Widget.css | ❓ Not checked |

---

## Feature Description

Полный рефакторинг системы атомов на модель **6 типов + subtype + display**. Обновление backend и frontend синхронно. Интеграция дизайн-библиотек из Pencil с переключаемыми темами.

---

## Архитектура

### Модель трёх уровней
```
Agent1: запрос → поиск → АТОМЫ в стейт (сырые данные)

Agent2: стейт (атомы) + запрос → выбирает режим:
  ├── ПРЕСЕТ: use_preset("ProductCard", atoms)
  │   └── Backend ставит displays по пресету
  │
  ├── КАСТОМНЫЙ ПРЕСЕТ: use_preset("ProductCard", atoms, overrides)
  │   └── Backend + агент переопределяет часть displays
  │
  └── ФРИСТАЙЛ: freestyle({ style: "product-hero", atoms, formation })
      └── Агент контролирует всё через style-алиасы
```

### Атом = единица данных
```
{
  type: "number",           // 6 типов: text, number, image, icon, video, audio
  subtype: "currency",      // формат данных
  display: "price-lg",      // визуальное представление (опционально)
  value: 99.99,
  slot: "price",            // слот в виджете
  meta: { currency: "USD" }
}
```

### Кто ставит display?
| Режим | Кто решает | Как |
|-------|------------|-----|
| Пресет | Backend | Пресет содержит маппинг slot → display |
| Кастомный пресет | Backend + Agent2 | Пресет + overrides от агента |
| Фристайл | Agent2 | Style-алиас или явные displays |

### Style-алиасы (оптимизация фристайла)
```go
var DisplayStyles = map[string]map[string]string{
  "product-hero": {
    "title": "h1",
    "price": "price-lg",
    "badge": "badge-success",
    "rating": "rating",
  },
  "product-compact": {
    "title": "h3",
    "price": "price",
    "badge": "tag",
    "rating": "rating-compact",
  },
  "service-card": {
    "title": "h2",
    "duration": "caption",
    "rating": "rating-compact",
  },
}
```

---

## Objective

1. **Backend:** Обновить AtomType на 6 типов + добавить Subtype, Display
2. **Backend:** Обновить пресеты с display-маппингом
3. **Backend:** Добавить freestyle tool с style-алиасами
4. **Frontend:** Обновить AtomRenderer на рендер по display
5. **Frontend:** Экспортировать CSS из Pencil как тему
6. **Frontend:** ThemeProvider для переключения тем

---

## Expertise Context

**backend-domain**:
- `atom_entity.go` — текущие 11 типов (включая selector)
- Пресеты в `internal/presets/`

**backend-handlers**:
- Tools для Agent2 в `internal/tools/`

**frontend-entities**:
- `atomModel.js` — 10 типов
- `AtomRenderer.jsx` — switch по type
- Виджеты используют templates и slots

---

## Relevant Files

### Backend (изменить)
- `project/backend/internal/domain/atom_entity.go` — новая модель атома ✅
- `project/backend/internal/domain/preset_entity.go` — добавить Display в FieldConfig ✅
- `project/backend/internal/domain/display_entity.go` — **создать** Display enum + styles ✅
- `project/backend/internal/presets/product_presets.go` — добавить display mapping ✅
- `project/backend/internal/presets/service_presets.go` — добавить display mapping ✅
- `project/backend/internal/tools/tool_render_preset.go` — использовать atom.Display вместо meta.display ✅
- `project/backend/internal/tools/tool_freestyle.go` — **создать** freestyle tool ✅
- `project/backend/internal/prompts/prompt_compose_widgets.go` — обновить промпты ✅
- `project/backend/internal/usecases/template_apply.go` — обновить под новые типы ✅
- `project/backend/internal/usecases/agent2_execute_test.go` — обновить тесты ✅

### Frontend (изменить)
- `project/frontend/src/entities/atom/atomModel.js` — 6 типов + enums ✅
- `project/frontend/src/entities/atom/AtomRenderer.jsx` — рендер по display ✅
- `project/frontend/src/entities/atom/Atom.css` — структурные стили ✅ → 🔧 Fixed with Pencil values

### Frontend (создать)
- `project/frontend/src/shared/theme/themeModel.js` — ThemeType enum ✅
- `project/frontend/src/shared/theme/ThemeProvider.jsx` — контекст тем ✅
- `project/frontend/src/shared/theme/themes/marketplace.css` — тема из Pencil ✅ → 🔧 Fixed with real Pencil tokens
- `project/frontend/src/shared/theme/index.js` — экспорты ✅

### Frontend (НЕ БЫЛО В СПЕКЕ — пропущено)
- `project/frontend/src/entities/widget/templates/ProductCardTemplate.jsx` — 🔧 Fixed to use AtomRenderer
- `project/frontend/src/entities/widget/templates/ProductCardTemplate.css` — 🔧 Fixed with Pencil design
- `project/frontend/src/entities/widget/templates/ServiceCardTemplate.jsx` — 🔧 Fixed to use AtomRenderer
- `project/frontend/src/entities/widget/templates/ServiceCardTemplate.css` — 🔧 Fixed with Pencil design
- `project/frontend/src/entities/widget/templates/ProductDetailTemplate.jsx` — 🔧 Fixed to use AtomRenderer
- `project/frontend/src/entities/widget/templates/ProductDetailTemplate.css` — 🔧 Fixed with Pencil design
- `project/frontend/src/entities/widget/templates/ServiceDetailTemplate.jsx` — 🔧 Fixed to use AtomRenderer
- `project/frontend/src/entities/widget/templates/ServiceDetailTemplate.css` — 🔧 Fixed with Pencil design
- `project/frontend/index.html` — Google Fonts 🔧 Fixed

---

## Step by Step Tasks

### Phase 1: Backend — Новая модель атома

#### 1.1 Обновить atom_entity.go ✅ DONE

```go
package domain

// AtomType — 6 базовых типов данных
type AtomType string

const (
    AtomTypeText   AtomType = "text"
    AtomTypeNumber AtomType = "number"
    AtomTypeImage  AtomType = "image"
    AtomTypeIcon   AtomType = "icon"
    AtomTypeVideo  AtomType = "video"
    AtomTypeAudio  AtomType = "audio"
)

// AtomSubtype — формат данных внутри типа
type AtomSubtype string

const (
    // text subtypes
    SubtypeString   AtomSubtype = "string"
    SubtypeDate     AtomSubtype = "date"
    SubtypeDatetime AtomSubtype = "datetime"
    SubtypeURL      AtomSubtype = "url"
    SubtypeEmail    AtomSubtype = "email"
    SubtypePhone    AtomSubtype = "phone"

    // number subtypes
    SubtypeInt      AtomSubtype = "int"
    SubtypeFloat    AtomSubtype = "float"
    SubtypeCurrency AtomSubtype = "currency"
    SubtypePercent  AtomSubtype = "percent"
    SubtypeRating   AtomSubtype = "rating"

    // image subtypes
    SubtypeImageURL    AtomSubtype = "url"
    SubtypeImageBase64 AtomSubtype = "base64"

    // icon subtypes
    SubtypeIconName  AtomSubtype = "name"
    SubtypeIconEmoji AtomSubtype = "emoji"
    SubtypeIconSVG   AtomSubtype = "svg"
)

// Atom — единица данных
type Atom struct {
    Type    AtomType               `json:"type"`
    Subtype AtomSubtype            `json:"subtype,omitempty"`
    Display string                 `json:"display,omitempty"` // визуальный формат
    Value   interface{}            `json:"value"`
    Slot    AtomSlot               `json:"slot,omitempty"`
    Meta    map[string]interface{} `json:"meta,omitempty"`
}
```

#### 1.2 Обновить preset_entity.go ✅ DONE

Добавить Display поле в FieldConfig:

```go
// FieldConfig defines how a field maps to an atom in a slot
type FieldConfig struct {
    Name     string      `json:"name"`
    Slot     AtomSlot    `json:"slot"`
    AtomType AtomType    `json:"atomType"` // остаётся для обратной совместимости
    Display  AtomDisplay `json:"display"`  // ← новое поле
    Priority int         `json:"priority"`
    Required bool        `json:"required"`
}
```

#### 1.4 Создать display_entity.go ✅ DONE

```go
package domain

// AtomDisplay — визуальные форматы
type AtomDisplay string

const (
    // text displays
    DisplayH1        AtomDisplay = "h1"
    DisplayH2        AtomDisplay = "h2"
    DisplayH3        AtomDisplay = "h3"
    DisplayH4        AtomDisplay = "h4"
    DisplayBodyLg    AtomDisplay = "body-lg"
    DisplayBody      AtomDisplay = "body"
    DisplayBodySm    AtomDisplay = "body-sm"
    DisplayCaption   AtomDisplay = "caption"
    DisplayBadge     AtomDisplay = "badge"
    DisplayBadgeSuccess AtomDisplay = "badge-success"
    DisplayBadgeError   AtomDisplay = "badge-error"
    DisplayBadgeWarning AtomDisplay = "badge-warning"
    DisplayTag       AtomDisplay = "tag"
    DisplayTagActive AtomDisplay = "tag-active"

    // number displays
    DisplayPrice         AtomDisplay = "price"
    DisplayPriceLg       AtomDisplay = "price-lg"
    DisplayPriceOld      AtomDisplay = "price-old"
    DisplayPriceDiscount AtomDisplay = "price-discount"
    DisplayRating        AtomDisplay = "rating"
    DisplayRatingText    AtomDisplay = "rating-text"
    DisplayRatingCompact AtomDisplay = "rating-compact"
    DisplayPercent       AtomDisplay = "percent"
    DisplayProgress      AtomDisplay = "progress"

    // image displays
    DisplayImage      AtomDisplay = "image"
    DisplayImageCover AtomDisplay = "image-cover"
    DisplayAvatar     AtomDisplay = "avatar"
    DisplayAvatarSm   AtomDisplay = "avatar-sm"
    DisplayAvatarLg   AtomDisplay = "avatar-lg"
    DisplayThumbnail  AtomDisplay = "thumbnail"
    DisplayGallery    AtomDisplay = "gallery"

    // icon displays
    DisplayIcon   AtomDisplay = "icon"
    DisplayIconSm AtomDisplay = "icon-sm"
    DisplayIconLg AtomDisplay = "icon-lg"

    // interactive displays
    DisplayButtonPrimary   AtomDisplay = "button-primary"
    DisplayButtonSecondary AtomDisplay = "button-secondary"
    DisplayButtonOutline   AtomDisplay = "button-outline"
    DisplayButtonGhost     AtomDisplay = "button-ghost"
    DisplayInput           AtomDisplay = "input"

    // layout displays
    DisplayDivider AtomDisplay = "divider"
    DisplaySpacer  AtomDisplay = "spacer"
)

// DisplayStyle — алиас для набора displays
type DisplayStyle string

const (
    StyleProductHero    DisplayStyle = "product-hero"
    StyleProductCompact DisplayStyle = "product-compact"
    StyleProductDetail  DisplayStyle = "product-detail"
    StyleServiceCard    DisplayStyle = "service-card"
    StyleServiceDetail  DisplayStyle = "service-detail"
)

// DisplayStyles — маппинг style → slot → display
var DisplayStyles = map[DisplayStyle]map[AtomSlot]AtomDisplay{
    StyleProductHero: {
        AtomSlotTitle:   DisplayH1,
        AtomSlotPrice:   DisplayPriceLg,
        AtomSlotBadge:   DisplayBadgeSuccess,
        AtomSlotPrimary: DisplayTag,
        AtomSlotHero:    DisplayImageCover,
    },
    StyleProductCompact: {
        AtomSlotTitle:   DisplayH3,
        AtomSlotPrice:   DisplayPrice,
        AtomSlotBadge:   DisplayTag,
        AtomSlotPrimary: DisplayCaption,
        AtomSlotHero:    DisplayThumbnail,
    },
    StyleServiceCard: {
        AtomSlotTitle:   DisplayH2,
        AtomSlotPrice:   DisplayPrice,
        AtomSlotPrimary: DisplayCaption,
    },
}
```

#### 1.5 Обновить пресеты ✅ DONE

Пресеты теперь содержат display mapping:

```go
// product_presets.go
var ProductCardPreset = Preset{
    Name: "ProductCard",
    Displays: map[AtomSlot]AtomDisplay{
        AtomSlotHero:    DisplayImageCover,
        AtomSlotBadge:   DisplayBadge,
        AtomSlotTitle:   DisplayH2,
        AtomSlotPrimary: DisplayTag,
        AtomSlotPrice:   DisplayPrice,
    },
}
```

#### 1.6 Обновить tool_render_preset.go ✅ DONE

Функция `buildAtoms` сейчас использует `atom.Meta["display"]`. Обновить на `atom.Display`:

```go
// Было:
atom.Meta = map[string]interface{}{"display": "chip"}

// Стало:
atom.Display = string(field.Display)
```

#### 1.7 Создать tool_freestyle.go ✅ DONE

```go
// FreestyleInput — входные данные для freestyle tool
type FreestyleInput struct {
    Style     DisplayStyle          `json:"style,omitempty"`     // алиас стиля
    Atoms     []Atom                `json:"atoms"`
    Overrides map[string]string     `json:"overrides,omitempty"` // slot → display
    Formation FormationMode         `json:"formation"`
}

func (t *FreestyleTool) Execute(input FreestyleInput) (*Widget, error) {
    // 1. Если есть style — применить DisplayStyles[style]
    // 2. Если есть overrides — переопределить
    // 3. Собрать виджет с атомами
}
```

#### 1.8 Обновить prompt_compose_widgets.go ✅ DONE

Промпты для Agent2 обновлены:
- `Agent2SystemPrompt`: 6 типов + subtypes + displays
- `Agent2ToolSystemPrompt`: добавлен freestyle tool, style aliases, display overrides

### Phase 2: Frontend — Рендер по display

#### 2.1 Обновить atomModel.js ✅ DONE

```javascript
// 6 базовых типов
export const AtomType = {
  TEXT: 'text',
  NUMBER: 'number',
  IMAGE: 'image',
  ICON: 'icon',
  VIDEO: 'video',
  AUDIO: 'audio',
};

// Подтипы
export const AtomSubtype = {
  // text
  STRING: 'string',
  DATE: 'date',
  DATETIME: 'datetime',
  URL: 'url',
  EMAIL: 'email',
  PHONE: 'phone',
  // number
  INT: 'int',
  FLOAT: 'float',
  CURRENCY: 'currency',
  PERCENT: 'percent',
  RATING: 'rating',
  // image
  IMAGE_URL: 'url',
  IMAGE_BASE64: 'base64',
  // icon
  ICON_NAME: 'name',
  ICON_EMOJI: 'emoji',
  ICON_SVG: 'svg',
};

// Display форматы
export const AtomDisplay = {
  // text
  H1: 'h1', H2: 'h2', H3: 'h3', H4: 'h4',
  BODY_LG: 'body-lg', BODY: 'body', BODY_SM: 'body-sm',
  CAPTION: 'caption',
  BADGE: 'badge', BADGE_SUCCESS: 'badge-success', BADGE_ERROR: 'badge-error', BADGE_WARNING: 'badge-warning',
  TAG: 'tag', TAG_ACTIVE: 'tag-active',
  // number
  PRICE: 'price', PRICE_LG: 'price-lg', PRICE_OLD: 'price-old', PRICE_DISCOUNT: 'price-discount',
  RATING: 'rating', RATING_TEXT: 'rating-text', RATING_COMPACT: 'rating-compact',
  PERCENT: 'percent', PROGRESS: 'progress',
  // image
  IMAGE: 'image', IMAGE_COVER: 'image-cover',
  AVATAR: 'avatar', AVATAR_SM: 'avatar-sm', AVATAR_LG: 'avatar-lg',
  THUMBNAIL: 'thumbnail', GALLERY: 'gallery',
  // icon
  ICON: 'icon', ICON_SM: 'icon-sm', ICON_LG: 'icon-lg',
  // interactive
  BUTTON_PRIMARY: 'button-primary', BUTTON_SECONDARY: 'button-secondary',
  BUTTON_OUTLINE: 'button-outline', BUTTON_GHOST: 'button-ghost',
  INPUT: 'input',
  // layout
  DIVIDER: 'divider', SPACER: 'spacer',
};

// Legacy mapping для обратной совместимости
export const LEGACY_TYPE_TO_DISPLAY = {
  'price': 'price',
  'badge': 'badge',
  'rating': 'rating',
  'button': 'button-primary',
  'divider': 'divider',
  'progress': 'progress',
  'selector': 'tag', // selector → tags
};
```

#### 2.2 Обновить AtomRenderer.jsx ✅ DONE

(код как в спеке)

### Phase 3: Frontend — Темы

#### 3.1 ThemeProvider.jsx ✅ DONE

(код как в спеке, разделён на несколько файлов для lint)

#### 3.2 Экспортировать CSS из Pencil (через MCP) ❌ FORGOTTEN → 🔧 FIXED

Используем Pencil MCP для экспорта дизайн-токенов:

1. `mcp__pencil__get_editor_state` — получить текущий файл ✅
2. `mcp__pencil__get_variables` — CSS переменные (цвета, шрифты, радиусы) ✅
3. `mcp__pencil__batch_get({ patterns: [{ reusable: true }] })` — компоненты дизайн-системы ✅
4. `mcp__pencil__get_guidelines(topic: 'code')` — гайдлайны генерации CSS ✅

**Извлечённые токены из Pencil:**
- Colors: `--accent-primary: #8B5CF6`, `--accent-orange: #F97316`, etc.
- Fonts: `Plus Jakarta Sans`, `Inter`
- Radius: 8/12/16/24px

#### 3.3 Интегрировать в App.jsx ✅ DONE

```jsx
import { ThemeProvider } from './shared/theme';
import './shared/theme/themes/marketplace.css';

function App() {
  return (
    <ThemeProvider defaultTheme="marketplace">
      {/* existing app */}
    </ThemeProvider>
  );
}
```

---

## Validation Commands

```bash
# Backend
cd project/backend && go build ./...
cd project/backend && go test ./...

# Frontend
cd project/frontend && npm run build
cd project/frontend && npm run lint
```

---

## Acceptance Criteria

### Backend
- [x] `atom_entity.go` содержит 6 типов + AtomSubtype
- [x] `display_entity.go` содержит AtomDisplay + DisplayStyles
- [x] Пресеты обновлены с display mapping
- [x] `tool_freestyle.go` создан и работает
- [x] `go test ./...` проходит
- [x] `prompt_compose_widgets.go` обновлён — ✅ Done

### Frontend
- [x] `atomModel.js` содержит 6 типов + enums + legacy mapping
- [x] `AtomRenderer.jsx` рендерит по display
- [x] CSS тема "marketplace" экспортирована из Pencil — 🔧 Fixed
- [x] `ThemeProvider` работает
- [x] Обратная совместимость: старые атомы рендерятся
- [x] Widget templates используют AtomRenderer — 🔧 Fixed (не было в спеке!)
- [x] Card/Detail CSS соответствует Pencil — 🔧 Fixed (не было в спеке!)
- [x] `lucide-react` установлен — ✅ Done

### Integration
- [x] Agent2 может использовать preset tool
- [x] Agent2 может использовать freestyle tool с style-алиасом
- [x] Визуально соответствует дизайну в Pencil — 🔧 Fixed

---

## Migration Notes

### Обратная совместимость
Frontend поддерживает legacy типы через `LEGACY_TYPE_TO_DISPLAY`:
- Если бэкенд отправит `type: "price"` — frontend поймёт
- Новый формат `type: "number", subtype: "currency", display: "price"` тоже работает

### Порядок деплоя
1. **Frontend first:** добавить поддержку нового формата + legacy fallback
2. **Backend second:** начать отправлять новый формат
3. **Cleanup:** убрать legacy код когда всё стабильно

---

## Dependencies

```bash
# Frontend
npm install lucide-react  # ✅ INSTALLED
```

---

## Related

- Концептуальная спека: `ADW/specs/DESIGN_SYSTEM_SPEC.md`
- Pencil MCP: используется для экспорта CSS из дизайн-файла (автоматически доступен)
