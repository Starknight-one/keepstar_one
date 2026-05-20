# Tenant A — K-beauty Cosmetics — Seed Schema

**Profile:** pure cosmetics merchant, K-beauty focus, Russian-language CSV inbox,
~1,000 SKUs target. Used to calibrate discovery agent against a realistic but
controlled cosmetics catalog.

**Inbox format:** single CSV file (UTF-8, comma-separated, BOM present —
typical Excel export from Russian merchant). Variants on separate rows.
Extras packed into JSON blob column. Some attributes intentionally missing
or noisy — see Noise profile below.

---

## 1. Master attribute schema (42 fields)

| # | Master field | Type | Example | Source CSV column (Russian merchant style) | Notes |
|---|---|---|---|---|---|
| 1 | `sku` | string | `COSRX-AS96-100` | `Артикул` | 0% missing |
| 2 | `barcode` | string | `8809416470016` | `Штрихкод` | 10% missing |
| 3 | `brand` | string | `COSRX` | `Бренд` | 0% missing |
| 4 | `line` | string | `Advanced Snail` | `Линейка` | 30% missing (часто часть `name`) |
| 5 | `name` | string | `COSRX Advanced Snail 96 Mucin Power Essence` | `Название` | 0% missing |
| 6 | `name_ru` | string | `COSRX Эссенция с муцином улитки 96%` | `Название (рус)` | 25% missing |
| 7 | `category` | string | `Уход за лицом` | `Категория` | 5% missing |
| 8 | `subcategory` | string | `Сыворотки и эссенции` | `Подкатегория` | 15% missing |
| 9 | `form` | string | `Эссенция` | `Тип` | 10% missing |
| 10 | `volume` | number | `100` | `Объём` (см. ниже — может слиться в текст) | 5% missing |
| 11 | `volume_unit` | string | `мл` | (вместе с volume или отдельно) | mixed `мл`/`ml`/`ML` |
| 12 | `ingredients` | text | `Snail Secretion Filtrate, Betaine, Butylene Glycol...` | `Состав` | 15% missing, длинный INCI |
| 13 | `key_ingredients` | string[] | `Snail Mucin\|Niacinamide\|Hyaluronic Acid` | `Активы` | 25% missing, pipe-separated |
| 14 | `skin_type` | string[] | `сухая\|чувствительная` | `Тип кожи` | 20% missing, comma/pipe mix |
| 15 | `skin_concerns` | string[] | `обезвоженность\|следы постакне` | `Проблемы кожи` | 30% missing |
| 16 | `age_range` | string | `25+` | `Возраст` | 40% missing |
| 17 | `application_zone` | string | `Лицо` | (в `additional_info` JSON) | 20% missing |
| 18 | `usage_frequency` | string | `Ежедневно, утром и вечером` | (в `additional_info` JSON) | 40% missing |
| 19 | `usage_instructions` | text | `Наносить после тонера, перед кремом...` | `Применение` | 20% missing |
| 20 | `description_short` | text | `Эссенция с 96% муцина улитки...` | `Описание` | 5% missing |
| 21 | `description_long` | text | (длинный маркетинговый текст) | `Подробное описание` | 25% missing |
| 22 | `key_benefits` | string[] | `Увлажнение\|Заживление\|Регенерация` | (в `additional_info` JSON) | 30% missing |
| 23 | `country_of_origin` | string | `Южная Корея` | `Страна` | 5% missing |
| 24 | `manufacturer` | string | `COSRX Co., Ltd.` | `Производитель` | 30% missing |
| 25 | `vegan` | bool | `да` | `Веган` | 50% missing, format mixed (`да`/`нет`/`1`/`0`/`true`) |
| 26 | `cruelty_free` | bool | `да` | `Без жестокости` | 50% missing |
| 27 | `halal` | bool | `нет` | (в `additional_info` JSON) | 80% missing |
| 28 | `certifications` | string[] | `EWG Verified\|COSMOS` | (в `additional_info` JSON) | 75% missing |
| 29 | `ph` | number | `5.5` | (в `additional_info` JSON) | 60% missing |
| 30 | `fragrance` | string | `Без отдушек` | (в `additional_info` JSON) | 50% missing |
| 31 | `pack_type` | string | `Флакон с пипеткой` | (в `additional_info` JSON) | 40% missing |
| 32 | `expiration_months` | number | `36` | `Срок годности` | 25% missing, mixed `24 мес`/`2 года`/`36` |
| 33 | `weight` | number (g) | `120` | `Вес` | 35% missing, mixed `120 г`/`120 g`/`120` |
| 34 | `dimensions` | string | `4×4×12 см` | `Габариты` | 60% missing |
| 35 | `price` | number | `1990` | `Цена` | 0% missing, mixed `1990`/`1 990`/`1990.00`/`1990,00`/`1990 ₽` |
| 36 | `price_compare` | number | `2490` | `Старая цена` | 70% missing |
| 37 | `currency` | string | `RUB` | `Валюта` | 60% missing (default RUB) |
| 38 | `in_stock` | bool | `да` | `В наличии` | 0% missing |
| 39 | `stock_qty` | number | `42` | `Количество` | 15% missing |
| 40 | `images` | string[] | `https://cdn.shopify.com/.../cosrx_essence.jpg\|...` | `Фото` | 10% missing, pipe-separated, mixed `.jpg/.png/.webp` |
| 41 | `variant_group_id` | string | `COSRX-AS96` | `parent_sku` | empty для родителя, заполнен у вариантов |
| 42 | `tags` | string[] | `новинка\|хит\|sale` | `Теги` | 50% missing |

