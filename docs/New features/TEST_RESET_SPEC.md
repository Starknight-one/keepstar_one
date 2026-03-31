# Test Reset — Spec

Дата: 2026-03-30
Статус: Planned
Тип: Рефакторинг

---

## Контекст

Текущие unit/integration тесты в `project/backend/` устарели, завязаны на V1 engine, и не отражают реальное поведение системы. Решение: удалить все backend тесты и заново покрывать систему через E2E тесты с единым хранением в `tests/`.

E2E тесты (`tests/e2e_run.py`, `tests/e2e_quick_test.py`, `tests/e2e_recon.py`) — единственные актуальные тесты, **их сохраняем**.

---

## Что удалить

### Backend тесты — 27 файлов, ~5500+ LOC

| Файл | LOC | Что тестирует |
|------|-----|---------------|
| `internal/domain/state_entity_test.go` | 120 | DeltaInfo.ToDelta() |
| `internal/domain/llm_cost_test.go` | 148 | LLM cost calculation |
| `internal/domain/span_test.go` | 159 | Span collection/tracing |
| `internal/domain/catalog_digest_test.go` | 131 | CatalogDigest.ToPromptText() |
| `internal/tools/tool_render_preset_test.go` | 120 | V1 render preset (мертвый код) |
| `internal/tools/rrf_merge_test.go` | 244 | RRF merge algorithm |
| `internal/tools/tool_catalog_search_test.go` | 640 | Catalog search tool |
| `internal/engine/engine_v2_test.go` | 100 | EngineV2 formation |
| `internal/engine/formation_test.go` | 100 | Formation building |
| `internal/engine/formation_fuzz_test.go` | 80 | Property-based fuzzing |
| `internal/usecases/agent1_execute_test.go` | 292 | Agent1 integration |
| `internal/usecases/agent2_execute_test.go` | 516 | Agent2 integration |
| `internal/usecases/cache_test.go` | 300 | Prompt caching chain |
| `internal/usecases/pipeline_mock_llm_test.go` | 207 | Pipeline mock LLM |
| `internal/usecases/tool_execution_integration_test.go` | 213 | Tool execution |
| `internal/usecases/state_rollback_test.go` | 530 | State rollback |
| `internal/usecases/navigation_test.go` | 623 | Navigation unit+integration |
| `internal/usecases/navigation_integration_test.go` | 213 | Navigation DB integration |
| `internal/adapters/postgres/shared_test.go` | 49 | DB connection pooling |
| `internal/adapters/postgres/postgres_session_integration_test.go` | 199 | Session state CRUD |
| `internal/adapters/postgres/postgres_state_test.go` | 100 | StateAdapter |
| `internal/adapters/postgres/catalog_search_relevance_test.go` | 100 | Vector search relevance |
| `internal/adapters/postgres/catalog_digest_test.go` | 100 | Catalog digest |
| `internal/adapters/postgres/postgres_catalog_integration_test.go` | 100 | Catalog integration |
| `internal/handlers/middleware_test.go` | 294 | CORS/tenant middleware |
| `internal/handlers/smoke_test.go` | 328 | HTTP smoke tests |
| `internal/prompts/prompt_analyze_query_test.go` | 51 | Agent1 prompt building |

### Frontend тесты

Нет — в `project/frontend/` нет ни одного application-level теста.

---

## Что сохранить

```
tests/
  e2e_run.py           — Main E2E test runner (pipeline validation)
  e2e_quick_test.py    — Quick validation tests
  e2e_recon.py         — Reconnaissance/discovery tests
  e2e_results.json     — Test results
  e2e_screenshots/     — Screenshot artifacts
```

---

## Что убрать из build/CI

- `Makefile` targets: `test-unit`, `test-integration`, `test-usecase`, `test-llm`, `test-all` — удалить или переписать на новую структуру
- Оставить/добавить target для E2E: `test-e2e` → `cd ../../tests && python e2e_run.py`

---

## Новая структура тестов (план)

Все новые тесты пишутся и хранятся в `tests/`:

```
tests/
  e2e_run.py              — существующий E2E runner
  e2e_quick_test.py       — существующий quick test
  e2e_recon.py            — существующий recon
  e2e_results.json
  e2e_screenshots/
  (будущие тесты по мере надобности)
```

---

## Порядок выполнения

1. Удалить все 27 `*_test.go` файлов из `project/backend/`
2. Обновить `Makefile` — убрать устаревшие test targets
3. Проверить `go build` — убедиться что удаление тестов не ломает сборку
4. Убедиться что E2E тесты в `tests/` проходят

---

## Связь с V1 Engine Removal

Этот рефакторинг логично делать **вместе** с V1 Engine Removal (см. `V1_ENGINE_REMOVAL_SPEC.md`), поскольку:
- 15 из 27 тестов завязаны на V1 код (`NewRegistry`, `PresetRegistry`, `NewAgent2ExecuteUseCase`)
- Обновлять их на V2 — бессмысленная работа если тесты всё равно не отражают реальное поведение
- Чистый срез: удалить V1 + тесты → покрывать E2E с нуля
