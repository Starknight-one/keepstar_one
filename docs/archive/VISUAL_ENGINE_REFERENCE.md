# Visual Assembly Engine — Полный справочник

> Этот документ описывает актуальное состояние движка.
> Обновлён: 2026-02-26. Перед работой перечитай исходные файлы — детали могли измениться.

---

## 1. Атом — базовый элемент

Каждый атом это: **type** + **subtype** + **format** + **display** + **slot** + **value** + **meta**

### 1.1 Типы и подтипы (type → subtype)

| Type | Subtypes | Описание |
|------|----------|----------|
| text | string, date, datetime, url, email, phone | Текстовые данные |
| number | int, float, currency, percent, rating | Числовые данные |
| image | url, base64 | Изображения |
| icon | name, emoji, svg | Иконки |
| video | — | Видео (не реализован на фронте) |
| audio | — | Аудио (не реализован на фронте) |

### 1.2 Format (трансформация значения)

Format отвечает за то, КАК отображается число/текст. Автоматически выводится из type+subtype, можно переопределить.

| Format | Что делает | Авто для type+subtype |
|--------|-----------|----------------------|
| currency | `₽1 234.56` (символ + locale) | number/currency |
| stars | `★★★★☆` (полные/пустые) | — |
| stars-text | `4.5/5` | — |
| stars-compact | `★ 4.5` | number/rating |
| percent | `42%` | number/percent |
| number | `1 234` (locale) | number/int, number/float |
| date | `25.02.2026` (locale) | text/date, text/datetime |
| text | Как есть | text/string (и всё остальное) |

**Format работает НЕЗАВИСИМО от display.** Можно: `format: "currency"` + `display: "h1"` → цена крупным заголовком. Это корректно.

### 1.3 Display (визуальная обёртка)

Display отвечает за то, В ЧЁМ отображается значение. CSS-стиль контейнера.

#### Текстовые обёртки (работают на text и number)

| Display | Что это визуально | CSS |
|---------|-------------------|-----|
| h1 | Крупный заголовок | 34px, 700, Plus Jakarta Sans |
| h2 | Заголовок | 28px, 700 |
| h3 | Подзаголовок | 22px, 700 |
| h4 | Мелкий заголовок | 18px, 600 |
| body-lg | Крупный текст | 18px, Inter |
| body | Обычный текст | 15px, Inter |
| body-sm | Мелкий текст | 13px, серый цвет |
| caption | Подпись | 11px, совсем серый |

#### Плашки (работают на text и number, есть фон)