---

## 2. Categories (taxonomy для тенанта A)

```
Уход за лицом
├── Очищение (гели, пенки, гидрофильные масла, бальзамы)
├── Тонирование (тонеры, эссенции-тонеры)
├── Сыворотки и эссенции
├── Кремы (дневные, ночные, увлажняющие, питательные)
├── Маски (тканевые, ночные, глиняные, патчи)
├── Солнцезащита
├── Уход за глазами
└── Уход за губами

Уход за телом
├── Гели для душа
├── Лосьоны и кремы для тела
└── Скрабы и пилинги

Уход за волосами
├── Шампуни
├── Маски и кондиционеры
└── Сыворотки и масла

Декоративная косметика
├── Тон (BB/CC, тональные, кушоны)
├── Помады и тинты
├── Тени и палетки
├── Туши и подводки
└── Хайлайтеры и румяна
```

Целевое распределение по узлам: ~70% `Уход за лицом`, ~10% `Уход за телом`,
~5% `Уход за волосами`, ~15% `Декоративная косметика`.

---

## 3. Brand anchors (real public K-beauty brands)

30 брендов, по 2-5 линеек каждый. Бренды реальные и общеизвестные, конкретные
карточки не копируем — атрибуты LLM-генерация.

```
COSRX → Advanced Snail / Acne Pimple Master / Low pH Good Morning / The Vitamin C
Innisfree → Green Tea Seed / Bija Trouble / Jeju Cherry Blossom / Volcanic Pore
Etude House → SoonJung / Moistfull Collagen / SunPro / Play Color
Beauty of Joseon → Glow Serum / Relief Sun / Dynasty Cream / Calming Serum
Some By Mi → AHA-BHA-PHA 30 Days / Yuja Niacin / Snail Truecica / Retinol Intense
Laneige → Water Bank / Lip Sleeping Mask / Cream Skin / Perfect Renew
Sulwhasoo → First Care Activating / Concentrated Ginseng / Snowise / Timetreasure
Missha → Time Revolution / M Perfect Cover / Near Skin / Geum Sul
TonyMoly → I'm Real / Egg Pore / Wonder / Floria
Klairs → Rich Moist / Supple Preparation / Freshly Juiced Vitamin / Midnight Blue
Pyunkang Yul → Essence Toner / Moisture Cream / ATO / Highly Concentrated
Round Lab → 1025 Dokdo / Birch Juice / Mugwort Calming / Pine Calming
Anua → Heartleaf 77 / Peach 70 / Niacinamide 10 / Birch Sap
Skin1004 → Madagascar Centella / Tea-trica / Hyalu-Cica / Probio-Cica
Numbuzin → No.3 Skin Softening / No.5 Vitamin / No.4 Collagen / No.1 Easy
Mixsoon → Bean Essence / Centella Asiatica / Galactomyces / Glacier Water
Hanyul → Pure Artemisia / White Chrysanthemum / Red Rice / Yuja
Mediheal → N.M.F Aquaring / Tea Tree Care / Vita Bright / Mugwort Bha
Goodal → Green Tangerine Vita C / Houttuynia Cordata / Heartleaf Calming
Iunik → Beta Glucan / Centella / Rose / Tea Tree
Purito → Centella Green Level / Galacto Niacin 97 / Sea Buckthorn / Daily Go-To
I'm From → Mugwort / Honey / Vitamin Tree / Rice
Abib → Heartleaf Calming / Mild Acidic / Quick Sheet / Gummy Sheet
Torriden → DIVE-IN Hyaluronic / BALANCEFUL / SOLID-IN
Manyo Factory → Galac Niacin / Bifida Biome / Pure Cleansing / Probiotics
Heimish → All Clean Balm / Bulgarian Rose / Marine Care / Matcha Biome
Banila Co → Clean It Zero / Dear Hydration / Miss Flower
Dr. Jart+ → Cicapair / Ceramidin / Vital Hydra / Dermask
Glow Recipe → Watermelon Glow / Plum Plump / Avocado / Strawberry Smooth
Then I Met You → Living Cleansing Balm / Soothing Tea / Honest Renewal
```

