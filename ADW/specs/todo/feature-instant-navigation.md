# Feature: Instant Navigation (Decision Tree + Pre-built Formations)

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

## Ключевой инсайт: Дерево решений

У пользователя в каждый момент **ограниченный набор действий**:

- **Клик по карточке** → детализация (детерминировано, LLM не нужен)
- **Back** → предыдущий экран (детерминировано)
- **Чат** → новый запрос (недетерминировано, нужен LLM)

Детерминированные действия можно **предсобрать**. Чат — единственное что требует round-trip, и это ожидаемо (юзер набирает текст, есть время на ответ).

Каждый ответ бэкенда создаёт **узел дерева**. У узла — предсобранные потомки (один уровень вперёд). Клик по карточке = переход к предсобранному потомку, мгновенно. Чат = новая ветка, round-trip, но результат создаёт новый узел со своими предсобранными потомками.

```
Chat: "корейские туши бренд X"
│
└── [Node A] Grid 6 товаров
    │   adjacent: Detail 1..6 [PRE-BUILT]
    │
    ├── Click карточка 3 →
    │   └── [Node B] Detail товара 3      ← instant, из adjacent
    │       │   Back → [Node A] [CACHED в стеке]
    │       │
    │       ├── Chat: "покажи похожие" →
    │       │   └── [Node C] Grid 4 похожих    ← round-trip (LLM)
    │       │       │   adjacent: Detail 1..4 [PRE-BUILT]
    │       │       │   Back → [Node B] [CACHED]
    │       │       └── ...
    │       │
    │       └── Back → [Node A] Grid         ← instant, из стека
    │
    └── Chat: "а бренд Y" →
        └── [Node D] Grid 5 товаров бренда Y   ← round-trip (LLM)
            │   adjacent: Detail 1..5 [PRE-BUILT]
            │   Back → [Node A] [CACHED]
            └── ...
```

### Три уровня стоимости перехода

| Что происходит | Стоимость | Пример |
|----------------|-----------|--------|
| Клик / Back (детерминированный переход) | ~0ms, из кэша | Expand карточки, back |
| Новый поиск (данные меняются, пресет тот же) | Быстрее полного цикла — прогнать данные через существующий пресет | "Покажи бренд Y" |
| Новый запрос на отображение (данные + пресет меняются) | Полный цикл Agent1 + Agent2 | "Покажи таблицей с характеристиками" |

### Оптимизация: структура и данные независимы

Пресет = трафарет. Данные = краска. Если пользователь меняет только данные ("другой бренд"), пресет переиспользуется. Полная пересборка нужна только когда меняется сам пресет ("покажи по-другому").

```
Запрос 1: Grid product_grid + данные бренда X → Details product_detail + данные X [PRE-BUILT]
Запрос 2: "бренд Y" → Grid product_grid (ТОТ ЖЕ пресет) + данные Y → Details product_detail (ТОТ ЖЕ) + данные Y
           ↑ пресет уже известен, прогнать новые данные через него — дёшево
```

### Что предсобирать для каждого типа узла

| Тип узла | Adjacent (pre-built) | Back |
|----------|---------------------|------|
| Grid / List / Carousel | Detail для каждого entity | Из стека (cached) |
| Detail | Ничего нового — next/prev уже в adjacent родительского грида | Из стека (cached) |

Один уровень предсборки вперёд достаточен. После каждого перехода можно досборать следующий уровень в background если понадобится.

## Архитектурный принцип

> **Backend генерирует — frontend рендерит.** FE = тупой рендерер, никакой formation-логики на клиенте. Вся сборка виджетов, atoms, presets — только на backend.

Мы НЕ дублируем preset engine на JS. Мы НЕ собираем formations на клиенте. Любой view всегда приходит готовым с backend. Frontend только кэширует и переключает готовые JSON-ы.

## Механизмы

### 1. Formation Stack (Back = мгновенный)

FE хранит стек ранее полученных formations. Чистый кэш — предыдущие JSON-ответы.

**Правила:**
- Pipeline response (новый чат-запрос) → **НЕ очищать стек** (это новая ветка от текущего узла, back должен работать)
- Expand → **push** текущую formation в стек
- Back → **pop** из стека, render мгновенно
- Background sync: fire-and-forget POST для синхронизации state на бэкенде

**Безопасность:** Backend state = source of truth. Стек — UX-оптимизация. При рассинхронизации — следующий pipeline call всё пересобирает.

### 2. Adjacent Formations (Expand = мгновенный)

При каждом ответе бэкенд **предсобирает** detail formations для всех entities текущей formation:

```json
{
  "formation": { "mode": "grid", "widgets": [...] },
  "adjacentFormations": {
    "product-123": { "mode": "single", "widgets": [...] },
    "product-456": { "mode": "single", "widgets": [...] }
  }
}
```

При expand:
1. FE ищет entity в `adjacentFormations` → мгновенный render
2. Push текущей formation в стек (для back)
3. Fire-and-forget POST для state sync

