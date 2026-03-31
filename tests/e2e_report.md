# E2E Test Report — 2026-03-30 23:27

**URL**: https://keepstar.one
**Tests**: 25 | **PASS**: 20 | **FAIL**: 5
**Avg response**: 5.0s | **Min**: 5.0s | **Max**: 5.0s

## Summary

| # | Status | Time | Layout | Widgets | Cols | Size | Query |
|---|--------|------|--------|---------|------|------|-------|
| 1 | PASS | 5s | grid | 12 | 3 | small | Привет, покажи кремы для лица |
| 2 | PASS | 5s | list | 12 | — | small | Покажи их списком |
| 3 | PASS | 5s | single | 1 | — | large | Покажи детально первый товар |
| 4 | **FAIL** | 5s | grid | 23 | 3 | small | Покажи только название и цены |
| 5 | **FAIL** | 5s | grid | 12 | 3 | small | Добавь рейтинг |
| 6 | **FAIL** | 5s | grid | 12 | 3 | small | Убери цены |
| 7 | PASS | 5s | grid | 12 | 3 | small | Покажи всё как было в самом начале |
| 8 | PASS | 5s | grid | 12 | 3 | small | Покажи цену первой, потом название |
| 9 | PASS | 5s | grid | 12 | 2 | large | Покажи крупными карточками |
| 10 | PASS | 5s | carousel | 12 | — | small | Покажи каруселью |
| 11 | PASS | 5s | table | 24 | — | — | Покажи таблицей |
| 12 | **FAIL** | 5s | grid | 12 | 3 | small | Покажи снова сеткой, маленькими |
| 13 | PASS | 5s | grid | 23 | 3 | small | Покажи горизонтальными карточками |
| 14 | **FAIL** | 5s | grid | 12 | 3 | small | Покажи только COSRX |
| 15 | PASS | 5s | grid | 3 | 2 | medium | Покажи их с составом и типом кожи |
| 16 | PASS | 5s | grid | 3 | 2 | medium | Покажи все кремы The Ordinary |
| 17 | PASS | 5s | grid | 5 | 2 | large | Покажи топ-5 с рейтингом, брендом и опис |
| 18 | PASS | 5s | comparison | 4 | — | — | Сравни первые 3 |
| 19 | PASS | 5s | comparison | 3 | — | — | Сравни два самых дешёвых |
| 20 | PASS | 5s | grid | 12 | 3 | small | Покажи все увлажняющие средства |
| 21 | PASS | 5s | grid | 3 | 2 | large | Покажи первые 3 крупными карточками с оп |
| 22 | PASS | 5s | single | 1 | — | large | Покажи детально вторую |
| 23 | PASS | 5s | grid | 12 | 2 | large | Покажи сеткой по 2 в ряд |
| 24 | PASS | 5s | grid | 12 | 3 | small | Покажи с тенями и акцентным цветом |
| 25 | PASS | 5s | grid | 3 | 2 | medium | Покажи топ-3 сыворотки |

---

## Block: 1_basic

### #01 — PASS

**Query**: Привет, покажи кремы для лица
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | grid | grid |
| Widgets | 10–30 | 12 |
| Columns | 3 | 3 |
| Size | small | small |
| Slots (1st card) | hero, title, price | hero, title, price |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 306.656px |

**Bot said**: Нашёл 23 товара
**Cards**: Осветляющий крем для лица с витамином С Nacific Vitamin C Newpair Cream, Очищающий крем-пенка для умывания с овсяными экстрактами Q+A Oat Milk Cream Clea, Увлажняющий крем для лица с гиалуроновой кислотой HOLIKA HOLIKA Hyaluronic Hydra

**Screenshot**: `tests/e2e_screenshots/01_1_basic.png`

> **Visual review**: _TODO — check screenshot_

---

### #02 — PASS

**Query**: Покажи их списком
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | list | list |
| Widgets | 10–30 | 12 |
| Columns | — | — |
| Size | — | small |
| Slots (1st card) | hero, title, price | hero, price, title |
| Card CSS | — | border-radius: 16px, shadow: no, flex: row, w: 868.109px |

**Bot said**: Нашёл 23 товара
**Cards**: Осветляющий крем для лица с витамином С Nacific Vitamin C Newpair Cream, Очищающий крем-пенка для умывания с овсяными экстрактами Q+A Oat Milk Cream Clea, Увлажняющий крем для лица с гиалуроновой кислотой HOLIKA HOLIKA Hyaluronic Hydra

