# Keepstar Backend

Go API server for the Keepstar chat widget.

## Stack

- Go 1.21+
- HTTP server (net/http)
- CORS (github.com/rs/cors)
- Environment config (godotenv)

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/chat` | POST | Send message, get AI response |
| `/health` | GET | Health check |

## AI Providers

Configurable via `AI_PROVIDER` env variable:
- `anthropic` (default) - Claude API
- `gigachat` - GigaChat API

## Setup

```bash
# Install dependencies
go mod download

# Create .env file in project root (../
ANTHROPIC_API_KEY=your_key
AI_PROVIDER=anthropic
BACKEND_PORT=8080

# Run
go run .

# Or build and run
go build -o server && ./server
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BACKEND_PORT` | 8080 | Server port |
| `AI_PROVIDER` | anthropic | AI provider (anthropic/gigachat) |
| `ANTHROPIC_API_KEY` | - | Anthropic API key |
| `GIGACHAT_CLIENT_ID` | - | GigaChat client ID |
| `GIGACHAT_CLIENT_SECRET` | - | GigaChat client secret |

## Project Structure

```
backend/
├── main.go          # Entry point, HTTP handlers
├── anthropic.go     # Anthropic Claude integration
├── gigachat.go      # GigaChat integration
├── go.mod
└── go.sum
```
