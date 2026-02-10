# Feature: Session Widget + Multi-Domain Deploy

**Дата:** 2026-02-10 19:30
**Статус:** Todo
**Приоритет:** High
**Зависит от:** done-session-init

## Контекст

Session init реализован — при открытии чата создаётся сессия с tenant seed. Следующие шаги: виджет для встраивания на сайты клиентов и мультидоменный деплой.

## План

### 1. Embeddable Chat Widget
- `<script src="https://keepstar.ai/widget.js" data-tenant="nike"></script>`
- Iframe-based widget с postMessage API
- Автоматический `X-Tenant-Slug` header из `data-tenant`
- Кастомизация: позиция, цвет, greeting text (из tenant settings)
- Минимальный бандл: ~15KB gzipped

### 2. Tenant-Aware Frontend
- `X-Tenant-Slug` header во всех API вызовах (pipeline, navigation, etc.)
- Tenant resolution: `data-tenant` attr → header → default from env
- Tenant branding: лого, цвета, greeting из tenant settings

### 3. Multi-Domain Deploy
- Nginx reverse proxy: `nike.keepstar.ai` → tenant slug "nike"
- Wildcard SSL: `*.keepstar.ai`
- Альтернатива: single domain + tenant из query param / widget attr

### 4. Tenant Settings Extension
- `greeting_text` — кастомный текст приветствия (вместо hardcoded)
- `widget_theme` — цветовая схема виджета
- `widget_position` — bottom-right / bottom-left
- Управление через admin panel

## Acceptance Criteria
- [ ] Widget встраивается одной строкой `<script>` на любой сайт
- [ ] Каждый тенант видит свои товары без конфигурации
- [ ] Greeting кастомизируется через admin panel
