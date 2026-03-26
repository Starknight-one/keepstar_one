# Updates 2

Лог изменений проекта Keepstar (продолжение).

---

## Alpha 0.3.0 — Design System Migration + Comparison Redesign — 2026-03-26

### Design System: Pencil → Code (Landing Style)

Перенос дизайн-системы из Pencil в чат-виджет. Новый визуальный стиль, унифицированный с лендингом.

**Primary Color Migration**
- `--color-primary`: `#8B5CF6` (фиолетовый) → `#18181B` (чёрный)
- Все зависимые переменные обновлены: `--color-primary-hover`, `--color-primary-light`, `--color-primary-dark`, `--color-border-focus`
- Добавлен `--color-accent-blue: #3B82F6` для stepper и assistant bubble
- Все CSS fallback-значения `#8B5CF6` вычищены из 11 файлов (CSS + JSX)

**Component Style Updates**
- **Buttons**: primary → чёрный, ghost → серый текст (был фиолетовый)
- **Badges**: `badge-new` → пастельный голубой (#DBEAFE), `badge-error` → пастельный розовый (#FFE4E6)
- **Tags**: белый фон + серая рамка (был серый фон, без рамки)
- **Typography**: H3 → 20px/600 (был 22px/700)
- **Assistant bubble**: голубой фон `rgba(59,130,246,0.08)` + скруглённые углы 16px (был прозрачный, без скруглений)
- **Stepper active dot**: синий `#3B82F6` (был серый)

**Затронутые файлы**
- `project/frontend/src/shared/theme/themes/marketplace.css`
- `project/frontend/src/entities/atom/Atom.css`
- `project/frontend/src/entities/atom/AtomV2.css`
- `project/frontend/src/entities/atom/AtomRenderer.jsx`
- `project/frontend/src/entities/atom/AtomV2Renderer.jsx`
- `project/frontend/src/features/chat/ChatPanel.css`
- `project/frontend/src/features/navigation/Stepper.css`
- `project/frontend/src/features/actions/ActionContext.css`
- `project/frontend/src/entities/widget/templates/ProductCardTemplate.css`
- `project/frontend/src/entities/widget/templates/ProductDetailTemplate.css`
- `project/frontend/src/entities/widget/templates/ServiceCardTemplate.css`
- `project/frontend/src/entities/widget/templates/ServiceDetailTemplate.css`

---

### Comparison Table — Full Redesign

Сравнительная таблица переписана из плоской CSS Grid в pricing-table стиль (по дизайну из Pencil).

**Новый layout**
- Колоночная структура (flex columns) вместо row-first grid
- Column header: фото 80×80 + название + бренд
- Featured column: тёмный фон `#0F172A`, белый текст, badge "Popular" (определяется по лучшему рейтингу)
- CTA кнопки "Подробнее" внизу каждой колонки → triggers expand navigation
- Boolean values: ✓/✗ иконки вместо текста
- Тонкие горизонтальные разделители вместо alternating backgrounds
- Responsive: адаптивные размеры на <600px

**V2 Engine Support**
- Добавлен V2 preset `product_comparison` в `PresetV2Registry` (backend)
- `ComparisonTemplate.jsx` поддерживает оба формата: V1 (`atoms` → `AtomRenderer`) и V2 (`atomsV2` → `AtomV2Renderer`)
- Автоматическое определение версии по наличию `widget.atomsV2`

**Затронутые файлы**
- `project/frontend/src/entities/widget/templates/ComparisonTemplate.jsx` — полная переписка
- `project/frontend/src/entities/widget/templates/ComparisonTemplate.css` — полная переписка
- `project/backend/internal/presets/preset_v2.go` — новый preset `product_comparison`

---

### Pencil Design System — Что не перенесено (TODO)

Компоненты которые есть в коде, но отсутствуют в Pencil дизайн-системе:

1. Overlay/Backdrop (полноэкранный blur layout)
2. Cart View (список товаров, ±, итого)
3. Liked Items View (избранное)
4. Back Button (навигация)
5. Loading/Skeleton States (спиннер, скелетоны, typing indicator)
6. Error States (сообщения об ошибках)
7. Empty States ("ничего не найдено", пустая корзина)
8. Image Carousel (точки навигации на карточках)
9. "Показать ещё" (fold/unfold кнопка)
10. Status Pill ("12 результатов")
11. Like + Cart кнопки на карточках (overlay)
12. Selector (выбор размера/цвета)
13. Formation layouts (carousel, single mode)
