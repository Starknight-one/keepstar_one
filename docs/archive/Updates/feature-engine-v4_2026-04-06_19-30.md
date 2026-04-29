# feature/engine-v4 — #4: Explicit `mode` param (rebuild | modify)

**Branch**: `feature/engine-v4`
**Date**: 2026-04-06 19:30 UTC
**Parent**: `d7e2a7f`

---

## Context

Gap #4 из `docs/Updates/feature-engine-v4_2026-04-04_04-07.md` — state carry-over. Раньше `visual_assembly.Execute` пытался сам угадать режим: если первый op — insert widget в `formation`, это build-from-scratch; иначе modify (грузить existing formation и применять ops как дельты). Эвристика ломалась если Agent2 начинал с insert в `$info` старого виджета а потом добавлял новый — детектор срабатывал неправильно, куски прошлой формации оставались в текущем turn'e.

Обсудили варианты: (а) срезать modify-путь насовсем и всегда rebuild, (б) явный флаг `mode` в тулу. Пользователь выбрал (б) — modify-путь очевидно полезен для дешёвых cosmetic tweaks ("сделай цену красной"), его срезать жалко. Просто убрать магию и заставить Agent2 явно решать каждый turn.

---

## Approach

### Tool schema — новый required param `mode`

`project_v4/backend/internal/tools/tool_visual_assembly.go`:

```go
"mode": map[string]interface{}{
    "type":        "string",
    "enum":        []string{"rebuild", "modify"},
    "description": "REQUIRED. \"rebuild\" discards the previous formation ... \"modify\" loads existing formation ... No default.",
},
// ...
"required": []string{"mode"},
```

В `Execute()`:
- Проверка наличия mode в самом начале — если нет или невалидный, возвращаем понятную ошибку.
- Убран старый `buildFromScratch` автодетектор.
- Загрузка existing formation теперь происходит **только** в `mode == "modify"`.
- Нулевая конфигурация при `rebuild` — движок получает пустую формацию и строит с нуля. Никакого clearing кода не нужно, `engineInput.Formation == nil` означает start fresh.
- Дополнительный guard: `preset` нельзя передавать в `modify` (пресеты раскрываются только на rebuild path).

### Prompt — новая секция MODE + пересмотр DECISION RULES

`project_v4/backend/internal/prompts/prompt_compose_widgets.go`:

1. Новая секция **MODE — you MUST pick one every turn** после HOW IT WORKS. Объясняет когда rebuild, когда modify, правило тайбрейка ("если можно сказать 'keep what's on screen but ...' → modify").
2. Все примеры (`preset only`, `preset + overrides`, `product card grid`, `product detail`, `compact rows`, `empty_not_found`) обновлены — добавлен `mode: "rebuild"` явно.
3. Новый пример **"modify existing formation"** — показывает minimal modify turn: `mode: "modify"` + 1 update op, без layout/columns.
4. DECISION RULES переписаны: первое правило теперь «ALWAYS pick a mode», старые три правила про data_change × formation_tree матрицу заменены на одно — "data_change → обычно rebuild; tweak без data_change → modify".
5. PARAMETERS: добавлен `mode` первой строкой. `preset` помечен как "only valid in rebuild mode".
6. ANTI-PATTERNS: добавлены "Do NOT forget mode" и "Do NOT pass preset in modify mode".

### Что НЕ тронуто

- `formation_tree` всё ещё строится и передаётся Agent2 — в modify режиме он критичен (targeting по IDs).
- Usecases (`navigation_back`, `navigation_expand`, `action_view`, `pipeline_execute`) не вызывают `VisualAssemblyTool.Execute` — они конструируют `engineInput` напрямую и зовут `engine.Execute()`. Не затронуты.
- Frontend — рендерит только `formation`, mode его не касается.
- `ExpandWildcardOps` — продолжает вызываться в modify path, работает как раньше.
- Пресеты и реестр — без изменений.
- Replicate/limit/layout параметры — без изменений.

---

## Behaviour delta

### Пример 1 — "покажи крема" (свежий поиск)

**До**: Agent2 конструировал ops с widget insert → автодетектор ловил → строил с нуля.
**После**: Agent2 явно пишет `mode: "rebuild", preset: "product_card"`. Семантически то же самое, но без магии.

### Пример 2 — "сделай цену красной" (на текущем экране)

**До**: Agent2 слал delta ops `[{update price red}]`. Автодетектор видел "не widget в начале" → грузил existing formation → применял дельту. Работало.
**После**: Agent2 слит `mode: "modify", ops: [{update price red}]`. Тот же результат, но явно.