Adjacent formations **не выбрасываются** при переходе в detail — они остаются для next/prev навигации между товарами.

### Связь с дельтами и глубина навигации

Каждый узел дерева = состояние стейта. Переход между узлами = дельта.

**Два механизма навигации по глубине:**

| Глубина | Механизм | Скорость | Где живёт |
|---------|----------|----------|-----------|
| 1-2 шага back | Formation stack (кэш JSON) | <16ms, instant | FE, в памяти |
| 3+ шагов (глубокая навигация) | Дельты — восстановление стейта по step | Round-trip к BE | BE, `chat_session_deltas` |

Formation stack = быстрый кэш для типичного поведения (открыл-закрыл, открыл другой). Покрывает ~90% навигации.

Дельты = полная машина времени. Для глубокой навигации ("верни меня к тому что было 5 шагов назад") бэкенд восстанавливает стейт из истории дельт. Этот механизм уже реализован (`rollback` delta type, `UpdateState` для bulk restore).

- Expand (клик) → дельта `push` в view + `update` template
- Back → дельта `pop` из view + `update` template
- Chat (новая ветка) → дельты `add` data + `update` template
- Deep back (3+ шагов) → BE находит целевой step в дельтах → восстанавливает стейт

Дельты образуют полную историю навигации по дереву.

---

## План реализации

### Phase 1: Formation Stack (Back = instant)

**Scope:** Только back. Zero BE changes.

#### FE: useFormationStack hook

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
export function syncExpand(sessionId, entityType, entityId) {
  fetch('/api/v1/navigation/expand', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sessionId, entityType, entityId }),
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

#### FE: Интеграция

- Expand (пока Phase 1 — expand ещё через сеть) → `stack.push(currentFormation)`
- Back → `formation = stack.pop()` → render → `syncBack(sessionId)`
- Pipeline response → `stack.push(currentFormation)` (новая ветка, back к текущему)

#### Верификация Phase 1
- Back transition < 16ms
- Цепочки: grid→detail→back, grid→detail→chat→grid→back→back
- Pipeline call после back — state корректен
- Back при пустом стеке — disabled/noop

---

### Phase 2: Adjacent Formations (Expand = instant)

#### BE: Pre-build adjacent formations

**Файл:** `project/backend/internal/handlers/handler_pipeline.go`

```go
type FormationResponse struct {
    Formation           *domain.FormationWithData            `json:"formation"`
    AdjacentFormations  map[string]*domain.FormationWithData `json:"adjacentFormations,omitempty"`
    SessionID           string                               `json:"sessionId"`
}
```

**Файл:** `project/backend/internal/usecases/pipeline_execute.go`

После Agent2 собрал formation, если mode != single:

```go
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
    // same for services
    return result
}
```

#### BE: Sync-only mode

**Файл:** `project/backend/internal/handlers/handler_navigation.go`

Новый query param `?sync=true`:
- Backend выполняет state update (ViewStack push/pop, deltas)
- Response: `{ "ok": true }` (без formation)

#### FE: Instant Expand

1. Ищем `adjacentFormations[entityId]`
2. Найден → `stack.push(currentFormation)` → render → `syncExpand(sessionId, entityType, entityId)`
3. НЕ найден → fallback на обычный POST expand

#### Верификация Phase 2
- Expand transition < 16ms (когда adjacentFormations есть)
- Detail view идентичен backend-rendered
- Pipeline response size рост < 50% (gzip)
- Backend state синхронизирован
- Fallback работает

---

## Payload analysis

Текущий pipeline response (grid, 10 products): ~15KB (gzip ~3KB)

С adjacentFormations (10 detail formations):
- Каждый detail widget: ~1.5KB
- 10 details: ~15KB additional
- Total: ~30KB (gzip ~5KB)

**Рост: +2KB gzipped.** Приемлемо.

## Риски и митигация

| Риск | Митигация |
|------|-----------|
| adjacentFormations увеличивает response | gzip; лимит 10 entities; для 20+ — отключить eager |
| Background sync fails | Следующий pipeline call пересобирает state. Self-healing. |
| Race: expand→back→expand быстро | Formation stack детерминирован. Debounce sync. |
| Expand из карусели (не grid) | adjacentFormations одинаково для grid/list/carousel |

## Метрики успеха

| Метрика | До | После |
|---------|-----|-------|
| Back latency | 100-300ms | < 16ms (Phase 1) |
| Expand latency | 100-300ms | < 16ms (Phase 2) |
| Network calls (back) | 1 blocking | 1 fire-and-forget |
| Network calls (expand) | 1 blocking | 1 fire-and-forget |
| Response size (gzip) | ~3KB | ~5KB |

## Не в скоупе

- Client-side formation building (нарушает архитектуру)
- Глубина предсборки > 1 уровня (достаточно одного)
- SSE/WebSocket
- Offline mode
- Pre-fetch on hover (Phase 3)

## Зависимости

- Phase 1 (formation stack) — **zero BE changes**, деплоится независимо
- Phase 2 (adjacent formations) — BE + FE, деплоятся вместе
