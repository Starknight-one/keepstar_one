# Keepstar Architecture

Embeddable AI-powered chat widget with dynamic widget composition.

## Product Vision

Чат-виджет который встраивается на любой сайт (как Intercom), но с AI-генерируемыми интерактивными виджетами. Пользователь общается с ботом, бот отвечает не просто текстом, а композицией виджетов (карточки товаров, рейтинги, кнопки, галереи).

**Ключевые отличия от Intercom:**
- Виджеты генерируются AI на лету
- Fullscreen overlay с canvas-like взаимодействием
- Actionable: кнопки, листание, корзина, лайки
- Кастомизируемый layout ("расположи кругом")

---

## Core Principles

### 1. Backend-First

```
Frontend = "тупая рендерилка"
Backend = вся логика, LLM, layout engine, state
```

Frontend получает готовый JSON и рендерит. Никакой бизнес-логики на клиенте.

### 2. Atomic Composition

```
Atoms → Widgets → Formations
```

- **Atoms**: базовые элементы (Text, Number, Image, Button, Rating...)
- **Widgets**: композиция атомов (карточка товара = Image + Text + Rating + Button)
- **Formations**: расположение виджетов на экране (grid, circle, stack...)

### 3. Lazy Rendering

```
LLM генерирует 15000 атомов
На экране рендерится 20-30 виджетов
Остальное подгружается при скролле/zoom
```

---

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  Client Site (любой сайт)                                       │
│                                                                 │
│  <script src="https://keepstar.io/chat.js"                      │
│          data-tenant="abc123" async>                            │
│  </script>                                                      │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  <iframe src="keepstar.io/widget?tenant=abc123">          │  │
│  │                                                           │  │
│  │    ┌─────────────────────────────────────────────────┐    │  │
│  │    │  Fullscreen Overlay                             │    │  │
│  │    │  ┌─────────────────────────────────────────┐    │    │  │
│  │    │  │  Widget Canvas (zoom/pan/scroll)        │    │    │  │
│  │    │  │  ┌───────┐ ┌───────┐ ┌───────┐         │    │    │  │
│  │    │  │  │Widget │ │Widget │ │Widget │ ...     │    │    │  │
│  │    │  │  └───────┘ └───────┘ └───────┘         │    │    │  │
│  │    │  └─────────────────────────────────────────┘    │    │  │
│  │    │  ┌─────────────────────────────────────────┐    │    │  │
│  │    │  │  Chat Panel                             │    │    │  │
│  │    │  └─────────────────────────────────────────┘    │    │  │
│  │    └─────────────────────────────────────────────────┘    │  │
│  │                                                           │  │
│  └───────────────────────────────────────────────────────────┘  │
│                              │                                  │
└──────────────────────────────┼──────────────────────────────────┘
                               │ HTTPS API
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│  Keepstar API (Go)                                              │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  /api/config/:tenantId                                  │    │
│  │  → theme, widget settings, prompts                      │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  /api/chat                                              │    │
│  │  → LLM Pipeline → Widget JSON + Layout                  │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  /api/widgets/viewport                                  │    │
│  │  → Lazy load: виджеты для текущего viewport             │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  /api/action                                            │    │
│  │  → Widget actions (add to cart, like, etc.)             │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                 │
│  ┌──────────────────────┐  ┌──────────────────────┐             │
│  │  PostgreSQL          │  │  Redis (optional)    │             │
│  │  - tenants           │  │  - sessions          │             │
│  │  - widget templates  │  │  - rate limiting     │             │
│  │  - user data         │  │  - cache             │             │
│  └──────────────────────┘  └──────────────────────┘             │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  LLM (Anthropic Claude Haiku)                           │    │
│  │  Stage 1: Query Analysis → Data Retrieval               │    │
│  │  Stage 2: Widget Composition → Layout Generation        │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Technology Stack

### Backend (Go)

| Component | Technology | Notes |
|-----------|------------|-------|
| Language | **Go 1.22+** | Fast, cheap hosting, single binary |
| Framework | **Chi** or **Fiber** | Lightweight HTTP router |
| Database | **PostgreSQL** | Tenants, configs, user data |
| Cache | **In-memory** → **Redis** | Sessions, rate limiting (Redis later) |
| LLM | **Anthropic SDK** | Claude Haiku (~$0.007/request) |

