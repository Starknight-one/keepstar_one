# Prompt cache audit — Agent1 digest restored, Agent2 conversation cache enabled

- **Branch**: `feature/engine-v4`
- **Date (UTC)**: 2026-04-09 01:21
- **Commit**: `403d1fe501cf3f371e9a15307d3249398edff635`
- **Parent**: `e4f9a0d9b3c3e30e666780e4385deb947d4b5f57`

## Context

Задача: понять почему у Agent2 кеш «иногда работает, иногда нет», и почему у
Agent1 «перестал работать кеш и перестало хватать токенов по каталогу». Аудит
через `/debug/traces/?format=json` на проде.

## Findings (audit of 10 recent traces, skipping the 2 latest broken ones)

### Agent1 — **0% hit rate во всех 80 трейсах**
- `cacheWrite=0` и `cacheRead=0` повсеместно. Значит cache_control **не приводит
  к созданию блока** на стороне Anthropic.
- **Root cause**: минимальный размер кешируемого блока для Haiku 4.5 —
  **2048 токенов**. Agent1 префикс = system (2741 chars ≈ ~700 токенов) +
  3 tools (~500–800 токенов) ≈ ~1300–1500 токенов — **ниже порога**, Anthropic
  молча игнорирует `cache_control`.
- Исторически катал. дайджест жил в system prompt и давал вес, который
  проталкивал префикс выше 2048. Коммит `095ee8b` («digest redesign — compact
  format ~300 tokens + one-time delivery at session init») перенёс его в
  conversation_history и ужал до 300 токенов. Оба изменения разом:
  1. Уронили префикс ниже 2048 → кеш отключился.
  2. Сделали дайджест зависимым от вызова `/session/init` — если фронт восстанавливает сессию из localStorage без init, дайджест вообще не попадает в историю → Agent1 теряет знание каталога. Это и есть «перестало хватать токенов».

### Agent2 — **кеш работает корректно, все «промахи» — TTL 5 мин**
Разбор по сессиям (хронологически):

| Сессия | Turn | input | cacheRead | cacheWrite | Gap |
|---|---|---|---|---|---|
| 0377179a | 1 | 684 | 0 | 4489 | первый |
| 0377179a | 2 | 1451 | **4489** ✅ | 0 | +13s |
| 0377179a | 3 | 2294 | **4489** ✅ | 0 | +31s |
| e10889c3 | 1 | 684 | 0 | 5384 | первый |
| e10889c3 | 2 | 5528 | **5384** ✅ | 0 | +25s |
| e10889c3 | 3 | 2645 | 0 | 5384 | +7:24 (TTL истёк) |
| e10889c3 | 4 | 4743 | 0 | 5384 | +5:38 (TTL истёк) |

Внутри активной сессии со 2-го turn'а — 100% hit rate. Система+tools
стабильны: `Agent2ToolSystemPrompt` это `const`, `visual_assembly` tool с
статическим schema, Go детерминированно сортирует map keys при JSON
marshal, `getAgent2Tools()` фильтрует по префиксу `visual_` и возвращает
один элемент. Кода-бага нет.

### Параллельная находка: map-iteration bug в `Registry.GetDefinitions()`
`for _, tool := range r.tools` по `map[string]ToolExecutor` — рандомный
порядок каждый вызов. Для Agent1 это было бы killer-баг для кеша, если бы
его префикс дотягивал до 2048. Для Agent2 не влияло (один tool). Hardening
всё равно нужен — как только дайджест вернётся и префикс перевалит за порог,
map-рандомность стала бы моментально ломать заново созданный кеш.

## Approach

1. **Инлайн дайджеста в Agent1 system prompt.** `buildSystemPromptWithDigest()`
   при первом вызове по данному `TenantSlug` идёт в `catalogPort.GetTenantBySlug
   → GetCatalogDigest → ToPromptText`, склеивает `Agent1SystemPrompt +
   "\n\n<catalog>\n...\n</catalog>\n"` и мемоизирует в `sync.Map[slug]string`.
   Префикс становится per-tenant, но байт-стабильным внутри процесса → cache
   hash совпадает между turn'ами. Фиксит **оба** симптома Agent1.
2. **Удалён seeding дайджеста в `/session/init`.** Был источник dual-path:
   дайджест либо в history (если init вызван), либо отсутствовал вообще.
   Теперь единственный источник — system prompt, гарантированно всегда есть.