**Screenshot**: `tests/e2e_screenshots/02_1_basic.png`

> **Visual review**: _TODO — check screenshot_

---

### #03 — PASS

**Query**: Покажи детально первый товар
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | single | single |
| Widgets | 1–1 | 1 |
| Columns | — | — |
| Size | large | large |
| Slots (1st card) | hero, title, price | hero, title, price, primary, secondary |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 400px |

**Bot said**: Нашёл 1 товар
**Cards**: Осветляющий крем для лица с витамином С Nacific Vitamin C Newpair Cream

**Screenshot**: `tests/e2e_screenshots/03_1_basic.png`

> **Visual review**: _TODO — check screenshot_

---

## Block: 2_show_hide

### #04 — FAIL

**Query**: Покажи только название и цены
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | single | grid |
| Widgets | 1–1 | 23 |
| Columns | — | 3 |
| Size | — | small |
| Slots (1st card) | title, price | title, price |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 278.703px |

**Bot said**: Нашёл 23 товара
**Cards**: Осветляющий крем для лица с витамином С Nacific Vitamin C Newpair Cream, Очищающий крем-пенка для умывания с овсяными экстрактами Q+A Oat Milk Cream Clea, Увлажняющий крем для лица с гиалуроновой кислотой HOLIKA HOLIKA Hyaluronic Hydra

**Failures**:
- LAYOUT: expected 'single', got 'grid'
- WIDGETS: expected <= 1, got 23

**Screenshot**: `tests/e2e_screenshots/04_2_show_hide.png`

> **Visual review**: _TODO — check screenshot_

---

### #05 — FAIL

**Query**: Добавь рейтинг
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | single | grid |
| Widgets | 1–1 | 12 |
| Columns | — | 3 |
| Size | — | small |
| Slots (1st card) | title, price | hero, title, price, primary |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 278.703px |

**Bot said**: Нашёл 23 товара
**Cards**: Осветляющий крем для лица с витамином С Nacific Vitamin C Newpair Cream, Очищающий крем-пенка для умывания с овсяными экстрактами Q+A Oat Milk Cream Clea, Увлажняющий крем для лица с гиалуроновой кислотой HOLIKA HOLIKA Hyaluronic Hydra

**Failures**:
- LAYOUT: expected 'single', got 'grid'
- WIDGETS: expected <= 1, got 12
- FIELD UNWANTED: slot 'hero' should NOT be in card, found: ['hero', 'price', 'primary', 'title']
- RATING: expected rating to be visible, not found

**Screenshot**: `tests/e2e_screenshots/05_2_show_hide.png`

> **Visual review**: _TODO — check screenshot_

---

### #06 — FAIL

**Query**: Убери цены
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | single | grid |
| Widgets | 1–1 | 12 |
| Columns | — | 3 |
| Size | — | small |
| Slots (1st card) | title | hero, title |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 278.703px |

**Bot said**: Нашёл 23 товара
**Cards**: Осветляющий крем для лица с витамином С Nacific Vitamin C Newpair Cream, Очищающий крем-пенка для умывания с овсяными экстрактами Q+A Oat Milk Cream Clea, Увлажняющий крем для лица с гиалуроновой кислотой HOLIKA HOLIKA Hyaluronic Hydra

**Failures**:
- LAYOUT: expected 'single', got 'grid'
- WIDGETS: expected <= 1, got 12
- FIELD UNWANTED: slot 'hero' should NOT be in card, found: ['hero', 'title']
- RATING: expected rating to be visible, not found

**Screenshot**: `tests/e2e_screenshots/06_2_show_hide.png`

> **Visual review**: _TODO — check screenshot_

---

### #07 — PASS

**Query**: Покажи всё как было в самом начале
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | grid | grid |
| Widgets | 10–30 | 12 |
| Columns | 3 | 3 |
| Size | small | small |
| Slots (1st card) | hero, title, price | hero, title, price |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 278.703px |

**Bot said**: Нашёл 23 товара
**Cards**: Осветляющий крем для лица с витамином С Nacific Vitamin C Newpair Cream, Очищающий крем-пенка для умывания с овсяными экстрактами Q+A Oat Milk Cream Clea, Увлажняющий крем для лица с гиалуроновой кислотой HOLIKA HOLIKA Hyaluronic Hydra

**Screenshot**: `tests/e2e_screenshots/07_2_show_hide.png`

> **Visual review**: _TODO — check screenshot_

---

## Block: 3_order

