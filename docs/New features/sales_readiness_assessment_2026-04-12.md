> ⚠️ **HISTORICAL — assessment from 2026-04-12 (pre-V5-production).** Current priorities are in /FINAL_PHASE_PLAN.md and docs/PRE_LAUNCH_TASKS.md. This evaluated 4 V4-engine features; work has since moved to V5 production stabilization + decomposition. Kept for reference on the feature complexity estimates. (flagged 2026-06-01)

# Sales-readiness assessment — 4 фичи на оценку

**Дата**: 2026-04-12
**Ветка**: `feature/engine-v4`
**Статус**: черновик оценки, не финальный план. Пересматривать после приземления B7 и фичи 2.
**Контекст**: пользователь рассматривает 4 новые фичи для перехода к продажам и демо. Задача этого документа — оценить сложность, зависимости и порядок.

---

## TL;DR

| # | Фича | Сложность | Срок | Value для продаж | Блокер демо? |
|---|------|-----------|------|------------------|--------------|
| 3 | Skip Agent2 на простых запросах | ★☆☆☆☆ | 1-2 дня | Cost 1.15¢ → ~0.6¢ | Нет, но merge-gate |
| 4 | Загружаемые дизайн-системы | ★★☆☆☆ | 2-3 дня | WOW (Halloween switch) | Нет |
| 2 | Редактор пресетов | ★★★☆☆ | MVP 1 нед / full 2-3 нед | Killer feature | Нет |
| 1 | Универсальный загрузчик | ★★★★★ | 2-3 нед (минимум) | CRITICAL для не-cosmetics клиентов | **ДА** |

**Общий фундамент — B7** (`catalog.field_definitions` + role-based slot resolution). Он уже в `PRE_LAUNCH_TASKS.md`. Без него фичи 1 и 2 живут только на cosmetics, а это значит демо только для одного клиента. С ним — fits all. Рекомендую B7 сразу после быстрых win'ов (3, 4), **до** фичи 2.

Подробный дизайн редактора пресетов (фича 2): см. `PRESET_EDITOR_SPEC.md`.

---

## Фича 3 — Skip Agent2 (★☆☆☆☆, 1-2 дня) — делать первой

**Почему дёшево**: вся инфраструктура уже в коде, надо только добавить gate.

### Что есть

- `project_v4/backend/internal/engine_v4/default_ops.go:11-25` — `ProductCardGridOps()` и `ProductDetailOps()` уже существуют и **уже используются** в `pipeline_execute.go:434,448` (`buildAdjacentTemplates`) и `navigation_back.go`. То есть direct-engine-call без Agent2 — это уже протоптанный путь для adjacent templates.
- Agent1 уже отдаёт все нужные сигналы: `ToolName`, `ProductsFound`, `StopReason` (`agent1_execute.go:192,305`).
- Microcontext уже классифицирует: `"new_search"`, `"filtered"`, `"no_data_change"` (`pipeline_execute.go:520-531`).

### Что нужно

1. В `pipeline_execute.go:233` перед вызовом Agent2 — gate:
   ```
   shouldSkip := agent1Resp.ToolName in {catalog_search, _internal_state_filter}
                 && agent1Resp.StopReason != "max_tokens"
                 && req.ScreenContext == nil
   ```
2. Если skip — дёрнуть `engine.Execute(ProductCardGridOps(), data, layout=grid, columns=...)`, сохранить в `state.Current.Template["formation"]`.
3. **Критично**: синтетически пополнить `state.Agent2History` — иначе следующий turn Agent2 увидит пустоту и сломается (Agent2History отдельный от Agent1History, см. `agent2_execute.go:334-346`).
4. Флаг `skipped_agent2` в PipelineTrace для метрик.

### Риски (что НЕ сломает)

- "Покажи крема" → catalog_search → skip ✓
- "Только дешевле 1000" → _internal_state_filter → skip ✓
- "Покрась цены в красный" → нет tool call от Agent1 → не попадает в skip → Agent2 работает как обычно ✓
- "Покажи по 2 в строку" → state-only request, нет tool call → Agent2 modify ✓

