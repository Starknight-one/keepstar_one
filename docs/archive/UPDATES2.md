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

### Pencil Design System — Что перенесено (Alpha 0.4.0)

**Новые shared/ui компоненты** (9 шт, `project/frontend/src/shared/ui/`):
- Spinner — 32px conic-gradient с mask
- TypingIndicator — 3 dots с staggered pulse в голубом bubble
- SkeletonCard — placeholder карточка с shimmer анимацией
- ErrorState — карточка с розовой рамкой, иконка, кнопка Retry
- EmptyState — конфигурируемый: icon (search-x/shopping-bag/heart) + title + desc + action
- StatusPill — голубой pill с иконкой поиска + count
- SelectorGroup — pill-кнопки (active=black, inactive=border, disabled=0.4)
- ColorSelector — 24px цветные кружки с blue box-shadow при выборе

**Стилевое выравнивание с Pencil** (11 компонентов):
- BackButton → белая пилюля + shadow + SVG стрелка
- ActionToolbar → pill shape + border-radius + shadow
- LikeButton → 36px, glass bg (backdrop-filter: blur)
- AddToCartButton → 36px circle с плюсиком на картинке (вместо full-width текстовой кнопки)
- CartView/LikedView → EmptyState компонент (вместо emoji)
- ChatHistory → TypingIndicator (вместо "Thinking...")
- ChatPanel error → styled карточка с розовой рамкой
- Show More → chevron icon + синий accent
- AtomV2 wrappers → badge 6/12, tag 4/10, pill 6/14, button 12/24
- Stepper → CSS variable tokens

**Интеграция**:
- StatusPill в FormationRenderer (количество товаров)
- Spinner в ChatInput (при отправке)

**Cleanup**:
- `widget.css` — font-family + color → CSS variables
- `ChatPanel.css` — font-family → CSS variables

---

### Landing Page — keepstar.one (Alpha 0.2.0 → 0.4.0)

Лендинг значительно прокачан и опубликован на **keepstar.one**.

**Страницы и навигация**
- 6 новых страниц: Terms of Use, Privacy Policy, Contact, About, Features, Changelog
- Footer redesign: 4 колонки (Product, Company, Legal) + social icons
- Все CTA кнопки привязаны к Demo Modal (lead capture)
- Smooth scroll + единый "Request live demo" CTA в hero
- React Router навигация между всеми страницами

**Визуальные улучшения**
- Confetti-анимации на всех секциях (reusable Confetti component с seeded генерацией)
- Фавикон: кастомный круглый логотип (32/180/192/512px)
- Infostyle copy audit: абстракции → конкретные сценарии, убраны нечестные статы
- Pricing: новый CSS + redesign
- UseCases: новая страница + компонент

**Инфраструктура**
- Подключение лендинга к admin API + seed blog posts
- Express 5 wildcard route fix
- better-sqlite3 build tools в admin Dockerfile

---

### TODO — Следующая сессия

1. **Прогон чат-кейсов**: пройти основные сценарии чата end-to-end, выявить и зафиксировать проблемы (поиск, comparison, expand/back, carousel, empty/error states)
2. **Фикс найденных проблем**: устранить баги и визуальные недочёты, выявленные при прогоне кейсов
3. **Пресеты из Pencil**: перенести визуальные пресеты из Pencil в `PresetV2Registry` (backend) — маппинг полей, слоты, размеры
4. **Formation layouts**: carousel и single mode — проверить соответствие Pencil
5. **Overlay/Backdrop**: полноэкранный blur layout (есть в Pencil, нет в коде)
6. **ProductGrid.css**: устаревшие Material Design цвета (#1976d2) → заменить на design tokens

---

### Git

**Alpha 0.3.0** — ветка `feature/alpha-0.3.0-design-system`
- Коммит `10127cd` — Design System Migration + Comparison Redesign

**Alpha 0.4.0** — merged в `main`
- Pencil → Code: 9 новых компонентов, 16 изменённых файлов, cleanup CSS tokens
