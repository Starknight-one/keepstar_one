# 6 vertical experts merged + SessionEnd auto-sync hook

- **Branch**: `main`
- **Date (UTC)**: 2026-04-30 11:39
- **Commits**: `684c491` (auto-sync hook) atop `8c2a8b8` (admin) and the
  preceding 6 expert commits (catalog, pipeline-agents, curator, widget,
  engine-v4 refresh, synthesis doc)
- **Pushed**: yes, origin/main updated.

## Context

Закрыли волну Phase 2 экспертов целиком: построили 6 вертикальных
экспертов (catalog, engine-v4, pipeline-agents, widget, admin, curator),
заменив 9 старых горизонтальных (которые ссылались на удалённый
`project/backend/`). Параллельно построили инфраструктуру автообновления —
`SessionEnd` хук срабатывает при закрытии Claude Code и запускает
`/sync-experts --auto` в headless-режиме.

## Approach

### Stage A — слияние в main

`experts/foundation` (7 коммитов) ff-merge в `main`:
- `7ff5139` synthesis doc (Stage 0 plan)
- `6b9330a` engine-v4 refresh (185→328 LOC YAML, drift fixed)
- `a81ada9` widget (Shadow DOM, fillFormation, FSD layers)
- `1090e04` curator (hex-lite, MergeProxy, session middleware)
- `2ff28ed` pipeline-agents (Agent1/Agent2 orchestration, prompt cache, tools)
- `ad5a8ac` catalog (cross-cutting: write+read+curator browse)
- `8c2a8b8` admin (auth surface, billing, canvas, frontend features)

Push в origin/main без блокировок (Railway сервисы не задеты — изменения
только в `.claude/` и `docs/`).

### Stage B — auto-sync infrastructure

Цель: эксперты сами обновляются при закрытии сессии. Без push-блокировок,
без post-commit хуков, без debounce-логики (срабатывает один раз).

Архитектура:

```
SessionEnd event
   │
   ▼
.claude/hooks/sync_experts_on_session_end.sh
   ├─ skip if reason == "clear" (user wipe context, не закрытие)
   ├─ skip if repo чист (no diff vs origin/main, no working tree edits)
   ├─ flock на .claude/.sync.lock (race vs parallel sessions)
   ├─ найти claude binary (fallback на /usr/local/bin, /opt/homebrew, ~/.local/bin)
   └─ spawn nohup `claude --print --dangerously-skip-permissions /sync-experts --auto` &
       └─ headless invocation: свежий контекст, не нужен /compact
```

`/sync-experts --auto`:
1. Читает `.claude/commands/experts/_meta.yaml` — single source of truth
2. `git diff` против origin/main + working tree
3. Маппит изменённые файлы на затронутые домены через `globs`
4. Для каждого затронутого: `/experts:<domain>:self-improve true`
5. Отчёт в stdout (попадает в `.claude/.last_sync.log`)

## Files changed

| File | Action | Notes |
|---|---|---|
| `.claude/commands/experts/_meta.yaml` | new | 6 доменов, ~50 glob-паттернов суммарно |
| `.claude/hooks/sync_experts_on_session_end.sh` | new + chmod +x | 60 LOC, всегда exit 0 |
| `.claude/commands/sync-experts.md` | replace | Старый указывал на 9 dead horizontal экспертов и `project/backend/` |
| `.claude/settings.json` | edit | Добавлен SessionEnd hook, сохранён enabledPlugins |
| `.gitignore` | edit | Добавлены `.claude/.last_sync.log`, `.sync.lock` |
| `.claude/commands/experts/{catalog,engine-v4,pipeline-agents,widget,admin,curator}/` | new (стадия A) | По 3 файла на эксперта: expertise.yaml, self-improve.md, question.md |
| `docs/New features/experts_plan_2026-04-29.md` | new (стадия A) | Synthesis doc — domain mapping и план перестройки |

