# Keepstar Backend

Go API server with hexagonal architecture.

## Stack

- Go 1.21+
- HTTP server (net/http)
- PostgreSQL (Neon)
- Anthropic Claude Haiku

## Architecture

```
cmd/server/main.go     # Entry point
internal/
├── domain/            # Entities, types
├── ports/             # Interfaces
├── adapters/          # Implementations
├── usecases/          # Business logic
├── handlers/          # HTTP layer
├── prompts/           # LLM prompts
├── config/            # Configuration
└── logger/            # Logging
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/chat` | POST | Send message, get AI response |
| `/api/v1/session/{id}` | GET | Get session with messages |
| `/api/v1/tenants/{slug}/products` | GET | List products for tenant |
| `/api/v1/tenants/{slug}/products/{id}` | GET | Get product details |
| `/health` | GET | Health check |
| `/ready` | GET | Readiness check |

## Setup

```bash
# Install dependencies
go mod download

# Create .env file in project root
ANTHROPIC_API_KEY=your_key
DATABASE_URL=postgresql://...
PORT=8080

# Run
go run ./cmd/server/

# Or build and run
go build -o server ./cmd/server/ && ./server
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8080 | Server port |
| `ANTHROPIC_API_KEY` | - | Anthropic API key (required) |
| `DATABASE_URL` | - | PostgreSQL connection string |
| `LLM_MODEL` | claude-haiku-4-5-20251001 | LLM model |
| `LOG_LEVEL` | info | Log level |
| `ENVIRONMENT` | development | Environment |

## Ports

| Port | Interface | Adapter |
|------|-----------|---------|
| LLMPort | Chat(ctx, message) | anthropic |
| CachePort | Session/message persistence | postgres |
| EventPort | Analytics tracking | postgres |
| CatalogPort | Product catalog | postgres |
| StatePort | Session state for agents | postgres |