### Risk-to-value

Очень низкий риск, ~40-50% токенов уходит на типовых запросах. Это **прямо тот merge-gate** на 1.15¢ → <1¢, который уже в плане (см. `v4_multi_widget_done_2026_04_07`).

---

## Фича 4 — Загружаемые дизайн-системы (★★☆☆☆, 2-3 дня)

**Почему дёшево**: backbone уже полностью на месте, осталось соединить провода.

### Что есть

- **Frontend**: `project/frontend/src/shared/theme/themes/marketplace.css` — 135 CSS variables (цвета, типографика, spacing, radius, shadow). `ThemeProvider.jsx` + Context + localStorage + Shadow DOM scope — всё живое.
- **Backend**: `project_admin/backend/internal/domain/tenant_settings.go:4` уже имеет `Theme string`. GET/PUT `/settings` работают.
- `ThemeSwitcher.jsx` уже есть (используется только в админке для тестов).
- Shadow DOM scope означает что темы можно переключать без утечек в хост-страницу.

### Что нужно

1. Новый endpoint в chat-backend: `GET /api/v1/tenant/{slug}/theme` → возвращает `{ cssVariables: {...}, presets: ["default", "halloween"] }`.
2. В `project/frontend/src/shared/config/WidgetConfigContext.jsx` — fetch темы при mount (и fallback на marketplace если нет).
3. В `WidgetApp.jsx:55` убрать hardcoded `defaultTheme="marketplace"` → динамика из конфига.
4. Schema: `tenant_themes (tenant_id, name, variables JSONB, is_active)` — или простой JSONB в `TenantSettings.Theme`.
5. Admin UI: `project_admin/frontend/src/features/settings/SettingsPage.jsx` → добавить вкладку Theme с JSON editor или visual palette picker + preview.
6. Для Halloween-switch: `ThemeSwitcher` в виджете (overlay или settings button) → PUT active_theme → refetch → inject new `<style>` в Shadow root.

### Подводный камень

`AtomV2Renderer.jsx:6-14` **дублирует** константы токенов из `project/backend/internal/engine/tokens.go` (`FONT_SIZE_TOKENS`, `COLOR_PALETTE`). Если клиент загружает кастомные шрифты/цвета — нужно, чтобы frontend резолвил токены из полученного theme-JSON, а не из хардкода. Это +полдня к оценке.

### Value

Демо-эффект огромный. "Вот виджет на сайте клиента — одним кликом Halloween → чёрно-оранжевый с эффектом трещин на кнопках". Для B2B2C это именно тот "wow", который закрывает первый звонок.

---

## Фича 2 — Редактор пресетов (★★★☆☆, MVP 1 неделя / full 2-3 недели)

**Полный дизайн-док**: см. `PRESET_EDITOR_SPEC.md` в этой же папке.

**Почему не сверхсложно**: архитектурная удача — $ref-система уже JSON-сериализуется 1:1. Миграция с Go builder → DB-stored — тривиальна.

### Что есть (коротко)

- `Op` struct в `engine_v4/ops.go` уже с полными `json` tags — готов к сохранению в JSONB.
- `$ref` resolution через `resolveRefs(op, refs)` (`ops.go:173-186`) — работает одинаково для preset-ops из Go или из БД.
- `tool_visual_assembly.go:199-219` уже делает `append(presetOps, userOps)` — концепт "preset как массив ops" уже работает.
- `FormationRenderer.jsx` — полный рендерер, reuse для live preview в админке.
- 12 пресетов в `presets_{product,system,nav}.go` — образцы для миграции.

### Фаза MVP (1 неделя)