### #08 — PASS

**Query**: Покажи цену первой, потом название
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | grid | grid |
| Widgets | 10–30 | 12 |
| Columns | — | 3 |
| Size | — | small |
| Slots (1st card) | title, price | price, title, hero |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 278.703px |

**Bot said**: Нашёл 23 товара
**Cards**: Осветляющий крем для лица с витамином С Nacific Vitamin C Newpair Cream, Очищающий крем-пенка для умывания с овсяными экстрактами Q+A Oat Milk Cream Clea, Увлажняющий крем для лица с гиалуроновой кислотой HOLIKA HOLIKA Hyaluronic Hydra

**Screenshot**: `tests/e2e_screenshots/08_3_order.png`

> **Visual review**: _TODO — check screenshot_

---

## Block: 4_size_layout

### #09 — PASS

**Query**: Покажи крупными карточками
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | grid | grid |
| Widgets | 10–30 | 12 |
| Columns | —–2 | 2 |
| Size | large | large |
| Slots (1st card) | hero, title, price | hero, title, price |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 426.047px |

**Bot said**: Нашёл 23 товара
**Cards**: Осветляющий крем для лица с витамином С Nacific Vitamin C Newpair Cream, Очищающий крем-пенка для умывания с овсяными экстрактами Q+A Oat Milk Cream Clea, Увлажняющий крем для лица с гиалуроновой кислотой HOLIKA HOLIKA Hyaluronic Hydra

**Screenshot**: `tests/e2e_screenshots/09_4_size_layout.png`

> **Visual review**: _TODO — check screenshot_

---

### #10 — PASS

**Query**: Покажи каруселью
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | carousel | carousel |
| Widgets | 10–30 | 12 |
| Columns | — | — |
| Size | — | small |
| Slots (1st card) |  | hero, title, price |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 731.078px |

**Bot said**: Нашёл 23 товара
**Cards**: Осветляющий крем для лица с витамином С Nacific Vitamin C Newpair Cream, Очищающий крем-пенка для умывания с овсяными экстрактами Q+A Oat Milk Cream Clea, Увлажняющий крем для лица с гиалуроновой кислотой HOLIKA HOLIKA Hyaluronic Hydra

**Screenshot**: `tests/e2e_screenshots/10_4_size_layout.png`

> **Visual review**: _TODO — check screenshot_

---

### #11 — PASS

**Query**: Покажи таблицей
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | table | table |
| Widgets | 10–30 | 24 |
| Columns | — | — |
| Size | — | — |
| Slots (1st card) |  |  |
| Card CSS | — | border-radius: 0px, shadow: no, flex: column, w: 140px |

**Bot said**: Нашёл 23 товара

**Screenshot**: `tests/e2e_screenshots/11_4_size_layout.png`

> **Visual review**: _TODO — check screenshot_

---

### #12 — FAIL

**Query**: Покажи снова сеткой, маленькими
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | grid | grid |
| Widgets | 10–30 | 12 |
| Columns | 4–5 | 3 |
| Size | small | small |
| Slots (1st card) |  | hero, title, price |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 278.703px |

**Bot said**: Нашёл 23 товара
**Cards**: Осветляющий крем для лица с витамином С Nacific Vitamin C Newpair Cream, Очищающий крем-пенка для умывания с овсяными экстрактами Q+A Oat Milk Cream Clea, Увлажняющий крем для лица с гиалуроновой кислотой HOLIKA HOLIKA Hyaluronic Hydra

**Failures**:
- COLUMNS: expected >= 4, got 3

**Screenshot**: `tests/e2e_screenshots/12_4_size_layout.png`

> **Visual review**: _TODO — check screenshot_

---

### #13 — PASS

**Query**: Покажи горизонтальными карточками
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | ['list', 'grid'] | grid |
| Widgets | 10–30 | 23 |
| Columns | — | 3 |
| Size | — | small |
| Slots (1st card) |  | hero, title, price |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 278.703px |

**Bot said**: Нашёл 23 товара
**Cards**: Осветляющий крем для лица с витамином С Nacific Vitamin C Newpair Cream, Очищающий крем-пенка для умывания с овсяными экстрактами Q+A Oat Milk Cream Clea, Увлажняющий крем для лица с гиалуроновой кислотой HOLIKA HOLIKA Hyaluronic Hydra

**Screenshot**: `tests/e2e_screenshots/13_4_size_layout.png`

> **Visual review**: _TODO — check screenshot_

---

## Block: 5_filter