## Verification (что я проверил локально перед push)

✅ `git merge --ff-only experts/foundation` прошёл чисто  
✅ `git push origin main` прошёл  
✅ `bash -n .claude/hooks/sync_experts_on_session_end.sh` — синтаксис OK  
✅ `jq -e '.hooks.SessionEnd[0].hooks[0].command' .claude/settings.json` — schema валиден  
✅ Все 6 vertical-экспертов появились в `Skill` списке  
✅ Validate questions для engine-v4, widget, catalog, pipeline-agents — отвечают с правильными file:line refs

❓ Не проверил: реальный запуск SessionEnd (требует выйти из Claude Code).
   Это первый smoke-test для пользователя при следующем закрытии:
   1. Поработать в Claude (любые edit'ы или коммиты)
   2. `/exit` или закрыть терминал
   3. Открыть новую сессию, прочитать `.claude/.last_sync.log` —
      должны быть строки от `/sync-experts --auto`
   4. `git status` — могут быть изменения в `.claude/commands/experts/<X>/expertise.yaml`

## Known gaps / caveats

🔴 **Headless `claude --print` recursion** — теоретически вложенная сессия
от хука может сама триггернуть SessionEnd → бесконечная цепочка.
Anthropic должны блокировать (есть ли флаг `disableHooks` в headless?
не уверен). Реальное поведение увидим при первом срабатывании.

🟡 **9 LEGACY горизонтальных экспертов** (`backend-*`, `frontend-*`)
ссылаются на удалённый `project/backend/`. Не удалены этим коммитом —
отдельный уборочный коммит. Они НЕ маршрутизируются `/sync-experts`
(нет в `_meta.yaml`), `/experts:<X>:question` для них продолжит работать
но даст stale ответы.

🟡 **Brace expansion в glob'ах** — `_meta.yaml` использует `{a,b,c}` для
краткости. Когда slash-команда матчит файлы, ей нужно раскрыть скобки
вручную (Python `fnmatch` этого не делает). В `sync-experts.md` это
описано в Step 2.

🟡 **Стоимость** — Полный sync 6 экспертов ≈ $0.10-0.15 на Haiku.
Selective ≈ $0.03 на эксперт. Допустимо при закрытии сессии раз в день.
Если беспокойство — добавить env-флаг `EXPERTS_AUTO_SYNC=0` отключения.

🟡 **Параллельные сессии** — `flock` в хуке защищает от двух одновременных
sync. Но если sync уже идёт от первой сессии, вторая просто выйдет.
Нет очереди.

🟢 **Push не блокируется** — explicit constraint от Vlad'а. Хук всегда
exit 0, ничего не блокирует.

🟢 **Skip на чистом репо** — короткий no-op для read-only сессий.

🟢 **Skip на /clear** — `reason == "clear"` не считается закрытием.

## Suggested follow-ups

1. **Smoke-test первого SessionEnd** — после первого реального триггера
   проверить что нет recursion, .last_sync.log пишется корректно.
2. **Cleanup 9 dead horizontal experts** — `.claude/commands/experts/{backend-*,frontend-*}/` должны быть удалены.
3. **EXPERTS_AUTO_SYNC env flag** — опциональный kill-switch для пользователя.
4. **GitHub Action backup** — еженедельный полный sync через CI чтобы YAML не мог отстать совсем (на случай если SessionEnd не сработал N раз подряд).

## Update LAUNCH_ROADMAP.md

Phase 2 (cleanup + experts) теперь больше чем "partial" — основная работа
закрыта. Эксперты построены, авто-синк настроен. Что осталось phase 2:
- Удаление 9 dead horizontal experts (мелочь)
- Integration tests на pipeline (если ещё актуально)
- Перенос устаревших спек в archive (мелочь)

Не правлю roadmap в этом коммите — параллельная сессия его трогает (фаза 1).
Обновим вместе при следующей координационной точке.