1. Schema: `presets_v4 (id, tenant_id, name, category, description, ops JSONB, default_replicate bool, refs TEXT[])`.
2. `PresetPort`: `SavePreset/LoadPreset/ListPresets(tenantID)`.
3. Рефактор `engine_v4/presets.go:registry` — tenant-aware lookup с fallback на system presets.
4. Миграция 12 hardcoded → seed в БД как `is_system=true, tenant_id=NULL`.
5. CRUD API: `POST/GET/PUT/DELETE /api/admin/presets`.
6. Admin UI: JSON editor + "Test with sample data" кнопка → рендер через FormationRenderer.
7. Dynamic Agent2 system prompt — инжектит per-tenant preset list.

### Фаза Full (+1-2 недели)

- Visual drag-n-drop редактор (а-ля Pencil): дерево ops слева, canvas справа, properties panel.
- Field selector: dropdown "какое поле каталога использовать для hero image" (опирается на B7).
- "Save from result" — превращение результата Agent2 в новый preset одной кнопкой.
- Preset versioning + publish/draft.

### Подводные камни

1. **fieldName hardcoded в системных пресетах** (`presets_product.go` содержит `"fieldName": "images"`, `"name"`, `"price"`). Пока B7 не сделан — юзер будет редактировать пресеты, привязанные к cosmetics-полям. Workaround существует (через override ops), но некрасив. **Рекомендую делать B7 до фичи 2.**
2. **Live preview endpoint**: `POST /api/admin/presets/preview` принимает ops + sample data → гоняет engine → возвращает formation. Без этого UX будет "сохранил-обновил-посмотрел" вместо realtime.
3. **Ops валидация**: клиент может сохранить кривые ops (parent не существует, замкнутый ref). Нужен dry-run validator на save.

### Value

Killer-фича. Клиент получает контроль над тем, как выглядит его виджет, не трогая код. Генерация через Agent2 остаётся как fallback — это даёт супер-гибкость без саппорта. Детально — в `PRESET_EDITOR_SPEC.md`.

---

## Фича 1 — Универсальный загрузчик (★★★★★, минимум 2-3 недели)

**Честная оценка: это не "фича", это отдельный трек.** Разбиваю на "MVP для демо" и "полный кейс".

### Что есть

- `project_admin/backend/internal/usecases/import.go` — async pipeline, JSON-only. `Upload()` → goroutine → batch processing → upsert → embeddings (OpenAI text-embedding-3-small) → `GenerateCatalogDigest` для Agent2.
- Enrichment через Haiku (`adapters/anthropic/enrichment_client.go:systemPromptV2`) — **жёстко под cosmetics** (closed list: skin_type, concern, product_form, hardcoded category tree "face-care > cleansing").
- Crawler (`cmd/crawler/main.go`) — 967 heybabescosmetics, JSON-LD extractor универсальный (~40% переиспользуемо), section parsing русскоязычный и cosmetics-specific.
- **НЕТ vision/OCR, НЕТ Excel/PDF parser, НЕТ `catalog.field_definitions`** (это B7).

### Что нужно для "MVP для демо" (~10-12 дней)

1. **B7 fundament**: `catalog.field_definitions` table + domain + prompt generator. **Без этого вся фича живёт только на cosmetics.** 3-5 дней сам по себе.
2. **Parser abstraction**: `Parser interface { Parse(ctx, reader) ([]ImportItem, error) }` + реализации:
   - JSON (уже есть, обернуть)
   - CSV (encoding/csv) — 1 день
   - Excel (.xlsx через excelize) — 1 день
   - PDF (pdfcpu / ledongthuc для текстового; image-PDF требует vision)
3. **LLM-schema inference**: новый usecase "анализирую файл → предлагаю схему (какие поля, какой роли)" → пользователь подтверждает/правит → сохраняется как field_definitions для его tenant. 2-3 дня.
4. **Generic enrichment prompt**: заменить hardcoded cosmetics-prompt на prompt-builder, который берёт field_definitions клиента и просит Haiku извлечь значения. 2 дня.
5. **Category tree extraction**: для любых каталогов LLM строит дерево из данных, пользователь правит. 1-2 дня.
6. **Frontend UX**: step-by-step wizard "загрузи файл → смотри превью → подтверди схему → смотри извлечённые поля → запускай импорт". 3-4 дня.

