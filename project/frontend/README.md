# Keepstar Frontend

React chat widget with Feature-Sliced Design architecture.

## Stack

- React 19
- Vite 7
- Feature-Sliced Design

## Architecture

```
src/
├── app/               # App entry, providers
├── features/          # Business features
│   ├── chat/          # Chat panel
│   ├── catalog/       # Product catalog
│   ├── overlay/       # Fullscreen overlay
│   ├── navigation/    # Back button
│   └── canvas/        # Widget canvas (future)
├── entities/          # Business entities
│   ├── atom/          # Atomic UI elements
│   ├── widget/        # Widget compositions
│   ├── formation/     # Formation layouts
│   └── message/       # Chat messages
└── shared/            # Shared utilities
    ├── api/           # API client
    └── theme/         # Theme system (CSS variables, provider, switcher)
```

## Setup

```bash
# Install dependencies
npm install

# Run dev server
npm run dev

# Build for production
npm run build

# Lint
npm run lint
```

## Scripts

| Command | Description |
|---------|-------------|
| `npm run dev` | Start dev server (http://localhost:5173) |
| `npm run build` | Production build |
| `npm run preview` | Preview production build |
| `npm run lint` | Run ESLint |

## API Client

Located at `src/shared/api/apiClient.js`:

| Function | Endpoint | Description |
|----------|----------|-------------|
| `sendChatMessage(message, sessionId)` | POST /api/v1/chat | Send chat message |
| `getSession(sessionId)` | GET /api/v1/session/{id} | Get session history |
| `getProducts(tenantSlug, filters)` | GET /api/v1/tenants/{slug}/products | List products |
| `getProduct(tenantSlug, productId)` | GET /api/v1/tenants/{slug}/products/{id} | Get product |
| `sendPipelineQuery(sessionId, query)` | POST /api/v1/pipeline | Two-agent pipeline |
| `expandView(sessionId, entityType, entityId)` | POST /api/v1/navigation/expand | Drill-down to detail |
| `goBack(sessionId)` | POST /api/v1/navigation/back | Navigate back |

## Features

### Chat (`features/chat/`)
- ChatPanel — main chat component
- ChatHistory — message list
- ChatInput — input with send button
- useChatMessages — state management
- useChatSubmit — API submission
- sessionCache — localStorage session cache (save/load/clear, 30min TTL)
- Session persistence via localStorage with instant cache restore

### Catalog (`features/catalog/`)
- ProductGrid — product display grid
- useCatalogProducts — catalog state management

### Overlay (`features/overlay/`)
- FullscreenOverlay — backdrop + content
- useOverlayState — { isOpen, open, close, toggle }
- Animations: backdrop-fade-in, chat-slide-in, widget-fade-in

### Navigation (`features/navigation/`)
- BackButton — back navigation for drill-down views

## Configuration

Backend API URL: `VITE_API_URL` env variable, fallback `http://localhost:8080/api/v1` (configured in apiClient.js)
