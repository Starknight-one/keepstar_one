# Seed Catalog Sources

Откуда брались данные для тестовых тенантов. Сохраняем чтобы потом
расширять каталоги до боевого ПИМа без переоткрытия источников.

---

## Tenant A — Cosmetics (Sephora)

- **Источник:** [`MayaKitzis/sephora_products`](https://huggingface.co/datasets/MayaKitzis/sephora_products) (Hugging Face)
- **URL:** `https://huggingface.co/datasets/MayaKitzis/sephora_products/resolve/main/product_info.csv`
- **Файл в репо:** `docs/seeds/tenant_A/sephora_products.csv` (~8 MB)
- **Объём:** 8,494 товара (взят полностью as-is)
- **Язык:** английский
- **Бренды:** 304 (Dior, TOM FORD, Charlotte Tilbury, Fenty Beauty, Clinique, Kiehl's, Drunk Elephant, Sephora Collection, и K-beauty: Laneige, Sulwhasoo, Innisfree, Glow Recipe, Dr. Jart+, belif)
- **Категории:** Skincare (28%), Makeup (28%), Hair (17%), Fragrance (17%), Bath & Body (5%), Mini, Men, Tools, Gifts
- **Поля (27):** product_id, product_name, brand_id, brand_name, loves_count, rating, reviews, size, variation_type/value/desc, ingredients, price_usd, value_price_usd, sale_price_usd, limited_edition, new, online_only, out_of_stock, sephora_exclusive, highlights, primary/secondary/tertiary_category, child_count, child_max/min_price
- **Заполненность:** product_name 100%, brands 100%, price 100%, ingredients 88%, size 80%, rating 96%, highlights 74%
- **Цены:** $3 — $1,900 USD, медиана $35
- **Photos:** ❌ нет URL изображений (deferred, см. task #5). Sephora CDN залочен Akamai, нужен альтернативный путь.

**Как расширить:** этот датасет конечен (8.5k). Для большего объёма косметики потенциальные источники:
- Альтернативные Sephora-датасеты на HF (нужен с image URLs)
- Direct Shopify `/products.json` от DTC брендов (Beauty of Joseon, Glow Recipe, и т.п. — каждый бренд отдельно)
- INCIDecoder, EWG Skin Deep — есть API/скрейп

---

## Tenant B — Electronics (Amazon)

- **Источник:** [`McAuley-Lab/Amazon-Reviews-2023`](https://huggingface.co/datasets/McAuley-Lab/Amazon-Reviews-2023) (Hugging Face, McAuley Lab UCSD)
- **Файл источника:** `raw/meta_categories/meta_Electronics.jsonl` (~5 GB, ~1M товаров total)
- **Файл в репо:** `docs/seeds/tenant_B/products.jsonl` (~13 MB)
- **Объём:** 3,000 товаров (стримом + quality filter)
- **Streamed:** 25.5 MB (отсечка на 3000 прошедших фильтр товаров)
- **Pass rate фильтра:** 41% (7,278 строк → 3,000 годных)
- **Язык:** английский
- **Бренды:** HP, Dell, ASUS, Samsung, Lenovo, Amazon Renewed, Fintie, плюс generic-noname
- **Категории:** Computers & Accessories (43%), Camera & Photo (15%), Television & Video (9%), Headphones (8%), Home Audio (4%), Car Electronics, Wearables, Security/Surveillance, GPS, Power
- **Поля (14):** main_category, title, average_rating, rating_number, features (bullet array), description (array), price, images [{thumb, large}], videos, store (бренд), categories (полный путь массивом), details (specs dict), parent_asin (SKU), bought_together
- **Цены:** $0.01 — $5000+ USD, скос к низу (1714 товаров под $25)
- **Photos:** ✅ есть URL фото в поле `images` (m.media-amazon.com CDN)

---

## Tenant C — Mixed Marketplace (Amazon, 5 категорий)

- **Источник:** тот же `McAuley-Lab/Amazon-Reviews-2023`, 5 разных категорий
- **Файл в репо:** `docs/seeds/tenant_C/products.jsonl` (~12 MB)
- **Объём:** 3,000 товаров (600 из каждой категории, маркированы `_source_category`)
- **Категории:**
  - `Office_Products` — 600 (Avery, Alliance, генерики)
  - `Sports_and_Outdoors` — 600
  - `Toys_and_Games` — 600 (LEGO, Mattel, Hasbro, Funko, Magic The Gathering)
  - `Pet_Supplies` — 600
  - `Tools_and_Home_Improvement` — 600 (DEWALT, Stanley, Rust-Oleum, 3M, Moen)
- **Total streamed:** ~19 MB
- **Photos:** ✅ есть

**Почему именно эти 5:**
- Office + Tools + Sports + Toys + Pet = реалистичный универсальный marketplace без overlap с Sephora (cosmetics) и Electronics (B) и Furniture (D).
- `Health_and_Personal_Care` пробовал — в дампе у этой категории `categories: []` пустые, не прошёл фильтр.
- Каждое поле помечено `_source_category` для трассировки происхождения.

---

## Tenant D — Furniture (Amazon)

- **Источник:** тот же `McAuley-Lab/Amazon-Reviews-2023`
- **Файл источника:** `raw/meta_categories/meta_Home_and_Kitchen.jsonl` (~12 GB, миллионы товаров)
- **Фильтр:** `'Furniture' in categories[]`
- **Файл в репо:** `docs/seeds/tenant_D/products.jsonl` (~15 MB)
- **Объём:** 3,000 товаров (после фильтра)
- **Streamed:** 313 MB (Furniture — ~3% от общего H&K, поэтому много отбраковки)
- **Lines scanned:** 89,504 (~3.4% Furniture-матч)
- **Язык:** английский
- **Подкатегории:** Living Room (947), Bedroom (688), Home Office (384), Dining (299), Game/Recreation (294), Kids' (94), Accent (77), Entryway (74), Kitchen (43), Bathroom (11)
- **Бренды:** Flash Furniture, Safavieh, Modway, Christopher Knight Home, East West Furniture, Baxton Studio, Coaster, Zinus, Signature Design by Ashley, Yaheetech
- **Поля:** те же 14 что у Electronics/Mixed (Amazon-стандарт)
- **Photos:** ✅ есть

---

## Тотальная схема Amazon-метаданных (B/C/D одинаковая)

```jsonc
{
  "main_category": "All Electronics" | "Amazon Home" | "Office Products" | ...,
  "title": "<long product title with attributes baked in>",
  "average_rating": 4.5,
  "rating_number": 1234,
  "features": ["bullet 1", "bullet 2", ...],   // маркетинговые буллеты
  "description": ["paragraph 1", ...],          // длинный текст
  "price": "29.99",                             // строкой
  "images": [{"thumb": "url", "large": "url", "variant": "..."}, ...],
  "videos": [...],
  "store": "Brand Name",
  "categories": ["L1", "L2", "L3", ...],        // полная иерархия
  "details": {                                  // technical specs (variable keys per category)
    "Brand": "...", "Color": "...", "Material": "...",
    "Capacity": "...", "Manufacturer": "...", ...
  },
  "parent_asin": "B0XXXXXXXX",                  // SKU
  "bought_together": [...]
}
```

В тенанте C добавлено поле `_source_category` для трассировки.

---

## Воспроизведение (если нужно перекачать или расширить)

Все стримящие скрипты лежат в логе сессии `docs/Updates/`. Базовый паттерн:

```python
import urllib.request, json
URL = "https://huggingface.co/datasets/McAuley-Lab/Amazon-Reviews-2023/resolve/main/raw/meta_categories/meta_<CATEGORY>.jsonl"
req = urllib.request.Request(URL, headers={"User-Agent": "KeepstarSeed/0.1"})
with urllib.request.urlopen(req) as resp:
    buf = b""
    while passed < TARGET:
        chunk = resp.read(65536)
        if not chunk: break
        buf += chunk
        while b"\n" in buf:
            line, buf = buf.split(b"\n", 1)
            p = json.loads(line)
            # quality filter: title, store, price, images, description/features, len(categories)>=2
            ...
```

Pass rate ~40% для Electronics, ~3% для Furniture (фильтр узкий), ~40-50% для остальных Amazon-категорий с заполненными `categories`.

---

## Все 33 категории Amazon Reviews 2023 (для будущего ПИМа)

```
All_Beauty, Amazon_Fashion, Appliances, Arts_Crafts_and_Sewing,
Automotive, Baby_Products, Beauty_and_Personal_Care, Books, CDs_and_Vinyl,
Cell_Phones_and_Accessories, Clothing_Shoes_and_Jewelry, Digital_Music,
Electronics, Gift_Cards, Grocery_and_Gourmet_Food, Handmade_Products,
Health_and_Household, Health_and_Personal_Care*, Home_and_Kitchen,
Industrial_and_Scientific, Kindle_Store, Magazine_Subscriptions, Movies_and_TV,
Musical_Instruments, Office_Products, Patio_Lawn_and_Garden, Pet_Supplies,
Software, Sports_and_Outdoors, Subscription_Boxes, Tools_and_Home_Improvement,
Toys_and_Games, Video_Games, Unknown
```

\* `Health_and_Personal_Care` — пустые `categories` в дампе, требует другого фильтра.

Каждая категория ~1-13 GB JSONL. Полный дамп = терабайт+ — но качать целиком не нужно, стримим первые N MB и фильтруем до целевого N товаров.

---

## License / Atribution

- **MayaKitzis/sephora_products** — license не указан явно на HF (учебный проект, EDA на публичных данных Sephora). Использовать с осторожностью для коммерческой публикации; для внутреннего тестирования OK.
- **McAuley-Lab/Amazon-Reviews-2023** — research-only license, цитировать [McAuley et al., 2024](https://arxiv.org/abs/2403.03952). Использование для тестирования системы — OK; редистрибьюция данных другим людям — нет.

Для боевого продакшна каталоги нужно будет источать иначе (своя интеграция с Shopify/marketplace API мерчанта).