### Frontend (React in iframe)

| Component | Technology | Notes |
|-----------|------------|-------|
| Framework | **React 18** | Ecosystem, ecosystem, ecosystem |
| UI Components | **shadcn/ui** | Copy-paste, full control |
| Styling | **Tailwind CSS** | CSS variables for theming |
| Animations | **Framer Motion** | Layout transitions |
| Zoom/Pan | **react-zoom-pan-pinch** | Touch support included |
| Gestures | **@use-gesture/react** | Drag, pinch, hover |
| Build | **Vite** | Fast builds |

### Infrastructure

| Component | Technology | Notes |
|-----------|------------|-------|
| Hosting | **Hetzner** | €5-20/month |
| CDN/SSL | **Cloudflare** (free) | Client domains routing |
| Domains | **Wildcard SSL** | *.keepstar.io |

---

## API Design

### GET /api/config/:tenantId

Returns tenant configuration for widget initialization.

```json
{
  "tenantId": "abc123",
  "theme": {
    "colors": {
      "primary": "#6366f1",
      "background": "#ffffff",
      "foreground": "#0f172a"
    },
    "fonts": {
      "body": "Inter"
    },
    "radius": "8px"
  },
  "chat": {
    "welcomeMessage": "Привет! Чем могу помочь?",
    "placeholder": "Напишите сообщение..."
  },
  "widgets": {
    "enabled": ["product", "rating", "gallery", "cta"]
  }
}
```

### POST /api/chat

Main chat endpoint. Returns text response + widgets + layout.

**Request:**
```json
{
  "tenantId": "abc123",
  "sessionId": "sess_xyz",
  "message": "Покажи все кроссовки Nike",
  "viewport": {
    "width": 1200,
    "height": 800
  }
}
```

**Response:**
```json
{
  "sessionId": "sess_xyz",
  "response": {
    "text": "Нашёл 12 моделей кроссовок Nike:",
    "totalWidgets": 12,
    "totalAtoms": 156,
    "layout": {
      "type": "grid",
      "columns": 3,
      "gap": 16
    },
    "viewport": {
      "widgets": [
        {
          "id": "w1",
          "type": "product",
          "position": { "x": 0, "y": 0 },
          "size": { "width": 280, "height": 320 },
          "atoms": [
            { "type": "image", "src": "...", "alt": "Nike Air Max" },
            { "type": "text", "variant": "h3", "content": "Nike Air Max 90" },
            { "type": "number", "value": 12990, "format": "currency" },
            { "type": "rating", "value": 4.5, "count": 128 },
            { "type": "button", "label": "В корзину", "action": "add_to_cart" }
          ]
        },
        // ... ещё 5-7 виджетов для текущего viewport
      ],
      "hasMore": true,
      "nextCursor": "cursor_abc"
    }
  },
  "cost": {
    "tokens": 4500,
    "usd": 0.007
  }
}
```

### GET /api/widgets/viewport

Lazy load widgets for scroll/zoom.

**Request:**
```
GET /api/widgets/viewport?sessionId=sess_xyz&cursor=cursor_abc&limit=10
```

**Response:**
```json
{
  "widgets": [...],
  "hasMore": true,
  "nextCursor": "cursor_def"
}
```

### POST /api/action

Widget actions (non-LLM, fast).

**Request:**
```json
{
  "tenantId": "abc123",
  "sessionId": "sess_xyz",
  "widgetId": "w1",
  "action": "add_to_cart",
  "payload": {
    "productId": "prod_123",
    "quantity": 1
  }
}
```

**Response:**
```json
{
  "success": true,
  "cart": {
    "itemCount": 3,
    "total": 25980
  }
}
```

---

## LLM Pipeline

Two-stage pipeline (from prototype):

```
User Query
    │
    ▼
┌─────────────────────────────────────┐
│  Stage 1: Query Analysis            │
│  - Understand intent                │
│  - Fetch relevant data (tools)      │
│  - ~$0.001-0.002                    │
└─────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────┐
│  Stage 2: Widget Composition        │
│  - Generate widget structure        │
│  - Determine layout                 │
│  - ~$0.004-0.005                    │
└─────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────┐
│  Layout Engine (non-LLM)            │
│  - Calculate positions              │
│  - Viewport slicing                 │
│  - Lazy load preparation            │
└─────────────────────────────────────┘
    │
    ▼
Response JSON
```

