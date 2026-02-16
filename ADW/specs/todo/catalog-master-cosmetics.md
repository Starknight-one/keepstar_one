# Мастер-каталог косметики — от краула до поиска

**Статус:** Todo
**Приоритет:** High (блокирует пилот)

## Контекст

Есть краулер `project_admin/backend/cmd/crawler/` который вытягивает товары с heybabescosmetics.com в формате `ImportRequest` JSON. Текущий результат: 967 товаров, 62 бренда, 7 структурированных атрибутов (description, ingredients, how_to_use, volume, skin_type, benefits, active_ingredients). Всё — сырой текст.

Мультитенантная БД уже поддерживает `master_products` (shared) → `products` (per-tenant), категории, JSONB attributes, vector embeddings.

**Проблема:** сырой текст из краулера нельзя напрямую положить в каталог — не будут работать ни фильтрация, ни нормальный vector search. Нужна нормализация атрибутов по закрытым спискам + правильное дерево категорий.

## Этапы

### Этап 1: Проектирование каталога

Прежде чем нормализовывать данные — нужно определить **куда** их нормализовывать. Без схемы каталога нельзя написать ни enrichment-промпт, ни import pipeline.

**1.1 Дерево категорий (cosmetics-specific)**

3 уровня макс. Один товар = одна категория. Категории глобальные (shared across tenants).

```
Уход за лицом
  ├── Очищение (пенки, гидрофильные масла, бальзамы, мицеллярная вода, мыло)
  ├── Тонизирование (тонеры, тонер-пэды, мисты)
  ├── Эксфолиация (пилинг-пэды, пилинги, скрабы, энзимные пудры)
  ├── Сыворотки и ампулы
  ├── Увлажнение (кремы, эмульсии, гели, крем для глаз, стики/бальзамы)
  ├── Солнцезащита (кремы, спреи, стики, кушоны)
  ├── Маски (тканевые, смываемые, ночные, патчи, альгинатные, гидрогелевые)
  ├── Точечные средства
  └── Эссенции
Декоративная косметика
  ├── Для лица (тон, консилер, кушон, пудра, румяна, хайлайтер)
  ├── Для глаз (тени, тушь, подводка, карандаш для бровей)
  ├── Для губ (помада, тинт, блеск, бальзам, скраб)
  └── Фиксаторы макияжа
Тело
  ├── Очищение (гели, мыло, скрабы)
  ├── Увлажнение (кремы, масла, гели)
  └── Парфюм
Волосы
  ├── Шампуни
  ├── Кондиционеры
  └── Уход (маски, филлеры, пилинги, несмываемый уход)
```

Не категории: "Подборки", "SALE", "Спецпредложения", "Наборы", "Новинки", "Бестселлеры" — это теги на tenant-уровне.

**1.2 Схема атрибутов (JSONB в master_products.attributes)**

Фасетные (enum, для фильтрации и поиска):

| Ключ | Тип | Enum-значения | Откуда |
|---|---|---|---|
| `product_form` | string | cream, serum, toner, foam, gel, oil, balm, mask_sheet, mask_wash, pads, spray, mist, powder, stick, emulsion, essence, ampoule, cleanser_oil, scrub, patch | LLM из name + description |
| `skin_type` | string[] | all, oily, dry, combination, sensitive, mature, problem, normal, dehydrated | LLM из skin_type текста |
| `concern` | string[] | acne, pigmentation, aging, dehydration, redness, pores, wrinkles, dullness, sensitivity, elasticity, dark_circles, blackheads, sun_protection, barrier_repair | LLM из description + benefits |
| `key_ingredients` | string[] | niacinamide, retinol, hyaluronic_acid, bha, aha, pha, vitamin_c, centella, snail_mucin, propolis, ceramide, peptides, collagen, tea_tree, mugwort, rice, green_tea, panthenol, arbutin, glutathione, squalane, bakuchiol, galactomyces, beta_glucan | LLM из INCI + active_ingredients |
| `volume` | string | "150 мл" | Краулер (уже есть) |

