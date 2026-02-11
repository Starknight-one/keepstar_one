# Done: Embeddable Chat Widget (Shadow DOM)

**Ветка:** `feat/embeddable-widget`
**Версия:** Alpha 0.0.1
**Дата:** 2026-02-11

---

## Что сделано

Фронтенд превращён из React SPA в встраиваемый виджет. Один `widget.js` файл (IIFE бандл, 72KB gzip), Shadow DOM, полная изоляция стилей от сайта клиента.

### Использование клиентом

```html
<script src="https://keepstar.one/widget.js" data-tenant="nike"></script>
```

Опционально:
```html
<script src="https://keepstar.one/widget.js" data-tenant="nike" data-api="https://keepstar.one/api/v1"></script>
```

---

## Новые файлы

| Файл | Назначение |
|------|-----------|
| `src/widget.jsx` | Entry point: Shadow DOM shell, Google Fonts, CSS injection, React mount. Читает `data-tenant`/`data-api` из script tag (prod) или `window.__KEEPSTAR_WIDGET__` (dev) |
| `src/WidgetApp.jsx` | Widget UI: trigger button + fullscreen overlay + chat panel + formation area. Вызывает `setTenantSlug()`/`setApiBaseUrl()` при mount |
| `src/shared/config/WidgetConfigContext.jsx` | React Context: `{ tenantSlug, apiBaseUrl }`. Provider + `useWidgetConfig()` hook |
| `src/widget.css` | Shadow DOM scoped styles: `:host` reset, `.chat-toggle-btn`. Заменяет глобальные `:root`/`body` из бывших `index.css`/`App.css` |

## Изменённые файлы

| Файл | Изменение |
|------|-----------|
| `src/shared/api/apiClient.js` | `API_BASE_URL` const → `_apiBaseUrl` let. Добавлены `setTenantSlug()`, `setApiBaseUrl()`, `getHeaders()`. Все fetch-вызовы используют `getHeaders()` с `X-Tenant-Slug` header |
| `vite.config.js` | lib mode (IIFE entry `src/widget.jsx`), `process.env.NODE_ENV` define, `shadowDomCss()` plugin — глушит обычные CSS imports компонентов (CSS идёт через `?inline`) |
| `index.html` | Тестовая страница клиента вместо SPA shell. `window.__KEEPSTAR_WIDGET__` для dev config |
| `package.json` | Убран `build:widget`, `build` теперь собирает виджет. Убран `vite-plugin-css-injected-by-js` |
| `project/Dockerfile` | Один `RUN npm run build` вместо двух. `widget.js` попадает в `./static/` через `COPY dist ./static` |
| `scripts/start.sh` | Увеличен sleep до 12s, обновлены комментарии |
| `.claude/commands/start.md` | "Widget (test page)" вместо "Frontend" |
| `.claude/commands/start_all.md` | Секция "Chat Widget" |
| `.claude/commands/stop.md` | "Widget dev server" |

## Удалённые файлы

| Файл | Причина |
|------|---------|
| `src/App.jsx` | SPA компонент с lorem ipsum — заменён на `WidgetApp.jsx` |
| `src/App.css` | Глобальные стили SPA — `.chat-toggle-btn` перенесён в `widget.css` |
| `src/main.jsx` | SPA entry point (`createRoot(#root)`) — заменён на `widget.jsx` |
| `src/index.css` | Глобальные `:root`/`body`/`button` ресеты — не нужны в Shadow DOM |
| `vite.widget.config.js` | Отдельный конфиг виджета — слит в основной `vite.config.js` |

---

## Архитектурные решения

### Shadow DOM CSS Strategy

Проблема: компоненты импортируют CSS обычным `import './Component.css'`. В Shadow DOM стили из `document.head` не работают.

Решение: Vite plugin `shadowDomCss()` глушит обычные CSS imports (подменяет на пустой модуль). Все CSS импортируются в `widget.jsx` через `?inline` (как строки) и инжектятся в shadow root через `<style>` тег. Одинаково работает в dev и prod.

### X-Tenant-Slug Header

`apiClient.js` — модуль-уровневые `_tenantSlug`/`_apiBaseUrl` + setter-функции. `getHeaders()` добавляет `X-Tenant-Slug` только если slug задан. Не ломает standalone режим (без slug header не шлётся, бэкенд фолбечится на дефолт).

### Dev/Prod Parity

Dev (`npm run dev`): Vite dev server → `index.html` → `window.__KEEPSTAR_WIDGET__` → Shadow DOM + CSS injection.
Prod (`npm run build`): IIFE бандл → `document.currentScript` → `data-tenant`/`data-api` → Shadow DOM + CSS injection.
Идентичное поведение.

---

## Метрики

- **Bundle size:** 251KB raw / 72.6KB gzip
- **CSS:** ~33KB (все 13 CSS файлов инлайнены)
- **React + ReactDOM:** ~68KB gzip (bundled, production mode)
- **Формат:** IIFE, один файл, zero dependencies
