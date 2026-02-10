# Feature: Instant Navigation (Client-Side Transitions)

**Статус:** Todo
**Приоритет:** High (UX-critical)
**Ветка:** `feature/instant-navigation`

## Проблема

Текущий flow навигации — синхронный round-trip на каждое действие:

```
[User clicks product] → POST /navigation/expand → Backend builds formation → JSON → FE renders
[User clicks back]    → POST /navigation/back   → Backend rebuilds prev   → JSON → FE renders
```

Задержка: **100-300ms** (сеть + preset build + state read/write). Для пользователя это ощутимый лаг — особенно на expand/back, которые **детерминированы** (LLM не нужен).

## Архитектурный принцип

> **Backend генерирует — frontend рендерит.** FE = тупой рендерер, никакой formation-логики на клиенте. Вся сборка виджетов, atoms, presets — только на backend.

Это значит: мы НЕ дублируем preset engine на JS. Мы НЕ собираем formations на клиенте. Любой view (grid, list, carousel, detail, single) всегда приходит готовым с backend.

## Анализ текущего состояния

| Компонент | Состояние |
|-----------|-----------|
| `FormationWithData` | Backend собирает, FE рендерит. Только текущий view (grid OR detail, не оба) |
| `ViewStack` (backend) | Хранит snapshots для back, но клиент его не знает |
| Preset definitions | Живут только на backend |
| Expand/Back usecases | Детерминированные (без LLM), ~5ms на backend, но +100-300ms сеть |

**Ситуаций много:** detail первым → grid → expand → list → back → carousel → back → back. Переходы непредсказуемы.

## Решение: Formation Cache + Eager Detail Formations

Два независимых механизма, оба сохраняют принцип "FE = рендерер":

### 1. Formation Stack на клиенте (Back = мгновенный)

FE хранит стек ранее полученных formations. Это **чистый кэш** — никакой логики, просто предыдущие JSON-ответы.

```
User видит grid → clicks expand → FE запоминает grid formation в стек
User видит detail → clicks back → FE достаёт grid из стека → render

Работает для ЛЮБЫХ переходов:
  grid → detail → back ✓
  detail → grid (pipeline) → list (pipeline) → back → back ✓
  carousel → detail → back ✓
```

**Правила стека:**
- Pipeline response (новый запрос) → **очистить стек** (новый контекст)
- Expand → **push** текущую formation в стек
- Back → **pop** из стека, render мгновенно
- Background sync: fire-and-forget POST на backend для синхронизации state

**Почему это безопасно:** Backend state остаётся source of truth. При следующем pipeline call — state пересобирается с нуля. Стек — это просто UX-оптимизация.

### 2. Eager Detail Formations (Expand = мгновенный)

При pipeline response бэкенд **предсобирает** detail formations для всех entities в текущей grid/list:

```json
{
  "formation": { "mode": "grid", "widgets": [...] },
  "adjacentFormations": {
    "product-123": { "mode": "single", "widgets": [{ "template": "ProductDetail", ... }] },
    "product-456": { "mode": "single", "widgets": [{ "template": "ServiceDetail", ... }] }
  },
  "sessionId": "..."
}
```

При expand:
1. FE ищет entity в `adjacentFormations` → **мгновенный render** (< 1ms, swap JSON)
2. Push текущей formation в стек (для back)
3. Fire-and-forget POST на backend для state sync

**Если formation текущая — single/detail (без adjacentFormations):**
- Expand не применим (мы уже в detail)
- Back = pop из стека

**Если formation — grid/list/carousel:**
- adjacentFormations содержит detail для каждого entity
- Expand = lookup по ID

### Почему это работает для всех ситуаций

| Ситуация | Back | Expand |
|----------|------|--------|
| Pipeline → grid | стек пуст, back disabled | adjacentFormations → instant |
| Grid → expand → detail | pop grid из стека → instant | не применим (уже detail) |
| Pipeline → detail (один продукт) | стек пуст, back disabled | не применим |
| Grid → expand → pipeline (новый запрос) → list | стек очищен (новый контекст) | adjacentFormations → instant |
| Grid → expand → back → expand другой | pop grid → instant; expand → adjacentFormations (всё ещё в кэше) → instant | |

---

## План реализации

### Phase 1: Formation Stack (Back = instant)

**Scope:** Только back. Минимальные изменения, zero BE changes.

#### FE: Formation Stack hook

**Файл:** `project/frontend/src/features/chat/model/useFormationStack.js` (NEW)

```js
import { useState, useCallback } from 'react';

export function useFormationStack() {
  const [stack, setStack] = useState([]);

  const push = useCallback((formation) => {
    setStack(prev => [...prev, formation]);
  }, []);

  const pop = useCallback(() => {
    let result = null;
    setStack(prev => {
      if (prev.length === 0) return prev;
      result = prev[prev.length - 1];
      return prev.slice(0, -1);
    });
    return result;
  }, []);

  const clear = useCallback(() => setStack([]), []);

  const canGoBack = stack.length > 0;

  return { push, pop, clear, canGoBack };
}
```

#### FE: Background Sync

