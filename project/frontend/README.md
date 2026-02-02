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
│   └── canvas/        # Widget canvas (future)
├── entities/          # Business entities
│   ├── atom/          # Atomic UI elements
│   ├── widget/        # Widget compositions
│   └── message/       # Chat messages
└── shared/            # Shared utilities
    └── api/           # API client
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

## Features

### Chat (`features/chat/`)
- ChatPanel — main chat component
- ChatHistory — message list
- ChatInput — input with send button
- useChatMessages — state management
- useChatSubmit — API submission
- Session persistence via localStorage

### Catalog (`features/catalog/`)
- ProductGrid — product display grid
- useCatalogProducts — catalog state management

## Configuration

Backend API URL: `http://localhost:8080` (configured in apiClient.js)