| Display | Визуально | Цвет фона по дефолту |
|---------|-----------|---------------------|
| badge | Бейдж (капс, маленький) | красный (#EF4444) |
| badge-success | Бейдж | зелёный (#22C55E) |
| badge-error | Бейдж | красный |
| badge-warning | Бейдж | оранжевый (#F97316) |
| badge-new | Бейдж | фиолетовый (#8B5CF6) |
| tag | Тег (крупнее бейджа) | серый (#F4F4F5) |
| tag-active | Активный тег | фиолетовый (#8B5CF6) |

#### Ценовые (специализированный шрифт)

| Display | Визуально | CSS |
|---------|-----------|-----|
| price | Цена стандарт | 20px, 700, Plus Jakarta Sans |
| price-lg | Крупная цена | 28px, 800 |
| price-old | Старая цена | 14px, зачёркнуто, серый |
| price-discount | Скидочная цена | 16px, 700, красный |

#### Рейтинг

| Display | Визуально |
|---------|-----------|
| rating | Звёзды оранжевые 18px |
| rating-text | Текст рейтинга 14px |
| rating-compact | Компактный 13px |

#### Кнопки (кликабельные, есть фон)

| Display | Визуально |
|---------|-----------|
| button-primary | Фиолетовая кнопка |
| button-secondary | Серая кнопка |
| button-outline | Обводка |
| button-ghost | Прозрачная |

#### Изображения (ТОЛЬКО для type=image)

| Display | Визуально |
|---------|-----------|
| image | 100% ширина, max-height 300px |
| image-cover | 100% ширина, 180px высота, cover |
| thumbnail | 80×80px |
| avatar | 40×40px, круглый |
| avatar-sm | 32×32px |
| avatar-lg | 56×56px |
| gallery | Горизонтальный скролл |

#### Иконки (ТОЛЬКО для type=icon)

| Display | Визуально |
|---------|-----------|
| icon | 18px |
| icon-sm | 16px |
| icon-lg | 24px |

#### Разделители

| Display | Визуально |
|---------|-----------|
| divider | Горизонтальная линия |
| spacer | Пустое пространство 16px |
| progress | Полоска прогресса (value в %) |
| percent | Текст с % |

### 1.4 Slot (позиция в виджете)

| Slot | Назначение | Макс атомов |
|------|-----------|-------------|
| hero | Главное изображение | 1 |
| title | Заголовок | 1 |
| price | Цена | 2 |
| badge | Бейджи-наложения | 3 |
| primary | Основные атрибуты | 4 |
| secondary | Вторичные атрибуты | 5 |

> **ВАЖНО**: Сейчас слоты используются как метаданные для constraints, но GenericCardTemplate их **игнорирует** — рендерит всё подряд в колонку. Это главная проблема раскладки.

---

## 2. Параметры visual_assembly tool

### 2.1 Данные

| Параметр | Тип | Что делает |
|----------|-----|-----------|
| show | string[] | Добавить поля (к дефолтным) |
| hide | string[] | Убрать поля |
| order | string[] | Порядок полей (остальные дописываются) |

### 2.2 Стиль

| Параметр | Тип | Что делает | Где видно эффект |
|----------|-----|-----------|-----------------|
| display | {field: display} | Обёртка атома | Везде (но валидируется по типу) |
| format | {field: format} | Трансформация значения | На text/number (не на image/icon) |
| color | {field: color} | Цвет | На badge/tag → фон. На text/price → цвет текста. **НЕ на rating** (баг) |
| shape | {field: shape} | Форма | **Только на badge/tag/button/image** (на text/price/heading — невидим, нет фона) |
| size | string или {field: size} | Размер | Строка: размер виджета. Объект: per-atom масштаб (sm/md/lg/xl) |

### 2.3 Позиционирование

| Параметр | Тип | Что делает |
|----------|-----|-----------|
| layer | {field: "1"} | z-index (только с position: relative) |
| anchor | {field: position} | Абсолютное позиционирование: top-left, top-right, bottom-left, bottom-right, center |
| place | string | Режим виджета: sticky (верх), floating (низ-право), default |
| direction | string | vertical (дефолт) или horizontal (картинка слева) |

### 2.4 Лейаут

| Параметр | Тип | Что делает |
|----------|-----|-----------|
| layout | string | grid, list, single, carousel, comparison, table |
| preset | string | Загрузить пресет как базу |
| compose | object[] | Мульти-секция: [{mode, show, hide, count, label}] |
| conditional | object[] | Условные стили: [{field, op, value, display, color}] |
| limit | number | Пагинация: макс виджетов (дефолт 50) |
| offset | number | Пагинация: смещение (дефолт 0) |

### 2.5 Матрица shape × display (где shape визуально работает)

```
             badge  tag  button  image  price  heading  body  rating
pill          ✓      ✓     ✓      -      ✗       ✗      ✗      ✗
rounded       ✓      ✓     ✓      ✓      ✗       ✗      ✗      ✗
square        ✓      ✓     ✓      ✓      ✗       ✗      ✗      ✗
circle        -      -     ✓      ✓      ✗       ✗      ✗      ✗
```
✓ = виден эффект, ✗ = бэкенд принимает но эффект невидим, - = нет смысла

### 2.6 Матрица color × display (как применяется цвет)

```
             badge/tag    heading/body/caption    price    rating    button    image
Задан →      Фон+авто     Цвет текста            Текст    НЕТ(баг)  Нет       Нет
             контраст
```

---

## 3. Constraints (что система делает автоматически)

### 3.1 Санитизация данных (Level 0)

| ID | Правило | Срабатывание |
|----|---------|-------------|
| D5 | Обрезка текста | title: 100 сим, primary: 60, secondary/grid: 120, secondary/list: 200, остальное: 300 |
| D6 | Понижение заголовка | h1 + >60 символов → h2. h2 + >80 → h3 |
| D7 | Валидация URL картинок | Нет http/https → картинка убирается |

### 3.2 Per-Atom (Level 1)

| ID | Правило | Срабатывание |
|----|---------|-------------|
| A1 | Badge длинный | >20 символов → становится tag |
| A2 | Tag длинный | >40 символов → становится body-sm |
| A4 | Badge capitalize | Первая буква → заглавная |
| A5 | Низкий рейтинг | <3.0 → принудительно stars-compact + rating-compact |

### 3.3 Per-Widget (Level 2)

| ID | Правило | Результат |
|----|---------|-----------|
| W1 | Больше 2 badge | 3-й+ → становится tag |
| W2 | Больше 5 tag | 6-й+ → value=nil → исчезает |
| W4 | Больше 1 заголовка h1/h2 | 2-й → h3 |
| W7 | Horizontal + >4 атома | → принудительно vertical |
| W8 | Size=tiny | Все image атомы удаляются |

### 3.4 Cross-Widget (Level 4, только grid/list)

| ID | Правило | Результат |
|----|---------|-----------|
| C1 | Поле в <70% виджетов | Редкое поле убирается. Для частого поля добавляется placeholder "—" |

### 3.5 Size → Max Atoms

| Size | Макс атомов | Поведение |
|------|-------------|-----------|
| tiny | 2 | Режется до 2 (обычно: name + price) |
| small | 3 | Режется до 3 (обычно: image + name + price) |
| medium | 5 | Режется до 5 |
| large | 10 | Режется до 10 |
| xl | — | Без лимита |

> **ВАЖНО**: Если задан show или hide — MaxAtomsPerSize **отключается**. Считается что ты явно выбрал поля.

---

## 4. AutoResolve (дефолты по количеству товаров)

| Товаров | Layout | Size | Показываемых полей |
|---------|--------|------|-------------------|
| 1 | single | large | все (до 13) |
| 2-6 | grid | medium | 5 (images, name, price, rating, brand) |
| 7-12 | list | small | 4 (images, name, price, rating) |
| 13+ | grid | small | 3 (images, name, price) |

---

## 5. Доступные поля

### Product

| Поле | Type | Default display | Default slot | Приоритет |
|------|------|----------------|-------------|-----------|
| images | image | image-cover | hero | 0 |
| name | text | h2 | title | 1 |
| price | number/currency | price | price | 2 |
| rating | number/rating | rating-compact | primary | 3 |
| brand | text | tag | primary | 4 |
| category | text | tag | primary | 5 |
| description | text | body-sm | secondary | 6 |
| tags | text | tag | secondary | 7 |
| stockQuantity | number/int | body-sm | secondary | 8 |
| productForm | text | tag | secondary | 9 |
| skinType | text | tag | secondary | 10 |
| concern | text | tag | secondary | 11 |
| keyIngredients | text | body-sm | secondary | 12 |

### Service

| Поле | Type | Default display | Default slot |
|------|------|----------------|-------------|
| images | image | image-cover | hero |
| name | text | h2 | title |
| price | number/currency | price | price |
| rating | number/rating | rating-compact | primary |
| duration | text | body | primary |
| provider | text | body | primary |
| availability | text | body | primary |
| description | text | body-sm | secondary |
| attributes | text | body-sm | secondary |

---

## 6. Пресеты

### Visual Assembly пресеты (для visual_assembly tool)

| Пресет | Layout | Size | Полей | Поля |
|--------|--------|------|-------|------|
| product_card_grid | grid | medium | 3 | images, name, price |
| product_card_detail | single | large | 9 | images, name, brand, category, price, rating, description, tags, stockQuantity |
| product_row | list | small | 5 | images(thumb), name(h3), brand(body-sm), price, rating(compact) |
| product_single_hero | single | large | 10 | images, name(h1), brand(badge), category(tag), price(lg), rating, description(lg), tags, productForm, skinType |
| search_empty | single | medium | 2 | name(h3), description |
| category_overview | grid | medium | 3 | images, category(h2), name(body-sm) |
| attribute_picker | grid | small | 1 | name(tag) |
| cart_summary | list | small | 4 | images(thumb), name(h4), price, stockQuantity(caption) |
| info_card | single | medium | 2 | name(h3), description |

### Legacy пресеты (для render_product_preset tool)

| Пресет | Template | Layout |
|--------|----------|--------|
| product_grid | ProductCard | grid |
| product_card | ProductCard | grid |
| product_compact | ProductCard | list |
| product_detail | ProductDetail | single |
| product_comparison | Comparison | comparison |
| service_card | ServiceCard | grid |
| service_list | ServiceCard | list |
| service_detail | ServiceDetail | single |

---

## 7. Известные проблемы

### Фронтенд: раскладка внутри карточки

**Главная проблема**: GenericCardTemplate рендерит все non-image атомы в `flex-direction: column`, каждый атом как отдельный flex-item. Нет группировки по слотам.

Результат:
- Теги (brand, category, tags, skinType) — **каждый на отдельной строке** вместо inline wrap
- Цена и рейтинг — **друг под другом** вместо рядом в строку
- Бейджи — **стопкой** вместо row
- Нет fold/collapse для длинных списков

**Что нужно**: GenericCardTemplate должен группировать атомы по slot и рендерить каждую группу соответствующим CSS-layout:
- hero → full-width image
- title → отдельная строка
- price → row (цена + старая цена рядом)
- primary → flow (tags/badges inline-wrap)
- secondary → stack или flow в зависимости от display

### shape на text/number

`shape: "pill"` на price/heading/body/rating — **невидим**. Бэкенд принимает молча. Нужно: либо валидация, либо warning в testbench.

### color на rating

Цвет задаётся в `atom.meta.color`, но `AtomRenderer.jsx:188` рендерит rating без `style` — цвет не применяется.

### MaxAtomsPerSize слишком грубый

Переход не плавный. 5 полей при medium → ок. 6 полей при medium → бэкенд молча режет до 5 без предупреждения. Нет промежуточных размеров или fold.

### Comparison mode

Бэкенд отдаёт `mode: "comparison"`, но FormationRenderer может не маршрутизировать его корректно.