### Stage 2 Output Format

LLM returns semantic structure, backend calculates positions:

```json
{
  "layout": "grid",
  "layoutParams": {
    "columns": "auto",
    "priority": "equal"
  },
  "widgets": [
    {
      "id": "w1",
      "type": "product",
      "size": "medium",
      "data": { "productId": "prod_123" },
      "atoms": [
        { "type": "image", "field": "image" },
        { "type": "text", "variant": "h3", "field": "name" },
        { "type": "number", "field": "price", "format": "currency" },
        { "type": "rating", "field": "rating" },
        { "type": "button", "label": "В корзину", "action": "add_to_cart" }
      ]
    }
  ]
}
```

---

## Lazy Loading Strategy

### Problem
LLM может сгенерировать 100+ виджетов (15000 атомов), но на экране помещается 10-20.

### Solution

```
┌─────────────────────────────────────────────────────────────────┐
│  Backend State (per session)                                    │
│                                                                 │
│  session_xyz: {                                                 │
│    allWidgets: [w1, w2, w3, ... w100],  // полный результат     │
│    renderedIds: [w1, w2, w3, w4, w5],   // что уже отдали       │
│    cursor: 5                                                    │
│  }                                                              │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│  Frontend                                                       │
│                                                                 │
│  1. Initial load: получает первые N виджетов                    │
│  2. Scroll/zoom: запрашивает следующие через cursor             │
│  3. Intersection Observer: триггерит загрузку                   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### API Flow

```
1. POST /api/chat
   → returns first 10 widgets + hasMore: true + cursor

2. User scrolls down

3. GET /api/widgets/viewport?cursor=...
   → returns next 10 widgets + new cursor

4. Repeat until hasMore: false
```

---

## Widget System

### Atom Types (10 базовых)

| Atom | Description | Props |
|------|-------------|-------|
| `text` | Текст (h1-h6, body, caption) | variant, content |
| `number` | Числа с форматированием | value, format (currency, percent, compact) |
| `image` | Изображения | src, alt, lazy |
| `icon` | Иконки/эмодзи | name, color |
| `badge` | Статус-бейджи | label, variant (success, warning, danger, info) |
| `button` | Кнопки | label, action, variant |
| `input` | Поля ввода | type, placeholder, value |
| `rating` | Звёздные рейтинги | value, count |
| `divider` | Разделители | orientation |
| `progress` | Прогресс-бары | value, max |

### Widget Sizes

| Size | Dimensions | Max Atoms |
|------|------------|-----------|
| tiny | 80-110px | 2 |
| small | 160-220px | 3 |
| medium | 280-350px | 5 |
| large | 384-460px | 10 |

### Layout Types

| Layout | Description | Use Case |
|--------|-------------|----------|
| `grid` | Сетка с колонками | Список товаров |
| `stack` | Вертикальный список | Чат-история |
| `circle` | По окружности | Сравнение |
| `focus` | Один большой + мелкие вокруг | Детали товара |
| `scatter` | Свободное расположение | Креативные запросы |

---

## Frontend Architecture

### Component Hierarchy

```
<WidgetProvider>                      // State management
  <FullscreenOverlay>                 // Затемнение + ESC/click outside
    <Backdrop onClick={close} />

    <TransformWrapper>                // Zoom/pan container
      <TransformComponent>
        <WidgetCanvas>                // Виджеты здесь
          {widgets.map(w => (
            <Widget
              key={w.id}
              position={w.position}
              selected={selected === w.id}
              onSelect={select}
            >
              {w.atoms.map(a => (
                <Atom key={a.id} {...a} />
              ))}
            </Widget>
          ))}
          <LazyLoadTrigger onVisible={loadMore} />
        </WidgetCanvas>
      </TransformComponent>
    </TransformWrapper>

    <ChatPanel>                       // Fixed position, поверх canvas
      <ChatHistory messages={messages} />
      <ChatInput
        onSubmit={sendMessage}
        context={selectedWidget}      // Контекст выделенного виджета
      />
    </ChatPanel>

    <Controls>                        // ESC, close button, zoom controls
      <ZoomControls />
      <CloseButton />
    </Controls>

  </FullscreenOverlay>