---

## 4. Noise profile

**Missing rates:** в таблице атрибутов (колонка Notes). В среднем по карточке
~22% полей пусто — норма для merchant inbox.

**Empty value representations** (per row randomly):
- пустая строка, `null`, `-`, `n/a`, `—`, `нет данных`

**Numeric format variations** (price, volume, weight):
- `1990`, `1 990`, `1990.00`, `1990,00`, `1990 руб`, `1990 ₽`, `1990р`

**Unit variations** (volume): `мл`, `ml`, `ML`, `m.l.`, no unit (just number).
**Unit variations** (weight): `г`, `g`, `гр`, `гр.`, no unit.

**Boolean variations** (vegan, cruelty_free, in_stock):
- `да`/`нет`, `true`/`false`, `1`/`0`, `Y`/`N`, `Yes`/`No`, `+`/`-`

**Typo rate ~2%:**
- Двойные буквы (`Сывороткаа`), пропущенные (`Сыортка`), кириллица/латиница
  смешано (`COSRХ`, `Сyворотка`), хвостовые пробелы.

**Mixed languages within row ~10%:**
- Например `Название = "COSRX Advanced Snail 96"`, остальные поля русские.
  Или наоборот — `Название` русское, а `Применение` — английский маркетинг.

**Duplicate detection traps ~3%:**
- Один и тот же товар с разным регистром бренда (`cosrx` vs `COSRX`),
  разными SKU, разным volume_unit. Discovery должен схлопнуть.

---

## 5. Variants

~25% товаров имеют варианты — обычно по объёму (30 мл / 50 мл / 100 мл).
В CSV варианты на отдельных строках, связаны через `parent_sku` (=
`variant_group_id`). У родителя `parent_sku` пустой, у вариантов — равен SKU
родителя.

**Trap для discovery:** ~5% вариантов с **неконсистентным** `name` между
строками одной группы (например родитель `COSRX Snail 96 Essence`,
вариант `COSRX Эссенция с муцином 50 мл` — то же самое, но другой нейминг).
Агент должен схлопнуть по `parent_sku`.

---

## 6. Images

- ~70% товаров — 1 URL в колонке `Фото`.
- ~20% — 2-4 URL, разделители: `|` (50% случаев), `\n` (30%), `;` (20%).
- ~5% — URL в JSON-массиве внутри `additional_info`.
- ~5% — поле пустое.
- ~5% URL заведомо битые (404, malformed scheme, дубль `https://https://`).

URL — публичные Shopify CDN, реальные бренд-сайты, Unsplash для категорий
без публичного CDN. Расширения mixed: `.jpg`/`.jpeg`/`.png`/`.webp`/без
расширения.

---

## 7. additional_info (JSON blob column)

В колонке `additional_info` сидит JSON с полями: `ph`, `pack_type`,
`fragrance`, `certifications`, `halal`, `application_zone`,
`usage_frequency`, `key_benefits`, `tagline`.

~15% карточек: JSON слегка кривой (висячая запятая, одиночные кавычки,
unescaped кириллица). Discovery должен парсить лояльно.

---

## 8. Sample row preview (1 строка merchant CSV)

```
Артикул,Штрихкод,Бренд,Линейка,Название,Категория,Подкатегория,Тип,Объём,Состав,Тип кожи,Цена,Старая цена,В наличии,Количество,Описание,Применение,Страна,Срок годности,Фото,parent_sku,additional_info
COSRX-AS96-100,8809416470016,COSRX,Advanced Snail,COSRX Advanced Snail 96 Mucin Power Essence 100 мл,Уход за лицом,Сыворотки и эссенции,Эссенция,100 мл,"Snail Secretion Filtrate (96%), Betaine, Butylene Glycol, Sodium Hyaluronate...","сухая|обезвоженная|чувствительная",1990,2490,да,42,"Эссенция с 96% муцина улитки — глубокое увлажнение и регенерация","Наносить 2-3 капли на очищенную кожу после тонера",Южная Корея,36 мес,"https://cdn.shopify.com/s/files/.../cosrx_essence_main.jpg|https://cdn.shopify.com/s/files/.../cosrx_essence_back.jpg",,"{""ph"":6.0,""pack_type"":""Флакон с пипеткой"",""fragrance"":""Без отдушек"",""key_benefits"":[""Увлажнение"",""Заживление"",""Регенерация""]}"
```

---

## 9. Open questions

- **Цены** — реалистичный диапазон. Косметика K-beauty в РФ: 500₽ (патчи)
  — 8 000₽ (премиум кремы Sulwhasoo). OK так?
- **Stock** — у скольких товаров `in_stock=нет`? Предлагаю ~15%.
- **`tags`** — какие теги интересны? Сейчас в схеме `новинка/хит/sale`,
  можно расширить.