### #14 — FAIL

**Query**: Покажи только COSRX
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | grid | grid |
| Widgets | 2–8 | 12 |
| Columns | — | 3 |
| Size | — | small |
| Slots (1st card) |  | hero, title, price |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 278.703px |

**Bot said**: Нашёл 23 товара
**Cards**: Осветляющий крем для лица с витамином С Nacific Vitamin C Newpair Cream, Очищающий крем-пенка для умывания с овсяными экстрактами Q+A Oat Milk Cream Clea, Увлажняющий крем для лица с гиалуроновой кислотой HOLIKA HOLIKA Hyaluronic Hydra

**Failures**:
- WIDGETS: expected <= 8, got 12

**Screenshot**: `tests/e2e_screenshots/14_5_filter.png`

> **Visual review**: _TODO — check screenshot_

---

### #15 — PASS

**Query**: Покажи их с составом и типом кожи
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | grid | grid |
| Widgets | 2–8 | 3 |
| Columns | — | 2 |
| Size | — | medium |
| Slots (1st card) |  | hero, title, price, secondary, primary |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 426.047px |

**Bot said**: Нашёл 3 товара
**Cards**: Успокаивающий крем для чувствительной кожи Cosrx Pure Fit Cica Cream, Лечебный точечный крем от акне Cosrx AC Collection Ultimate Spot Cream, Точечный крем центелла против акне и купероза COSRX Centella Blemish Cream

**Screenshot**: `tests/e2e_screenshots/15_5_filter.png`

> **Visual review**: _TODO — check screenshot_

---

### #16 — PASS

**Query**: Покажи все кремы The Ordinary
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | grid | grid |
| Widgets | 1–30 | 3 |
| Columns | — | 2 |
| Size | — | medium |
| Slots (1st card) |  | hero, title, price, primary |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 426.047px |

**Bot said**: Нашёл 3 товара
**Cards**: Успокаивающий крем для чувствительной кожи Cosrx Pure Fit Cica Cream, Лечебный точечный крем от акне Cosrx AC Collection Ultimate Spot Cream, Точечный крем центелла против акне и купероза COSRX Centella Blemish Cream

**Screenshot**: `tests/e2e_screenshots/16_5_filter.png`

> **Visual review**: _TODO — check screenshot_

---

## Block: 6_complex

### #17 — PASS

**Query**: Покажи топ-5 с рейтингом, брендом и описанием, крупно
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | grid | grid |
| Widgets | 4–6 | 5 |
| Columns | — | 2 |
| Size | large | large |
| Slots (1st card) |  | hero, title, price, primary |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 426.047px |

**Bot said**: Нашёл 5 товаров
**Cards**: Восстанавливающий крем с термальной водой MEDI-PEEL Herb Thermal Ceramide Cream, Очищающий гидрофильный крем с экстрактом риса The Saem Natural Condition Rice Cl, Очищающий гидрофильный крем с экстрактом авокадо The Saem Natural Condition Avoc

**Screenshot**: `tests/e2e_screenshots/17_6_complex.png`

> **Visual review**: _TODO — check screenshot_

---

### #18 — PASS

**Query**: Сравни первые 3
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | ['comparison', 'table'] | comparison |
| Widgets | 2–4 | 4 |
| Columns | — | — |
| Size | — | — |
| Slots (1st card) |  |  |
| Card CSS | — | border-radius: 0px, shadow: no, flex: column, w: 140px |

**Bot said**: Нашёл 3 товара

**Screenshot**: `tests/e2e_screenshots/18_6_complex.png`

> **Visual review**: _TODO — check screenshot_

---

### #19 — PASS

**Query**: Сравни два самых дешёвых
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | ['comparison', 'table'] | comparison |
| Widgets | 2–3 | 3 |
| Columns | — | — |
| Size | — | — |
| Slots (1st card) |  |  |
| Card CSS | — | border-radius: 0px, shadow: no, flex: column, w: 140px |

**Bot said**: Нашёл 2 товара

**Screenshot**: `tests/e2e_screenshots/19_6_complex.png`

> **Visual review**: _TODO — check screenshot_

---

## Block: 7_drilldown

### #20 — PASS

**Query**: Покажи все увлажняющие средства
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | grid | grid |
| Widgets | 5–50 | 12 |
| Columns | — | 3 |
| Size | — | small |
| Slots (1st card) |  | hero, title, price |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 278.703px |