</WidgetProvider>
```

### State Management

```typescript
interface WidgetState {
  // Session
  sessionId: string | null

  // Widgets
  widgets: Widget[]
  selectedWidgetId: string | null
  hasMore: boolean
  cursor: string | null
  loading: boolean

  // Viewport
  zoom: number
  pan: { x: number, y: number }

  // Chat
  messages: Message[]
  inputValue: string

  // UI
  isOpen: boolean
  theme: Theme
}
```

Для MVP достаточно React Context. Позже можно добавить Zustand если нужно.

### Theming (CSS Variables)

```css
/* Генерируется из tenant config */
:root {
  --color-primary: 99 102 241;
  --color-background: 255 255 255;
  --color-foreground: 15 23 42;
  --radius: 0.5rem;
  --font-sans: 'Inter', sans-serif;
}

/* Использование в компонентах */
.widget {
  background: rgb(var(--color-background));
  border-radius: var(--radius);
}

.button-primary {
  background: rgb(var(--color-primary));
}
```

---

## Embed Script

### Loader (chat.js, ~2KB)

```javascript
(function() {
  const tenant = document.currentScript.dataset.tenant;
  const iframe = document.createElement('iframe');

  iframe.src = `https://keepstar.io/widget?tenant=${tenant}`;
  iframe.style.cssText = `
    position: fixed;
    bottom: 20px;
    right: 20px;
    width: 60px;
    height: 60px;
    border: none;
    z-index: 999999;
  `;

  // Expand to fullscreen on open
  window.addEventListener('message', (e) => {
    if (e.origin !== 'https://keepstar.io') return;

    if (e.data.type === 'keepstar:open') {
      iframe.style.width = '100vw';
      iframe.style.height = '100vh';
      iframe.style.bottom = '0';
      iframe.style.right = '0';
    }

    if (e.data.type === 'keepstar:close') {
      iframe.style.width = '60px';
      iframe.style.height = '60px';
      iframe.style.bottom = '20px';
      iframe.style.right = '20px';
    }
  });

  document.body.appendChild(iframe);
})();
```

---

## Security Considerations

### Multi-tenancy Isolation

- Tenant ID validated on every request
- Session bound to tenant
- Data queries scoped to tenant

### Rate Limiting

- Per-tenant limits (requests/minute)
- Per-session limits
- LLM cost caps

### CORS

- Strict origin validation
- Credentials only for known domains

### Input Validation

- All user input sanitized before LLM
- Widget actions validated against allowed actions

---

## MVP Roadmap

### Phase 1: Foundation (Week 1-2)

- [ ] Go backend skeleton (Chi/Fiber)
- [ ] PostgreSQL schema (tenants, sessions)
- [ ] Basic /api/chat endpoint (mock response)
- [ ] React widget shell in iframe
- [ ] Embed script (chat.js)

### Phase 2: Core Features (Week 3-4)

- [ ] LLM integration (Stage 1 + Stage 2)
- [ ] Atom components (all 10 types)
- [ ] Widget rendering
- [ ] Fullscreen overlay with backdrop
- [ ] Chat input + history

### Phase 3: Polish (Week 5-6)

- [ ] Theming (CSS variables from config)
- [ ] Lazy loading (cursor pagination)
- [ ] Widget actions (add to cart, like)
- [ ] Mobile responsive
- [ ] Close on ESC / click outside

### Phase 4: Production (Week 7-8)

- [ ] Admin panel (tenant config)
- [ ] Custom domain support (Cloudflare)
- [ ] Monitoring + logging
- [ ] First customer deployment

---

## File Structure

```
Keepstar_one_ultra/
├── .claude/                    # Agent layer (ADW)
├── ADW/                        # SDLC orchestrator
│
├── api/                        # Go backend
│   ├── cmd/
│   │   └── server/
│   │       └── main.go         # Entry point
│   ├── internal/
│   │   ├── config/             # Configuration
│   │   ├── handler/            # HTTP handlers
│   │   │   ├── chat.go
│   │   │   ├── config.go
│   │   │   ├── widgets.go
│   │   │   └── action.go
│   │   ├── llm/                # LLM pipeline
│   │   │   ├── client.go       # Anthropic client
│   │   │   ├── stage1.go       # Query analysis
│   │   │   ├── stage2.go       # Widget composition
│   │   │   └── prompts/        # System prompts
│   │   ├── layout/             # Layout engine
│   │   │   ├── engine.go
│   │   │   ├── grid.go
│   │   │   ├── circle.go
│   │   │   └── focus.go
│   │   ├── widget/             # Widget building
│   │   │   ├── builder.go
│   │   │   └── atoms.go
│   │   ├── tenant/             # Multi-tenancy
│   │   │   ├── service.go
│   │   │   └── repository.go
│   │   ├── session/            # Session management
│   │   │   └── store.go
│   │   └── storage/            # Database
│   │       ├── postgres.go
│   │       └── migrations/
│   ├── pkg/                    # Shared utilities
│   │   └── response/
│   ├── go.mod
│   └── go.sum
│
├── widget/                     # React widget (iframe content)
│   ├── src/
│   │   ├── components/
│   │   │   ├── atoms/          # 10 atom components
│   │   │   │   ├── Text.tsx
│   │   │   │   ├── Number.tsx
│   │   │   │   ├── Image.tsx
│   │   │   │   ├── Button.tsx
│   │   │   │   ├── Rating.tsx
│   │   │   │   ├── Badge.tsx
│   │   │   │   ├── Icon.tsx
│   │   │   │   ├── Input.tsx
│   │   │   │   ├── Divider.tsx
│   │   │   │   ├── Progress.tsx
│   │   │   │   └── index.ts
│   │   │   ├── widget/
│   │   │   │   ├── Widget.tsx
│   │   │   │   ├── WidgetCanvas.tsx
│   │   │   │   └── LazyLoadTrigger.tsx
│   │   │   ├── chat/
│   │   │   │   ├── ChatPanel.tsx
│   │   │   │   ├── ChatInput.tsx
│   │   │   │   └── ChatHistory.tsx
│   │   │   ├── overlay/
│   │   │   │   ├── FullscreenOverlay.tsx
│   │   │   │   ├── Backdrop.tsx
│   │   │   │   └── Controls.tsx
│   │   │   └── ui/             # shadcn/ui components
│   │   ├── context/
│   │   │   └── WidgetContext.tsx
│   │   ├── hooks/
│   │   │   ├── useWidgets.ts
│   │   │   ├── useChat.ts
│   │   │   └── useTheme.ts
│   │   ├── lib/
│   │   │   ├── api.ts          # API client
│   │   │   └── utils.ts
│   │   ├── styles/
│   │   │   └── globals.css
│   │   ├── types/
│   │   │   └── index.ts
│   │   ├── App.tsx
│   │   └── main.tsx
│   ├── index.html
│   ├── vite.config.ts
│   ├── tailwind.config.js
│   ├── tsconfig.json
│   └── package.json
│
├── embed/                      # Embed script
│   ├── src/
│   │   └── chat.js
│   └── build.js                # Minification
│
├── admin/                      # Admin panel (later)
│   └── ...
│
├── deploy/
│   ├── Dockerfile
│   ├── docker-compose.yml
│   └── nginx.conf
│
├── docs/
│   ├── ARCHITECTURE.md         # This file
│   └── API.md                  # API documentation
│
└── README.md
```

---

## Cost Estimates

### LLM Costs (Claude Haiku)

| Metric | Value |
|--------|-------|
| Cost per request | ~$0.007 |
| 1000 requests/day | ~$7/day = $210/month |
| 10000 requests/day | ~$70/day = $2100/month |

### Infrastructure (Hetzner)

| Component | Cost |
|-----------|------|
| VPS (CX21) | €5-10/month |
| PostgreSQL (managed) | €10-15/month or self-hosted |
| Cloudflare | Free |

**Total MVP cost: ~€15-25/month + LLM usage**

---

## Open Questions

1. **Мобильное приложение?** WebView или нативный SDK?
2. **Оффлайн режим?** Кэширование виджетов?
3. **Аналитика?** Что трекать?
4. **Биллинг?** Per-request или подписка?
5. **Self-hosted вариант?** Для enterprise?