3. **`CacheConversation: true` для Agent2.** История (Agent2History, до 4
   сообщений) теперь тоже попадает в кешируемый блок — маленький выигрыш
   бесплатно.
4. **Sort в `Registry.GetDefinitions()`.** `sort.Slice` по `Name`. Hardening
   перед увеличением Agent1 префикса.
5. В `Agent1ExecuteResponse.SystemPrompt` / `SystemPromptChars` теперь пишется
   динамический промпт — чтобы `/debug/traces` показывал реальный размер
   контекста с дайджестом.

## Files changed

| File | Change |
|---|---|
| `project_v4/backend/internal/usecases/agent1_execute.go` | `sync.Map` digest cache, `buildSystemPromptWithDigest()`, использование dynamic prompt в ChatWithToolsCached и в `Agent1ExecuteResponse.SystemPrompt` |
| `project_v4/backend/internal/usecases/agent2_execute.go` | `CacheConversation: len(messages) > 1` в CacheConfig |
| `project_v4/backend/internal/handlers/handler_session.go` | удалён блок seeding'а дайджеста в conversation_history, заменён комментарием |
| `project_v4/backend/internal/tools/tool_registry.go` | `sort.Slice(defs, ByName)` в `GetDefinitions()` + импорт `sort` |

## Verification

**Локально**: `go build ./...` — чисто.

**На проде после деплоя** (`v4-engine-production.up.railway.app`):
```bash
curl -s "https://v4-engine-production.up.railway.app/debug/traces/?format=json&limit=5" \
  | jq '.[0].agent1 | {inputTokens, cacheRead, cacheWrite, systemPromptChars}'
```
Ожидаемые изменения:
- `systemPromptChars` у Agent1 вырастет с `2741` до чего-то в районе `5000–8000`
  (зависит от жирности дайджеста heybabes — 967 продуктов + категории + фильтры
  + бренды + ингредиенты).
- На первом turn'е по тенанту в свежем процессе: `agent1.cacheWrite > 0`.
- На втором+ turn'е в пределах 5 минут: `agent1.cacheRead > 0`, `cacheWrite = 0`.
- `agent1.inputTokens` резко упадёт (c ~4000 до ~300–600) — т.к. жирный префикс
  уедет в `cacheRead` и перестанет биллиться по полной цене.
- Стоимость Agent1 per-turn в пределах сессии ожидается **~10× меньше**
  (cache_read биллится по 0.1× от input price).

**Failure mode**: если у heybabes дайджест окажется слишком компактным и
system+tools всё ещё будет <2048 токенов — кеш опять не активируется. Fallback:
увеличить дайджест (добавить больше top-брендов/ингредиентов в генератор) или
допушить стабильного контента в `Agent1SystemPrompt`. Маловероятно при 967
продуктах, но проверить после деплоя первым же запросом.

## Known gaps / caveats

- **`digestCache` не инвалидируется** до рестарта процесса. Если админ
  перегенерирует catalog digest в рантайме — Agent1 будет работать со старым
  до следующего рестарта Railway. Для MVP приемлемо, позже можно добавить
  `DigestUpdated` hook или TTL.
- **Agent1 cache prefix теперь per-tenant.** Multi-tenant deployments получат
  отдельный cache-блок на каждого активного тенанта. При большом числе
  одновременных тенантов это может увеличить cache_creation траты (но каждый
  отдельный тенант получит такой же выигрыш внутри своей сессии).
- **Hot path stale read**: при восстановлении сессии из localStorage в
  conversation_history всё ещё могут быть старые `<catalog>…` пары (user +
  assistant:ok) от старого seeding'а. Они безвредны (Agent1 их просто увидит
  как часть истории, плюс новый дайджест в system prompt), но занимают
  бесплатные токены. Очистка опциональна — можно сделать миграцию или
  игнорировать (естественно вымоются через TTL сессий).
- **Agent2 CacheConversation эффект** ожидается скромный: Agent2History — до 4
  сообщений (2 turn'а tool_use + tool_result), обычно не более 1–2к токенов.
  Основную экономию по-прежнему даёт system+tools блок.
- **Два последних trace'а** на момент аудита были помечены как «поломка» —
  пользователь просил их не трогать, в аудит они не попали.
