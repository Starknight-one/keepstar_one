# Testbench Testing Log

**Date:** 2026-02-25 — 2026-02-26
**Branch:** `feature/visual-assembly-engine`

---

## Bug #1: Show Fields / Order не влияют на рендер

**Steps:**
1. Открыл тестбенч, Auto preset, 6 товаров → получил grid (image + name + price + brand)
2. Добавил в Show Fields: name, price, brand, description → результат не изменился
3. Заменил description на rating (description отсутствует в данных) → результат не изменился
4. Изменил Order: name > rating > brand > price → результат не изменился

**Expected:** Карточки должны показывать добавленные поля и менять порядок отображения.

**Actual:** Карточки всегда одинаковые: image + name(h3) + price + brand(tag). Show fields и order игнорируются.

**Root cause:**
1. **Order** — не был реализован в handler_testbench.go, параметр `order` вообще не читался
2. **Show fields не появляются** — constraint C1 (`normalizeFieldSet`) удаляет поля которые есть менее чем у 70% виджетов. Если rating есть у 3 из 6 товаров → 3 < int(6×0.7)=4 → rating удаляется из ВСЕХ карточек

**Fix:**
1. Добавлена обработка `params["order"]` — переставляет fields в указанном порядке, остальные в конец
2. Когда show fields указаны явно → cross-widget constraints (C1) пропускаются, чтобы user-specified поля не удалялись

**Status:** fixed, retested OK

---

## Bug #2: Rating=0 не показывается (разнобой в карточках)

**Steps:**
1. Auto preset, 6 товаров → rating показывается у товаров где есть данные, у остальных (rating=0) — пусто
2. Карточки выглядят по-разному: у одних 4 поля, у других 3

**Expected:** Rating=0 → показывать "★ 0" единообразно у всех карточек.

**Root cause:** `productFieldGetter` возвращал nil при `p.Rating == 0` → атом не создавался → карточка без рейтинга.

**Fix:**
- `productFieldGetter`: убрал `if p.Rating == 0 { return nil }` → теперь всегда возвращает `p.Rating` (0.0 для пустых)
- `serviceFieldGetter`: аналогично
- Тест `TestProductFieldGetter_ZeroRatingReturnsZero` обновлён
- Frontend AtomRenderer уже корректно обрабатывает value=0 (guard: `atom.value !== 0`)

**Status:** fixed

---

## Bug #2b: `attributes` в product fieldRanking — бесполезное поле

**Context:** `fieldRanking["product"]` содержал `"attributes"`, но `productFieldGetter` не знает такого поля (только у сервисов есть `Service.Attributes map[string]interface{}`). Для продуктов это всегда nil.

**What is `attributes`:** Поле `Service.Attributes` — произвольные пары ключ-значение для услуг. Для продуктов "атрибуты" — это PIM-поля: `productForm`, `skinType`, `concern`, `keyIngredients`.

**Fix:** Убрал `attributes` из `fieldRanking["product"]`, добавил PIM-поля: `productForm`, `skinType`, `concern`, `keyIngredients`.

**TODO (prompt):** В Agent2 промпте нужно описать семантику: если пользователь говорит "покажи атрибуты / подробнее / точнее" → это PIM-поля продукта. "Покажи какие атрибуты у крема" → вывести все доступные поля продукта.

**Status:** частично fixed (ranking исправлен, prompt TODO)

---

## Bug #2c: keyIngredients — "ключевые" а не все

**Context:** Пользователь ожидает увидеть ВСЕ ингредиенты, но поле `KeyIngredients []string` в каталоге содержит только ключевые (из PIM enrichment). Полного состава нет в структуре данных.

**Также:** D5 constraint обрезает secondary slot до 120 символов в grid, 300 в single — длинные списки будут с "…"

**Status:** logged — это ограничение данных каталога, не движка. Допилить позже при расширении PIM

---

## Bug #3: Button action не реализован

**Context:** AtomRenderer.jsx line ~275 — `TODO: dispatch action to parent via context or callback`

**Expected:** Кнопки в виджетах должны выполнять действия (добавить в корзину, подробнее и т.д.)

**Status:** logged, не критично для MVP

---

## Bug #4: W7 constraint — horizontal + >4 атомов → vertical принудительно

**Context:** constraints.go — если пользователь явно попросил horizontal, но >4 атомов, constraint принудительно меняет на vertical.

**Expected:** Если пользователь явно задал direction, не переопределять.

**Status:** logged

---

## Bug #5: Image URL валидация без кэша