**Файл:** `project/frontend/src/features/chat/api/backgroundSync.js` (NEW)

```js
export function syncExpand(sessionId, entityId) {
  fetch('/api/v1/navigation/expand', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sessionId, entityId }),
    keepalive: true,
  }).catch(() => {});
}

export function syncBack(sessionId) {
  fetch('/api/v1/navigation/back', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sessionId }),
    keepalive: true,
  }).catch(() => {});
}
```

#### FE: Интеграция в chat flow

При получении pipeline response → `stack.clear()` (новый контекст).
При expand (пока Phase 1 — expand всё ещё через сеть) → `stack.push(currentFormation)`.
При back → `formation = stack.pop()` → render → `syncBack(sessionId)`.

#### Верификация Phase 1
- Back transition < 16ms
- Работает для цепочек: grid→detail→back, grid→detail→detail→back→back
- Pipeline call после back — state корректен
- Back при пустом стеке — disabled/noop

---

### Phase 2: Eager Detail Formations (Expand = instant)

#### BE: Pre-build adjacent formations

**Файл:** `project/backend/internal/handlers/handler_pipeline.go`

Добавить `AdjacentFormations` в response:

```go
type FormationResponse struct {
    Formation           *domain.FormationWithData            `json:"formation"`
    AdjacentFormations  map[string]*domain.FormationWithData `json:"adjacentFormations,omitempty"`
    SessionID           string                               `json:"sessionId"`
    // ...existing fields
}
```

**Файл:** `project/backend/internal/usecases/pipeline_execute.go`

После Agent 2 собрал основную formation, если mode = grid/list/carousel:

```go
// Pre-build detail formations for all entities (for instant client-side expand)
if formation.Mode != "single" {
    adjacentFormations = buildAdjacentDetails(state, presetRegistry)
}
```

**Файл:** `project/backend/internal/usecases/pipeline_helpers.go` (NEW)

```go
func buildAdjacentDetails(state *domain.SessionState, presets *presets.PresetRegistry) map[string]*domain.FormationWithData {
    result := make(map[string]*domain.FormationWithData)
    for _, p := range state.Current.Data.Products {
        preset := presets.Get("product_detail")
        formation := tools.BuildFormation(preset, []domain.Product{p}, "single", "large")
        result[p.ID] = formation
    }
    // same for services if present
    return result
}
```

#### BE: Sync-only mode для expand/back

**Файл:** `project/backend/internal/handlers/handler_navigation.go`

Новый query param `?sync=true`:
- Backend выполняет state update (ViewStack push/pop, deltas)
- Response body: `{ "ok": true }` (без formation — экономит сериализацию)

#### FE: Instant Expand

При expand:
1. Ищем `adjacentFormations[entityId]`
2. Если найден → `stack.push(currentFormation)` → render → `syncExpand(sessionId, entityId)`
3. Если НЕ найден (edge case) → fallback на обычный POST expand

#### Верификация Phase 2
- Expand transition < 16ms (когда adjacentFormations есть)
- Detail view идентичен backend-rendered (snapshot test)
- Pipeline response size рост < 50% (gzip)
- Backend state синхронизирован (verify via /debug/traces)
- Fallback работает когда adjacentFormations отсутствует

---

## Payload analysis

Текущий pipeline response (grid, 10 products): ~15KB (gzip ~3KB)

С adjacentFormations (10 detail formations):
- Каждый detail widget: ~1.5KB (atoms: gallery, title, price, brand, category, rating, stock, description, specs)
- 10 details: ~15KB additional
- Total: ~30KB (gzip ~5KB)

**Рост: +2KB gzipped.** Приемлемо.

---

## Риски и митигация

| Риск | Митигация |
|------|-----------|
| adjacentFormations увеличивает response | gzip; лимит 10 entities; для 20+ — отключить eager |
| Background sync fails | При следующем pipeline call — state пересобирается. Self-healing. |
| Race: expand→back→expand быстро | Formation stack + debounce sync. Стек детерминирован. |
| Backend добавил новый preset/поле | adjacentFormations всегда актуальны — собираются при каждом pipeline call |
| Expand из карусели (не grid) | adjacentFormations работает одинаково для grid/list/carousel |

## Метрики успеха

| Метрика | До | После |
|---------|-----|-------|
| Back latency (user-perceived) | 100-300ms | < 16ms (Phase 1) |
| Expand latency (user-perceived) | 100-300ms | < 16ms (Phase 2) |
| Network calls for back | 1 blocking | 1 fire-and-forget |
| Network calls for expand | 1 blocking | 1 fire-and-forget |
| Pipeline response size (gzip) | ~3KB | ~5KB |

## Не в скоупе

- SSE/WebSocket для real-time push
- Client-side search/filtering (всегда через pipeline)
- Client-side formation building (нарушает архитектуру)
- Offline mode / Service Worker
- Pre-fetch on hover (Phase 3 — можно добавить позже как оптимизацию)

## Зависимости

- Phase 1 (formation stack) — **zero BE changes**, деплоится независимо
- Phase 2 (eager formations) — BE + FE, деплоятся вместе