Текстовые (для vector search, не для фильтрации):

| Ключ | Тип | Откуда |
|---|---|---|
| `description` | string | Краулер (уже есть) |
| `benefits` | string | Краулер (уже есть) |
| `active_ingredients` | string | Краулер (уже есть) |
| `how_to_use` | string | Краулер (уже есть) |
| `ingredients` | string (INCI) | Краулер (уже есть) |

Визуальные (будущее, из картинок через vision):

| Ключ | Тип | Описание |
|---|---|---|
| `packaging` | string | tube, jar, bottle, spray, dropper, pump, sachet, cushion, stick, tube_squeeze |
| `packaging_color` | string | основной цвет упаковки |
| `visual_description` | string | "прозрачный флакон с дозатором, розовая жидкость" |

**1.3 Маппинг категорий из краулера**

Таблица соответствия: 30 категорий из краулера → наше дерево. Ручная, один раз.

### Этап 2: Создание каталога в БД

- Seed дерева категорий в `catalog.categories` (с parent_id)
- Создание тенанта (slug, name, type=retailer)
- Создание admin user для тенанта

### Этап 3: Enrichment agent

Go-скрипт (или расширение краулера) который прогоняет crawl JSON через LLM:

**Input:** crawl JSON (967 товаров)
**Output:** enriched JSON с нормализованными атрибутами

Для каждого товара LLM получает:
```
name, description, INCI, active_ingredients, skin_type_text, benefits
```

LLM возвращает structured JSON:
```json
{
  "category_slug": "face-care/toning/toners",
  "product_form": "toner",
  "skin_type": ["oily", "combination"],
  "concern": ["acne", "pores"],
  "key_ingredients": ["bha", "tea_tree", "centella"]
}
```

Промпт содержит полные enum-списки и дерево категорий. LLM выбирает только из закрытых списков.

**Оценка:** 967 товаров × Haiku = ~5 мин, ~$0.50.

Визуальные атрибуты (packaging, color) — отдельный проход с vision моделью по картинкам. Позже.

### Этап 4: Import в БД

Прокачка существующего import pipeline в админке:

1. Enriched JSON → `POST /admin/api/catalog/import`
2. Import pipeline маппит `category_slug` → `category_id`
3. Upsert master_products с нормализованными attributes
4. Upsert tenant product listings (price, stock)
5. Post-import: generate embeddings, rebuild catalog digest

Embedding text для vector search собирается из:
```
name + description + brand + category_name + product_form + skin_type + concern + key_ingredients + benefits
```

Это даёт семантический поиск по: "увлажняющий крем для сухой кожи с гиалуроновой кислотой" → правильно матчит товары.

### Будущее: Отзывы

Отзывы — отдельная сущность, не атрибут товара.

**Таблица** `catalog.reviews` (product_id, author, rating, text, date).

**Для vector search** — два подхода (нужно исследовать):
- **Дайджест в embedding**: сгенерировать LLM-саммари отзывов ("пользователи отмечают: быстро впитывается, хорошо от акне") → подмешать в embedding text master_product
- **Отдельный vector index**: embeddings на отзывах, отдельный поиск "по опыту пользователей"

Отзывы на heybabes загружаются динамически (Bitrix composite cache), обычным HTTP не достать — нужен headless browser или API endpoint. Отложено.

## Зависимости

- Краулер: **готов** (967 товаров, 7 атрибутов)
- БД схема: **готова** (master_products, products, categories, stock, embeddings)
- Import pipeline: **готов** (POST /admin/api/catalog/import)
- LLM enrichment: **нужно написать**
- Категории seed: **нужно написать**
- Tenant creation: **нужно добавить** (или через API signup)

## Что НЕ входит

- Полный краул всех 5000 товаров (пока 1000 достаточно)
- Визуальные атрибуты из картинок (отдельная задача)
- Парсинг отзывов из Bitrix (отдельная задача)
- Подборки/коллекции с сайта как source для тегов (отдельная задача)
- Другие сайты/источники данных
