# V2 Engine — Deferred Work

Дата обновления: 2026-03-20.

---

## DONE — V1→V2 Tool Schema (Участок 2, 2026-03-20)

~~Сейчас Agent2 видит V1 tool schema (18 параметров), между ними `convertV1ParamsToV2` костыль.~~

**Сделано:**
- V2-нативная tool schema с `atoms` параметром
- `parseV2Input()` — прямой парсинг без bridge
- `convertV1ParamsToV2` удалён
- V1 код изолирован в `tool_visual_assembly_v1.go`
- Промпт выровнен с schema (direction добавлен, display_meta убран)

---

## 1. Comparison/Table через V2 движок

Сейчас `ComparisonTemplate.jsx` — legacy компонент который **сам** решает как рендерить (собирает поля, строит CSS grid). V2 движок не контролирует его — просто шлёт `mode: "comparison"` и виджеты.

**Что нужно:**
- V2 preset для comparison (layout tree вместо hardcoded frontend)
- Table layout через LayoutNode (rows/columns)
- Cap на количество виджетов (V1 делал products[:4])
- Убить `ComparisonTemplate.jsx`, рендерить через `LayoutTreeRenderer`

**Приоритет:** Высокий — comparison/table единственные layout modes вне контроля движка.

---

## 2. Compose в V2

V1 `compose` параметр (multi-section formations) не портирован в V2. Сейчас `compose` есть только в V1 schema.

**Что нужно:**
- Добавить `compose` в V2 schema и `AgentInstructions`
- `parseV2Input` → парсинг compose секций
- V2 engine → multi-section formation building
- Промпт Agent2 → примеры с compose

**Приоритет:** Средний — нужен для сложных UI (каталог + фильтры + рекомендации в одном ответе).

---

## 3. V1 Code Removal

V1 engine, V1 presets, V1 constraints, compat converters — всё сосуществует с V2.

**Почему отложено:** V2 должен быть полностью стабилен и протестирован прежде чем удалять V1 fallback. V1 используется testbench, navigation, debug.

**~4,100 LOC к удалению:**

| Блок | LOC | Файлы |
|------|-----|-------|
| V1 tool path | ~450 | `tool_visual_assembly_v1.go` (целый файл) |
| V1 presets | 354 | 3 файла в `presets/` |
| V1 engine functions | ~580 | `formation.go`, `assembly.go`, `constraints.go`, `layout.go` |
| V1 compat | 172 | `engine/compat.go` (целый файл) |
| V1 tool render | 241 | `tool_render_preset.go` (целый файл) |
| V1 промпты | 248 | `prompt_compose_widgets.go` (Agent2SystemPrompt + Builder v1) |
| V1 registry | 18 | `tool_registry.go` (NewRegistry) |
| Frontend V1 templates | 2,237 | 5 JSX + 5 CSS + AtomRenderer + Atom.css |

**Порядок:**
1. Перевести testbench/navigation/debug на V2
2. Убрать V1 backend код
3. Убрать `engine/compat.go` когда фронтенд полностью на V2 (сейчас нужен — `WidgetV2ToLegacy`)
4. Убрать V1 frontend templates
5. Убрать `ENGINE_VERSION` env var, сделать V2 единственным

**Приоритет:** Средний — после стабилизации V2 в production.

---

## 4. Visual Container Operations

Из спеки: `background`, `border`, `shadow`, `padding`, `borderRadius`, `opacity` — 6 операций.

**Что нужно:**
- `ContainerStyle` struct в domain (background, border, shadow, padding, borderRadius, opacity)
- Поле в LayoutNode или AtomV2
- Design tokens: `shadowScale`, `borderScale`
- Frontend: CSS mapping
- Tool schema: добавить в atoms override

**Приоритет:** Низкий — CSS дефолты достаточны для текущих layout'ов.

---

## 5. Interactive Wrappers

Из спеки: `input`, `switch`, `slider`, `checkbox`, `radio` — 5 интерактивных обёрток.

**Почему отложено:** нет форм в продукте. Виджет — read-only каталог. Нужны когда появятся: фильтры в чате, quantity picker, настройки.

**Что нужно:**
- Добавить в `WrapperConfig.Type` новые значения
- Frontend: InputWrapper, SwitchWrapper и т.д.
- Engine: правила rigidity для interactive (всегда locked)
- Tool schema: параметр для указания interactive wrapper

**Приоритет:** Низкий — когда появятся формы/фильтры.

---

## 6. Advanced Group Wrappers

Из спеки: `accordion`, `tabs`, `dropdown`, `modal`, `popover`, `tooltip` + ещё.

**Почему отложено:** хватает `collapse` + `carousel` (уже в AutoLayout). Tabs/accordion нужны для detail view с 10+ секций.

**Что нужно:**
- Расширить `LayoutNode.GroupWrapper` enum
- Frontend: TabsGroupWrapper, AccordionGroupWrapper
- Engine: правила когда автоматически применять (>5 секций → tabs)

**Приоритет:** Низкий — когда detail view станет сложнее.

---

## 7. Icon Operations

Из спеки: `iconSize`, `iconColor`, `iconStyle` — 3 операции.

**Почему отложено:** в каталоге нет иконок. Появятся когда добавим: category icons, brand logos, action icons.

**Что нужно:**
- `IconStyle` struct в domain
- Поле в AtomV2 (аналогично MediaStyle)
- Frontend: SVG/emoji рендер с токенами
- Design tokens: iconSize map уже есть в tokens.go

**Приоритет:** Низкий.

---

## 8. Sizing Operations (min/max, overflow)

Из спеки: `minWidth`/`maxWidth`/`minHeight`/`maxHeight`, `overflow` strategy.

**Почему отложено:** BudgetDown/NeedsUp + junction rules уже обрабатывают overflow. Explicit min/max нужны для custom layouts.

**Приоритет:** Не планируется — CSS дефолты достаточны.

---

## 9. Container Operations (contentFit, margin)

Из спеки: `contentFit`, `margin`.

**Почему отложено:** CSS дефолты достаточны.

**Приоритет:** Не планируется.

---

## Приоритет возврата

```
Ближайшее (Участок 3):
  1. Comparison/Table через движок  ← единственные layout modes вне контроля V2
  2. Compose в V2                   ← multi-section formations
  3. V1 Code Removal                ← ~4100 LOC чистка

Когда появится потребность:
  4. Visual Container Operations    ← background, border, shadow
  6. Advanced Group Wrappers        ← tabs/accordion
  7. Icon Operations                ← когда иконки в каталоге
  5. Interactive Wrappers           ← когда формы/фильтры

Не планируется:
  8. Sizing Operations
  9. Container Operations
```
