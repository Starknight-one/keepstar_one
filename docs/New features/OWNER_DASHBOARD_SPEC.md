# Owner Dashboard — Spec

Дашборд владельца Keepstar. Единое место для мониторинга бизнеса: клиенты, оплаты, здоровье сервиса.

---

## Зачем

Сейчас нет единого экрана где видно:
- Кто платит, кто нет, у кого скоро заканчивается
- Как чувствует себя сервис (ошибки, latency, uptime)
- Насколько активны клиенты (сколько чатов, какая нагрузка)

Дашборд решает это — один экран, открыл утром и понял что происходит.

---

## Секции

### 1. Revenue & Billing

Что оплачено, что нет, MRR.

| Метрика | Источник |
|---------|----------|
| MRR (Monthly Recurring Revenue) | subscriptions |
| Активные подписки / всего | subscriptions |
| Просроченные оплаты | payments (status = overdue) |
| Upcoming renewals (7 дней) | subscriptions (expires_at) |
| Revenue за месяц (график) | payments (created_at, amount) |

**Карточки наверху**: MRR, Active / Total tenants, Overdue count (красная если > 0)

**Таблица**: список клиентов с колонками — tenant, plan, status (active/trial/overdue/churned), paid_until, monthly_amount

**Действия**: фильтр по статусу, сортировка по дате оплаты

---

### 2. Clients Overview

Активность клиентов — кто пользуется, кто нет.

| Метрика | Источник |
|---------|----------|
| Sessions за 24h / 7d / 30d | chat_sessions |
| Messages за период | chat_messages |
| Avg sessions per tenant | chat_sessions group by tenant |
| Inactive tenants (0 sessions за 7d) | chat_sessions |
| Top tenants by usage | chat_sessions count |

**Карточки**: Total sessions (24h), Active tenants (7d), Inactive tenants (7d)

**График**: sessions per day (line chart, 30 дней)

**Таблица**: tenant, sessions (24h/7d/30d), last_active, product_count, plan

---

### 3. Service Health

Здоровье пайплайна и инфраструктуры.

| Метрика | Источник |
|---------|----------|
| Pipeline success rate | traces (status = ok / total) |
| Avg response time (p50, p95) | traces (duration) |
| Error rate (%) | traces (status = error) |
| LLM cost за период | traces (tokens → cost) |
| DB connection pool | pg_stat_activity |
| Uptime | health endpoint pings |

**Карточки**: Success rate %, P95 latency, Error count (24h), LLM spend (today/month)

**График**: latency p50/p95 за 7 дней (line), errors per hour (bar)

**Alerts** (визуальные, на дашборде):
- Error rate > 5% → красный banner
- P95 > 10s → желтый banner
- Overdue payments → красный badge на Revenue секции

---

### 4. Quick Actions

Быстрые действия без ухода в админку.

- **Pause tenant** — остановить виджет для клиента (не удалить)
- **Send reminder** — напомнить об оплате (email/telegram)
- **View traces** — перейти в /debug/traces для конкретного tenant
- **Kill sessions** — убить зависшие сессии

---

## Данные

### Новые таблицы

```sql
-- Подписки клиентов
CREATE TABLE admin.subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES admin.tenants(id),
    plan TEXT NOT NULL,              -- 'trial' | 'starter' | 'pro' | 'enterprise'
    status TEXT NOT NULL DEFAULT 'trial', -- 'active' | 'trial' | 'overdue' | 'churned'
    monthly_amount_cents INT,
    trial_ends_at TIMESTAMPTZ,
    paid_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- Платежи
CREATE TABLE admin.payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES admin.tenants(id),
    subscription_id UUID REFERENCES admin.subscriptions(id),
    amount_cents INT NOT NULL,
    status TEXT NOT NULL,            -- 'paid' | 'pending' | 'overdue' | 'refunded'
    paid_at TIMESTAMPTZ,
    due_date TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now()
);
```

### Существующие данные (уже есть)

- `chat_sessions` — сессии по tenant
- `traces` — pipeline traces (duration, status, tokens)
- `admin.tenants` — список клиентов
- `catalog.products` — товары клиента

---

## API Endpoints

```
GET /api/v1/dashboard/summary        — карточки всех секций одним запросом
GET /api/v1/dashboard/revenue        — revenue детализация + таблица клиентов
GET /api/v1/dashboard/clients        — usage по клиентам
GET /api/v1/dashboard/health         — service health метрики
GET /api/v1/dashboard/health/history — графики latency/errors за период

Query params: ?period=24h|7d|30d
```

---

## UI

- **Где**: отдельный раздел в админке (`/dashboard`), первый экран после логина
- **Layout**: 4 summary-карточки наверху → 3 tab-секции (Revenue / Clients / Health)
- **Стиль**: минимальный, как в текущей админке. Графики — lightweight (recharts или подобное)
- **Auth**: только owner role (не tenant admin)
- **Responsive**: desktop-first, но читаемо на планшете

---

## Порядок реализации

1. **DB**: миграции subscriptions + payments, seed тестовыми данными
2. **Backend**: dashboard handler + SQL-запросы агрегации
3. **Frontend**: summary cards + таблица клиентов
4. **Graphs**: подключить recharts, графики sessions/latency/revenue
5. **Alerts**: визуальные баннеры по порогам
6. **Actions**: pause/reminder/kill (по необходимости)

---

## Не в скоупе (пока)

- Stripe/платёжная интеграция (пока ручной ввод оплат)
- Email/Telegram нотификации (пока только визуально на дашборде)
- Multi-owner access (пока один владелец)
- Billing автоматика (invoice generation, dunning)