### Что НЕ успеть за 2-3 недели

- Vision для product photos (если клиент приносит папку с фотками без названий). +1 неделя с OpenAI/Claude Vision.
- OCR для сканированных PDF-каталогов. +3-4 дня.
- Полноценный crawler-generator "дай URL → я сам пойму структуру" — нужна LLM-driven extraction, крайне нестабильно.

### Подводные камни (из аудита)

1. `buildEmbeddingText()` (`import.go:288-318`) жёстко выбирает cosmetics PIM поля для эмбеддингов. Для ноутбуков это не сработает — нужно параметризовать через field_definitions.
2. `MasterProduct` struct в `domain/product.go:35-65` имеет колонки `skin_type TEXT[]`, `concern TEXT[]`, `key_ingredients TEXT[]` — прямая привязка к cosmetics в схеме БД. Нужно мигрировать на JSONB `attributes` + field_definitions метаданные.
3. `catalog_migrations.go:139-206` — ETC жёстких колонок. Миграция может быть болезненной, если 967 products heybabescosmetics уже записаны.
4. `enrichment_client.go:systemPromptV2` — category tree прямо в промпте, для universal нужно передавать tree из БД.

### Value

Критическое. Без этого демо работает только на "heybabescosmetics" и 1-2 клиентов, которые согласятся принести данные в точно таком же формате. С этим — демо работает на любом каталоге, что разблокирует реальные продажи.

---

## Рекомендованный порядок

Если цель "через несколько дней быть sales-ready на 1-2 нишах":

**Неделя 1**:
- **День 1-2**: Фича 3 (Agent2 skip) — быстрая cost-win + merge V4 в main. Это ещё и аргумент для продаж "мы в 2 раза дешевле".
- **День 3-5**: Фича 4 (design system upload). Halloween-demo — WOW для первого звонка.

**Неделя 2**:
- **Дни 6-10**: B7 fundament — `field_definitions` + generic prompt-builder. Без этого всё упирается.

**Неделя 2-3**:
- **Дни 11-15**: Фича 2 MVP (preset editor на базе B7). Этот порядок даёт клиенту "загрузил данные → видишь поля → сам редактируешь виджет".

**Неделя 3-5**:
- **Дни 16-25**: Фича 1 (загрузчик с parser abstraction + LLM schema inference + wizard UX). С B7 этот блок станет в 2 раза дешевле, потому что вся инфраструктура готова.

**Итого: 4-5 недель до sales-ready состояния с full-stack**: "клиент приносит Excel → визард разбирает → видит каталог → переключает тему → сам правит виджет → вставляет скрипт на сайт".

Если надо быстрее — можно обрезать фичу 1 до "Excel + JSON + один общий LLM-schema-inference, без PDF/vision" — это ужимается в неделю после B7.

---

## Главный watchout

**B7 (catalog.field_definitions + role-based slot resolution) — критический блокер.** Он уже в `PRE_LAUNCH_TASKS.md`, но я бы его повысил в приоритете. Он разблокирует:

- Фичу 1 (без него — cosmetics-only)
- Фичу 2 (без него — пресеты привязаны к hardcoded fieldName cosmetics)
- Фичу 4 частично (field_definitions пересекается с тем, какие токены применять к каким полям)

Если B7 не делается — все 3 фичи выше становятся "универсальными в кавычках", и на первом же не-cosmetics клиенте всё поплывёт.

**Второй момент**: фича 2 (редактор пресетов) и фича 4 (design system) вместе создают впечатление "full visual customization platform". Это очень сильный аргумент на демо. Если время ограничено — лучше пожертвовать полнотой фичи 1 (сделать CSV+JSON только), чем отказаться от связки 2+4.