### Пример 3 — carry-over bug

**До**: если Agent2 по ошибке начинал с insert в `$info` прошлого виджета + добавлял новый виджет → автодетектор видел "не widget первым" → грузил старую формацию → новый виджет добавлялся поверх старой. Экран показывал вчерашнее + сегодняшнее.
**После**: Agent2 обязан явно сказать rebuild или modify. В rebuild никаких прошлых виджетов не будет. В modify новый виджет реально добавится поверх — но это явное решение Agent2, не сюрприз.

### Пример 4 — Agent2 забыл `mode`

**До**: не применимо.
**После**: tool возвращает `invalid mode: "" — must be "rebuild" or "modify"`, Anthropic tool_use error прокидывается в следующий turn → Agent2 исправится на повторе. Alternative: LLM увидит JSON schema `required: ["mode"]` и не отправит без mode в первую очередь.

---

## Token expectations

Не измеряли через API (решили что трейсов достаточно), но качественно:

- **rebuild turns**: примерно как сейчас (preset + layout/columns). +5 токенов на `mode: "rebuild"` строку.
- **modify turns**: примерно как сейчас. +5 токенов на `mode: "modify"`.
- **Input промпт**: стал чуть длиннее из-за секции MODE (~100 токенов), но **системный промпт кешируется** → amortized near-zero после warm-up.
- **Ошибки типа carry-over, где Agent2 отправлял лишние ops**: уходят → экономия в худших случаях.

Если захочется реального замера — смотреть `/debug/traces/` waterfall, где Usage уже собирается в `LLMUsageWithCache()`.

---

## Files changed

| File | Change |
|---|---|
| `project_v4/backend/internal/tools/tool_visual_assembly.go` | `mode` в schema (required), parsing в Execute, mode-gated formation load, preset-in-modify guard |
| `project_v4/backend/internal/prompts/prompt_compose_widgets.go` | Секция MODE, обновлены все примеры, DECISION RULES переписаны, ANTI-PATTERNS обновлены, PARAMETERS обновлены |
| `docs/Updates/feature-engine-v4_2026-04-06_19-30.md` | **NEW** — этот файл |

Итого: 2 code files + 1 doc.

---

## Verification

### Локально

```bash
cd project_v4/backend
go build ./...                             # ок
go test ./...                              # все существующие тесты passes (cached)
```

### На проде (после деплоя)

1. `/version` → новый build hash.
2. **rebuild happy path**: "покажи крема" → в трейсе Agent2 tool input содержит `mode: "rebuild"` + `preset: "product_card"`. Грид рендерится.
3. **modify happy path**: после (2) → "сделай цену красной" → трейс показывает `mode: "modify"` + 1 update op. Цена красная, остальное не меняется.
4. **modify-then-rebuild**: после (3) → "покажи детали первого" → `mode: "rebuild"` + `preset: "product_detail"`. Грид полностью заменяется detail виджетом — carry-over нет.
5. **preset в modify — ошибка**: (если удастся спровоцировать) Agent2 отправляет `mode: "modify", preset: "product_card"` → tool возвращает "preset can only be used with mode=rebuild", Agent2 пересобирает.
6. **mode отсутствует**: в новом промпте это anti-pattern, schema `required: ["mode"]` заставит LLM положить mode. Если каким-то чудом не положит — понятная error message.

---

## Known gaps / caveats

1. **Prompt cache invalidation** — изменение системного промпта ломает Anthropic prompt cache. Первый запрос после деплоя дороже, дальше warm-up.
2. **Замер токенов не автоматизирован** — дев-дока опирается на качественную оценку и существующие debug трейсы. Если захочется CI-friendly benchmark — отдельная задача (real-API test harness с `LLMResponse.Usage`).
3. **modify mode может застрять на мусорной формации** — если state.Current.Template["formation"] содержит что-то битое, modify ops применяются к битой базе. Это не новая проблема, была и до mode флага. Ложится в будущий gap "state sanitization".
4. **Agent2 может ошибиться с выбором mode** — пример: пользователь сказал "сделай цену красной" но Agent2 выбрал rebuild. Результат визуально может быть тем же (preset применил red override), но это лишние токены на output. Рассчитываем на инструкции в MODE секции + будем смотреть трейсы.
5. **Frontend не знает про mode** — и правильно, формация на выходе одинаковая. Но если потом захотим показывать в UI "этот turn был modify" для дебага — надо будет прокидывать.