**Bot said**: Нашёл 50 товаров
**Cards**: Увлажняющие пилинг-пэды на основе газированной воды MEDI-PEEL Aqua Mooltox Spark, Увлажняющие гелевые тонер-пэды для лица с коллагеном Biodance Collagen Gel Toner, Увлажняющие тонер-пэды для лица с берёзовым соком ANUA Birch 70% Moisture Boosti

**Screenshot**: `tests/e2e_screenshots/20_7_drilldown.png`

> **Visual review**: _TODO — check screenshot_

---

### #21 — PASS

**Query**: Покажи первые 3 крупными карточками с описанием
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | grid | grid |
| Widgets | 2–4 | 3 |
| Columns | — | 2 |
| Size | large | large |
| Slots (1st card) |  | hero, title, price, primary |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 426.047px |

**Bot said**: Нашёл 3 товара
**Cards**: Увлажняющие пилинг-пэды на основе газированной воды MEDI-PEEL Aqua Mooltox Spark, Увлажняющие гелевые тонер-пэды для лица с коллагеном Biodance Collagen Gel Toner, Увлажняющие тонер-пэды для лица с берёзовым соком ANUA Birch 70% Moisture Boosti

**Screenshot**: `tests/e2e_screenshots/21_7_drilldown.png`

> **Visual review**: _TODO — check screenshot_

---

### #22 — PASS

**Query**: Покажи детально вторую
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | single | single |
| Widgets | 1–1 | 1 |
| Columns | — | — |
| Size | — | large |
| Slots (1st card) |  | hero, title, price, primary, secondary |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 400px |

**Bot said**: Нашёл 1 товар
**Cards**: Увлажняющие гелевые тонер-пэды для лица с коллагеном Biodance Collagen Gel Toner

**Screenshot**: `tests/e2e_screenshots/22_7_drilldown.png`

> **Visual review**: _TODO — check screenshot_

---

## Block: 8_v2

### #23 — PASS

**Query**: Покажи сеткой по 2 в ряд
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | grid | grid |
| Widgets | 5–50 | 12 |
| Columns | 2 | 2 |
| Size | — | large |
| Slots (1st card) |  | hero, title, price |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 426.047px |

**Bot said**: Нашёл 50 товаров
**Cards**: Увлажняющие пилинг-пэды на основе газированной воды MEDI-PEEL Aqua Mooltox Spark, Увлажняющие гелевые тонер-пэды для лица с коллагеном Biodance Collagen Gel Toner, Увлажняющие тонер-пэды для лица с берёзовым соком ANUA Birch 70% Moisture Boosti

**Screenshot**: `tests/e2e_screenshots/23_8_v2.png`

> **Visual review**: _TODO — check screenshot_

---

### #24 — PASS

**Query**: Покажи с тенями и акцентным цветом
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | grid | grid |
| Widgets | 5–50 | 12 |
| Columns | — | 3 |
| Size | — | small |
| Slots (1st card) |  | hero, title, price |
| Card CSS | — | border-radius: 16px, shadow: yes, flex: column, w: 278.703px |

**Bot said**: Нашёл 50 товаров
**Cards**: Увлажняющие пилинг-пэды на основе газированной воды MEDI-PEEL Aqua Mooltox Spark, Увлажняющие гелевые тонер-пэды для лица с коллагеном Biodance Collagen Gel Toner, Увлажняющие тонер-пэды для лица с берёзовым соком ANUA Birch 70% Moisture Boosti

**Screenshot**: `tests/e2e_screenshots/24_8_v2.png`

> **Visual review**: _TODO — check screenshot_

---

### #25 — PASS

**Query**: Покажи топ-3 сыворотки
**Time**: 5s

| | Expected | Actual |
|---|----------|--------|
| Layout | grid | grid |
| Widgets | 2–4 | 3 |
| Columns | — | 2 |
| Size | — | medium |
| Slots (1st card) |  | hero, title, price, primary |
| Card CSS | — | border-radius: 16px, shadow: no, flex: column, w: 426.047px |

**Bot said**: Нашёл 3 товара
**Cards**: Точечная противовоспалительная сыворотка с комплексом кислот CosRX The AHA 2 BHA, Увлажняющая сыворотка с бета-глюканом Dr.Althea Premium Intensive Hydration Boos, Смываемая пилинг-сыворотка для лица с комплексом кислот IsNtree Hyper Acid4 30 S

**Screenshot**: `tests/e2e_screenshots/25_8_v2.png`

> **Visual review**: _TODO — check screenshot_

---