**Context:** constraints.go D7 — image URLs валидируются при каждом рендере. Нет кэширования результатов.

**Expected:** Для прода — кэшировать результаты валидации.

**Status:** logged, оптимизация для прода

---

## Bug #6: Direction "horizontal" — не понятен эффект

**Steps:**
1. Auto preset, 6 товаров → grid
2. Ставлю Direction = horizontal → визуально ничего не меняется (или почти ничего)

**Expected:** Пользователь ожидает что "horizontal" = карточки в ряд (как карусель).

**Actual:** "horizontal" меняет только внутреннюю раскладку карточки (картинка слева, контент справа). В узких колонках grid разница почти незаметна — картинка 120px в узкой колонке визуально сливается.

**Root cause:** Direction — параметр виджета (внутренняя раскладка), а не формации (расположение виджетов). Название вводит в заблуждение.

**TODO:** Подумать над UX — возможно direction на уровне формации должен переключать layout (grid→carousel), а не внутреннюю раскладку карточки.

**Status:** logged, UX решение

---

## Bug #7: Single hero preset — меньше полей чем Auto single

**Steps:**
1. 1 товар, preset = Auto → видны все поля (images, name, price, rating, brand, category, description, tags, stockQuantity + PIM)
2. 1 товар, preset = product_single_hero → только 6 полей (images, name, brand, price, rating, description)

**Expected:** Hero = "самая полная карточка", должна показывать максимум информации.

**Root cause:** Пресет был определён всего с 6 полями. Auto single использует maxFields=10 из fieldRanking.

**Fix:** Добавил в ProductSingleHeroPreset недостающие поля: category, tags, productForm, skinType (итого 10 полей).

**Status:** fixed

---

## Bug #8: Category overview — все карточки одинаковые

**Steps:**
1. preset = category_overview, 6 товаров → все карточки показывают "Essences" (одна категория)

**Expected:** Пресет для обзора РАЗНЫХ категорий.

**Actual:** Тестовые данные (heybabes) — все товары в одной категории "Essences". Пресет работает корректно, но бесполезен с текущими данными.

**Root cause:** Ограничение тестовых данных, не баг движка. Пресет задуман для выборки из разных категорий.

**Status:** logged — not a bug, data limitation

---

## Bug #9: Slice aliasing — глобальный fieldRanking перезаписывался (CRITICAL)

**Found by:** Fuzz-тест, 10,000 random итераций

**Root cause:** `AutoResolve()` возвращал `ranking[:maxF]` — sub-slice глобального `fieldRanking`. У sub-slice cap=13, len=5. Когда вызывающий код делал `append(fields, "newField")`, Go писал прямо в глобальный массив `fieldRanking[5]`, перезаписывая "category" на "newField". Последующие вызовы `AutoResolve` видели испорченные данные → дубликаты полей, непредсказуемый набор атомов.

**Severity:** Critical — в проде при нескольких последовательных запросах данные полей становились случайными.

**Fix:** `AutoResolve` теперь возвращает `copy()` вместо sub-slice.

**Status:** fixed

---

## Bug #10: Nil placeholders в C1 constraint

**Found by:** Fuzz-тест

**Root cause:** `normalizeFieldSet` добавлял placeholder-атомы для пропущенных полей. Для number/image типов ставил `value = nil` → нарушение инварианта "нет nil атомов".

**Fix:** number placeholder → `float64(0)`, image placeholder → `""` (фронт покажет заглушку).

**Status:** fixed

---

## Bug #11: Дубликаты полей в C1 normalizeFieldSet

**Found by:** Fuzz-тест

**Root cause:** `normalizeFieldSet` не проверяла дубликаты при фильтрации — если атом с одним FieldName попадал дважды (из-за bug #9), оба проходили.

**Fix:** Добавлен dedup guard через `presentFields` map.

**Status:** fixed

---

## Fuzz Test Results (2026-02-26)

**Файл:** `formation_fuzz_test.go`

| Тест | Комбинаций | Время | Результат |
|------|-----------|-------|-----------|
| TestExhaustiveStructural | 315,000 | 5.5s | **PASS** |
| TestExhaustiveAtomTransforms | 2,392 | 0.03s | **PASS** |
| TestRandomFuzzCombined | 10,000 | 0.24s | **PASS** |

**Инварианты (12 шт.):** nil values, valid displays, valid formats, entity refs, no duplicate fields, max badges, max tags, max headings, direction propagation, color overrides, widget count, field consistency.

**Вывод:** Backend data pipeline прошёл 327,392 комбинации без единого нарушения. Данные из движка корректны при любых параметрах.

---
