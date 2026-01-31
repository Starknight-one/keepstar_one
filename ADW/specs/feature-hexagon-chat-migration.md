# Feature: Hexagon Chat Migration

## Feature Description
Migrate chat to the new hexagonal architecture. Simple chat: user sends a message → receives a text response from Claude Haiku 4.5. No widgets, no two-agent pipeline — just a working chat through the new structure.

## Objective
- Fully working chat through hexagonal architecture
- Backend: `cmd/server/main.go` → handlers → usecases → adapters
- Frontend: `features/chat` instead of `components/Chat`
- Remove old code (`main.go`, `anthropic.go`, `gigachat.go`, `components/Chat`)
- Model: `claude-haiku-4-5-20251001`

## Expertise Context
Expertise used:
- **backend**: Hexagonal architecture, layer_rules, cmd/server/main.go already set up as bootstrap
- **frontend**: Feature-sliced structure, features/chat already has stubs (ChatPanel, useChatMessages, useChatSubmit)

## Relevant Files

### Existing Files (TO MODIFY)

**Backend:**
- `project/backend/internal/adapters/anthropic/anthropic_client.go` — implement Chat method
- `project/backend/internal/ports/llm_port.go` — simplify to Chat method
- `project/backend/internal/usecases/chat_analyze_query.go` — rename and use as simple ChatUseCase
- `project/backend/internal/handlers/handler_chat.go` — implement HandleChat
- `project/backend/internal/config/config.go` — update default model
- `project/backend/cmd/server/main.go` — simplify (remove unused usecases)

**Frontend:**
- `project/frontend/src/features/chat/ChatPanel.jsx` — already ready
- `project/frontend/src/features/chat/ChatInput.jsx` — implement
- `project/frontend/src/features/chat/ChatHistory.jsx` — implement
- `project/frontend/src/shared/api/apiClient.js` — already ready for /api/v1/chat
- `project/frontend/src/App.jsx` — switch to ChatPanel

### Files to DELETE
- `project/backend/main.go`
- `project/backend/anthropic.go`
- `project/backend/gigachat.go`
- `project/frontend/src/components/Chat.jsx`
- `project/frontend/src/components/Chat.css`

### Files to CREATE (if needed)
- `project/frontend/src/features/chat/ChatPanel.css` — chat styles

## Step by Step Tasks
IMPORTANT: Execute strictly in order.

### 1. Simplify LLM Port
**File:** `project/backend/internal/ports/llm_port.go`
- Remove AnalyzeQuery and ComposeWidgets (not needed yet)
- Add simple method:
```go
type LLMPort interface {
    Chat(ctx context.Context, message string) (string, error)
}
```
- Remove complex Request/Response structs

### 2. Implement Anthropic Adapter
**File:** `project/backend/internal/adapters/anthropic/anthropic_client.go`
- Copy logic from old `anthropic.go`
- Implement `Chat(ctx, message) (string, error)`
- Use model from config
- Anthropic API: POST https://api.anthropic.com/v1/messages

### 3. Update Config
**File:** `project/backend/internal/config/config.go`
- Change default model to `claude-haiku-4-5-20251001`

### 4. Create Simple Chat UseCase
**File:** `project/backend/internal/usecases/chat_send_message.go` (new file, rename chat_analyze_query.go)
- Simple usecase:
```go
type SendMessageUseCase struct {
    llm ports.LLMPort
}

func (uc *SendMessageUseCase) Execute(ctx context.Context, message string) (string, error) {
    return uc.llm.Chat(ctx, message)
}
```

### 5. Implement Chat Handler
**File:** `project/backend/internal/handlers/handler_chat.go`
- Parse JSON body: `{ "message": "..." }`
- Call usecase
- Return: `{ "response": "..." }`
- Error handling

### 6. Simplify cmd/server/main.go
**File:** `project/backend/cmd/server/main.go`
- Remove unused usecases (analyzeQuery, composeWidgets, executeSearch)
- Remove productStore and cache
- Keep only:
  - config.Load()
  - anthropic.NewClient()
  - usecases.NewSendMessageUseCase()
  - handlers.NewChatHandler()

### 7. Implement Frontend ChatInput
**File:** `project/frontend/src/features/chat/ChatInput.jsx`
- Input + Send button
- Props: onSubmit, disabled
- Copy styles from old Chat.css

### 8. Implement Frontend ChatHistory
**File:** `project/frontend/src/features/chat/ChatHistory.jsx`
- Render message list
- Props: messages, isLoading
- Show "Thinking..." when isLoading

### 9. Add Styles
**File:** `project/frontend/src/features/chat/ChatPanel.css`
- Copy styles from `components/Chat.css`
- Import in ChatPanel.jsx

### 10. Update App.jsx
**File:** `project/frontend/src/App.jsx`
- Replace `import Chat from './components/Chat'` with `import { ChatPanel } from './features/chat/ChatPanel'`
- Use ChatPanel instead of Chat

### 11. Delete Old Code
- `rm project/backend/main.go`
- `rm project/backend/anthropic.go`
- `rm project/backend/gigachat.go`
- `rm project/frontend/src/components/Chat.jsx`
- `rm project/frontend/src/components/Chat.css`
- `rmdir project/frontend/src/components` (if empty)

### 12. Update Run Commands
**File:** `project/backend/README.md` (if instructions exist)
- New run command: `go run ./cmd/server/`

### 13. Validation
- Backend: `cd project/backend && go build ./...`
- Backend: `cd project/backend && go run ./cmd/server/` — verify it starts
- Frontend: `cd project/frontend && npm run build`
- E2E: Open http://localhost:5173, send a message, receive a response

## Validation Commands
```bash
# Backend build
cd project/backend && go build ./...

# Backend tests
cd project/backend && go test ./...

# Frontend build
cd project/frontend && npm run build

# Frontend lint
cd project/frontend && npm run lint
```

## Acceptance Criteria
- [ ] Backend starts via `go run ./cmd/server/`
- [ ] POST /api/v1/chat returns response from Claude
- [ ] Frontend uses features/chat/ChatPanel
- [ ] Old files (main.go, anthropic.go, gigachat.go, components/Chat) deleted
- [ ] Chat works: input message → response displayed
- [ ] No errors on `go build ./...`
- [ ] No errors on `npm run build`

## Notes
- **Simplification:** Not implementing two-agent pipeline, widgets, sessions — those are next features. Goal is minimal working chat through new architecture.
- **API endpoint:** New `/api/v1/chat`, old `/api/chat` will stop working after deleting main.go
- **CORS:** Already configured in handlers/middleware_cors.go
