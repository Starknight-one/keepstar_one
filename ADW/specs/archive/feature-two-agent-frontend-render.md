# Feature: Two-Agent Pipeline - Frontend Rendering (Phase 4)

## Feature Description

Implement frontend rendering for Formation JSON from the Two-Agent Pipeline. This creates the visual layer where:
- FormationRenderer displays widgets in grid/carousel/single layouts
- AtomRenderer handles all atom types with proper styling
- WidgetRenderer respects size constraints (tiny/small/medium/large)
- Chat integrates with Pipeline API to show product formations

This is Phase 4 from SPEC_TWO_AGENT_PIPELINE.md - the UI visualization step.

## Objective

Enable the flow: Pipeline API → Formation JSON → FormationRenderer → Visual widgets in chat

**Verification**: After "покажи ноутбуки" in chat, see product cards displayed in a grid layout.

## Expertise Context

Expertise used:
- **frontend**: Feature-Sliced Design, existing entities (atom, widget, message), React patterns
- **backend**: FormationWithData structure, Pipeline response format

Key insights from expertise:
- AtomRenderer already exists with basic types (TEXT, PRICE, IMAGE, RATING, BUTTON, BADGE)
- WidgetRenderer exists with ProductCard, TextBlock, QuickReplies
- MessageBubble already supports `message.widgets` and `message.formation`
- ProductGrid shows pattern for converting data to widgets
- Need to add FormationRenderer for layout modes (grid, carousel, single)
- CSS needs size classes for widget constraints
- useChatSubmit.js currently uses sendChatMessage - needs to switch to pipeline

## Relevant Files

### Existing Files (to modify)
- `project/frontend/src/entities/atom/AtomRenderer.jsx` - Add NUMBER, ICON, DIVIDER, PROGRESS + styles
- `project/frontend/src/entities/atom/atomModel.js` - Already complete
- `project/frontend/src/entities/widget/WidgetRenderer.jsx` - Add size prop handling
- `project/frontend/src/entities/widget/widgetModel.js` - Add WidgetSize enum
- `project/frontend/src/entities/message/MessageBubble.jsx` - Use FormationRenderer (keep backward compat)
- `project/frontend/src/shared/api/apiClient.js` - Add pipeline API function
- `project/frontend/src/features/chat/useChatSubmit.js` - Switch to pipeline API
- `project/backend/internal/handlers/routes.go` - Add pipeline route
- `project/backend/cmd/server/main.go` - Wire pipeline handler

### New Files
- `project/frontend/src/entities/formation/FormationRenderer.jsx` - Grid/carousel/single layouts
- `project/frontend/src/entities/formation/formationModel.js` - Formation types
- `project/frontend/src/entities/formation/Formation.css` - Layout styles
- `project/frontend/src/entities/atom/Atom.css` - Atom type styles
- `project/frontend/src/entities/widget/Widget.css` - Widget size styles
- `project/backend/internal/handlers/handler_pipeline.go` - Pipeline HTTP handler

## Step by Step Tasks

IMPORTANT: Execute strictly in order.

### 1. Create Pipeline Handler (Backend)

File: `project/backend/internal/handlers/handler_pipeline.go`

```go
package handlers

import (
	"encoding/json"
	"net/http"

	"keepstar/internal/usecases"
)

// PipelineHandler handles pipeline requests
type PipelineHandler struct {
	pipelineUC *usecases.PipelineExecuteUseCase
}

// NewPipelineHandler creates a pipeline handler
func NewPipelineHandler(pipelineUC *usecases.PipelineExecuteUseCase) *PipelineHandler {
	return &PipelineHandler{pipelineUC: pipelineUC}
}

// PipelineRequest is the request body
type PipelineRequest struct {
	SessionID string `json:"sessionId"`
	Query     string `json:"query"`
}

// PipelineResponse is the response body
type PipelineResponse struct {
	SessionID string                      `json:"sessionId"`
	Formation *usecases.FormationResponse `json:"formation,omitempty"`
	Agent1Ms  int                         `json:"agent1Ms"`
	Agent2Ms  int                         `json:"agent2Ms"`
	TotalMs   int                         `json:"totalMs"`
}

// HandlePipeline handles POST /api/v1/pipeline
func (h *PipelineHandler) HandlePipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		http.Error(w, "Query is required", http.StatusBadRequest)
		return
	}

	// Generate session ID if not provided
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	result, err := h.pipelineUC.Execute(r.Context(), usecases.PipelineExecuteRequest{
		SessionID: sessionID,
		Query:     req.Query,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := PipelineResponse{
		SessionID: sessionID,
		Agent1Ms:  result.Agent1Ms,
		Agent2Ms:  result.Agent2Ms,
		TotalMs:   result.TotalMs,
	}

	if result.Formation != nil {
		resp.Formation = &usecases.FormationResponse{
			Mode:    string(result.Formation.Mode),
			Grid:    result.Formation.Grid,
			Widgets: result.Formation.Widgets,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func generateSessionID() string {
	return "sess_" + randomString(16)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[i%len(letters)]
	}
	return string(b)
}
```

### 2. Add FormationResponse Type to Usecases

File: `project/backend/internal/usecases/pipeline_execute.go`

Add after existing types:

```go
// FormationResponse is the JSON-friendly formation for HTTP response
type FormationResponse struct {
	Mode    string           `json:"mode"`
	Grid    *domain.GridConfig `json:"grid,omitempty"`
	Widgets []domain.Widget  `json:"widgets"`
}
```

### 3. Wire Pipeline Handler in main.go

File: `project/backend/cmd/server/main.go`

Add after existing handler creation:

```go
// Create Pipeline handler
pipelineUC := usecases.NewPipelineExecuteUseCase(
	anthropicClient,
	stateAdapter,
	toolRegistry,
	appLog,
)
pipelineHandler := handlers.NewPipelineHandler(pipelineUC)
```

### 4. Add Pipeline Route

File: `project/backend/internal/handlers/routes.go`

Update SetupRoutes function signature and add route:

```go
// SetupRoutes configures all HTTP routes
func SetupRoutes(mux *http.ServeMux, chat *ChatHandler, session *SessionHandler, health *HealthHandler, pipeline *PipelineHandler) {
	// Health checks
	mux.HandleFunc("/health", health.HandleHealth)
	mux.HandleFunc("/ready", health.HandleReady)

	// API v1
	mux.HandleFunc("/api/v1/chat", chat.HandleChat)
	mux.HandleFunc("/api/v1/session/", session.HandleGetSession)
	mux.HandleFunc("/api/v1/pipeline", pipeline.HandlePipeline)
}
```

Update call in main.go accordingly.

### 5. Add Widget Size Model

File: `project/frontend/src/entities/widget/widgetModel.js`

Add WidgetSize enum to existing file:

```javascript
// Widget sizes with constraints
export const WidgetSize = {
  TINY: 'tiny',     // 80-110px, max 2 atoms
  SMALL: 'small',   // 160-220px, max 3 atoms
  MEDIUM: 'medium', // 280-350px, max 5 atoms
  LARGE: 'large',   // 384-460px, max 10 atoms
};
```

### 6. Create Formation Model

File: `project/frontend/src/entities/formation/formationModel.js`

```javascript
// Formation layout modes
export const FormationMode = {
  GRID: 'grid',
  CAROUSEL: 'carousel',
  SINGLE: 'single',
  LIST: 'list',
};

// Formation structure from backend:
// {
//   mode: FormationMode,
//   grid: { rows: number, cols: number },
//   widgets: Widget[]
// }
```

### 7. Create Atom Styles

File: `project/frontend/src/entities/atom/Atom.css`

```css
/* Text styles */
.atom-text { display: inline; }
.atom-text.style-heading { font-size: 1.1em; font-weight: 600; }
.atom-text.style-body { font-size: 1em; }
.atom-text.style-caption { font-size: 0.85em; color: #666; }

/* Number styles */
.atom-number { font-family: monospace; }
.atom-number.format-currency::before { content: '$'; }
.atom-number.format-percent::after { content: '%'; }

/* Price */
.atom-price { font-weight: 600; color: #2563eb; }

/* Image sizes */
.atom-image { object-fit: cover; border-radius: 8px; }
.atom-image.size-small { width: 60px; height: 60px; }
.atom-image.size-medium { width: 120px; height: 120px; }
.atom-image.size-large { width: 100%; max-height: 200px; }

/* Rating */
.atom-rating { color: #f59e0b; letter-spacing: 2px; }

/* Badge variants */
.atom-badge {
  padding: 2px 8px;
  border-radius: 12px;
  font-size: 0.75em;
}
.atom-badge.variant-success { background: #dcfce7; color: #166534; }
.atom-badge.variant-warning { background: #fef3c7; color: #92400e; }
.atom-badge.variant-danger { background: #fee2e2; color: #991b1b; }

/* Button */
.atom-button {
  padding: 8px 16px;
  background: #2563eb;
  color: white;
  border: none;
  border-radius: 6px;
  cursor: pointer;
}
.atom-button:hover { background: #1d4ed8; }

/* Icon */
.atom-icon { font-size: 1.2em; }

/* Divider */
.atom-divider {
  width: 100%;
  height: 1px;
  background: #e5e7eb;
  margin: 8px 0;
}

/* Progress */
.atom-progress {
  width: 100%;
  height: 8px;
  background: #e5e7eb;
  border-radius: 4px;
  overflow: hidden;
}
.atom-progress-bar {
  height: 100%;
  background: #2563eb;
  transition: width 0.3s;
}
```

### 8. Update AtomRenderer with Styles

File: `project/frontend/src/entities/atom/AtomRenderer.jsx`

Update to use CSS classes and handle all atom types:

```jsx
import { AtomType } from './atomModel';
import './Atom.css';

export function AtomRenderer({ atom }) {
  const style = atom.meta?.style || '';
  const format = atom.meta?.format || '';
  const size = atom.meta?.size || 'medium';
  const variant = atom.meta?.variant || '';

  switch (atom.type) {
    case AtomType.TEXT:
      return (
        <span className={`atom-text ${style ? `style-${style}` : ''}`}>
          {atom.value}
        </span>
      );

    case AtomType.NUMBER:
      return (
        <span className={`atom-number ${format ? `format-${format}` : ''}`}>
          {formatNumber(atom.value, format)}
        </span>
      );

    case AtomType.PRICE:
      return (
        <span className="atom-price">
          {atom.meta?.currency || '$'}{atom.value}
        </span>
      );

    case AtomType.IMAGE:
      return (
        <img
          className={`atom-image size-${size}`}
          src={atom.value}
          alt={atom.meta?.label || ''}
        />
      );

    case AtomType.RATING:
      const stars = Math.round(atom.value);
      return (
        <span className="atom-rating">
          {'★'.repeat(stars)}{'☆'.repeat(5 - stars)}
        </span>
      );

    case AtomType.BADGE:
      return (
        <span className={`atom-badge ${variant ? `variant-${variant}` : ''}`}>
          {atom.value}
        </span>
      );

    case AtomType.BUTTON:
      return (
        <button
          className="atom-button"
          data-action={atom.meta?.action}
          onClick={() => handleAction(atom.meta?.action)}
        >
          {atom.value}
        </button>
      );

    case AtomType.ICON:
      return <span className="atom-icon">{atom.value}</span>;

    case AtomType.DIVIDER:
      return <div className="atom-divider" />;

    case AtomType.PROGRESS:
      return (
        <div className="atom-progress">
          <div
            className="atom-progress-bar"
            style={{ width: `${atom.value}%` }}
          />
        </div>
      );

    default:
      return <span>{String(atom.value)}</span>;
  }
}

function formatNumber(value, format) {
  if (format === 'currency') return value.toLocaleString();
  if (format === 'percent') return value;
  if (format === 'compact') return compactNumber(value);
  return value;
}

function compactNumber(num) {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M';
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K';
  return num;
}

function handleAction(action) {
  // TODO: dispatch action to parent
  console.log('Widget action:', action);
}
```

### 9. Create Widget Styles

File: `project/frontend/src/entities/widget/Widget.css`

NOTE: Using regular CSS classes, NOT CSS Modules `composes` syntax.

```css
/* Widget base */
.widget {
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: white;
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0,0,0,0.1);
  overflow: hidden;
}

/* Widget sizes */
.widget.size-tiny {
  width: 80px;
  min-width: 80px;
  max-width: 110px;
  padding: 8px;
}

.widget.size-small {
  width: 160px;
  min-width: 160px;
  max-width: 220px;
  padding: 12px;
}

.widget.size-medium {
  width: 280px;
  min-width: 280px;
  max-width: 350px;
  padding: 16px;
}

.widget.size-large {
  width: 384px;
  min-width: 384px;
  max-width: 460px;
  padding: 20px;
}

/* Widget types - extend .widget base class */
.widget-product-card {
  /* Inherits from .widget via className="widget widget-product-card" */
}

.widget-product-card .atom-image {
  width: 100%;
  aspect-ratio: 1;
  object-fit: cover;
}

.widget-text-block {
  padding: 16px;
}

.widget-quick-replies {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
```

### 10. Update WidgetRenderer with Sizes

File: `project/frontend/src/entities/widget/WidgetRenderer.jsx`

NOTE: Using `widget ${typeClass} ${sizeClass}` pattern for class composition.

```jsx
import { WidgetType } from './widgetModel';
import { AtomRenderer } from '../atom/AtomRenderer';
import './Widget.css';

export function WidgetRenderer({ widget }) {
  const sizeClass = widget.size ? `size-${widget.size}` : 'size-medium';

  switch (widget.type) {
    case WidgetType.PRODUCT_CARD:
      return <ProductCard widget={widget} sizeClass={sizeClass} />;

    case WidgetType.TEXT_BLOCK:
      return <TextBlock widget={widget} sizeClass={sizeClass} />;

    case WidgetType.QUICK_REPLIES:
      return <QuickReplies widget={widget} />;

    default:
      return <DefaultWidget widget={widget} sizeClass={sizeClass} />;
  }
}

function ProductCard({ widget, sizeClass }) {
  return (
    <div className={`widget widget-product-card ${sizeClass}`}>
      {widget.atoms.map((atom, i) => (
        <AtomRenderer key={i} atom={atom} />
      ))}
    </div>
  );
}

function TextBlock({ widget, sizeClass }) {
  return (
    <div className={`widget widget-text-block ${sizeClass}`}>
      {widget.atoms.map((atom, i) => (
        <AtomRenderer key={i} atom={atom} />
      ))}
    </div>
  );
}

function QuickReplies({ widget }) {
  return (
    <div className="widget-quick-replies">
      {widget.atoms.map((atom, i) => (
        <AtomRenderer key={i} atom={atom} />
      ))}
    </div>
  );
}

function DefaultWidget({ widget, sizeClass }) {
  return (
    <div className={`widget ${sizeClass}`}>
      {widget.atoms?.map((atom, i) => (
        <AtomRenderer key={i} atom={atom} />
      ))}
    </div>
  );
}
```

### 11. Create Formation Styles

File: `project/frontend/src/entities/formation/Formation.css`

```css
/* Formation layouts */
.formation {
  display: flex;
  gap: 16px;
  padding: 8px 0;
}

/* Grid mode */
.formation-grid {
  display: grid;
  gap: 16px;
}

.formation-grid.cols-2 { grid-template-columns: repeat(2, 1fr); }
.formation-grid.cols-3 { grid-template-columns: repeat(3, 1fr); }
.formation-grid.cols-4 { grid-template-columns: repeat(4, 1fr); }

/* Carousel mode */
.formation-carousel {
  display: flex;
  overflow-x: auto;
  scroll-snap-type: x mandatory;
  gap: 16px;
  padding: 8px 4px;
}

.formation-carousel > * {
  scroll-snap-align: start;
  flex-shrink: 0;
}

.formation-carousel::-webkit-scrollbar {
  height: 6px;
}

.formation-carousel::-webkit-scrollbar-thumb {
  background: #cbd5e1;
  border-radius: 3px;
}

/* Single mode */
.formation-single {
  display: flex;
  justify-content: center;
}

.formation-single .widget {
  width: 100%;
  max-width: 400px;
}

/* List mode */
.formation-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.formation-list .widget {
  width: 100%;
  max-width: 100%;
}
```

### 12. Create FormationRenderer

File: `project/frontend/src/entities/formation/FormationRenderer.jsx`

```jsx
import { FormationMode } from './formationModel';
import { WidgetRenderer } from '../widget/WidgetRenderer';
import './Formation.css';

export function FormationRenderer({ formation }) {
  if (!formation || !formation.widgets?.length) {
    return null;
  }

  const { mode, grid, widgets } = formation;
  const cols = grid?.cols || 2;

  const layoutClass = getLayoutClass(mode, cols);

  return (
    <div className={layoutClass}>
      {widgets.map((widget) => (
        <WidgetRenderer key={widget.id} widget={widget} />
      ))}
    </div>
  );
}

function getLayoutClass(mode, cols) {
  switch (mode) {
    case FormationMode.GRID:
    case 'grid':
      return `formation-grid cols-${cols}`;

    case FormationMode.CAROUSEL:
    case 'carousel':
      return 'formation-carousel';

    case FormationMode.SINGLE:
    case 'single':
      return 'formation-single';

    case FormationMode.LIST:
    case 'list':
    default:
      return 'formation-list';
  }
}
```

### 13. Create Formation Index

File: `project/frontend/src/entities/formation/index.js`

```javascript
export { FormationRenderer } from './FormationRenderer';
export { FormationMode } from './formationModel';
```

### 14. Update MessageBubble with Backward Compatibility

File: `project/frontend/src/entities/message/MessageBubble.jsx`

IMPORTANT: Keep support for both legacy `widgets` array AND new `formation` object.

```jsx
import { MessageRole } from './messageModel';
import { FormationRenderer } from '../formation/FormationRenderer';
import { WidgetRenderer } from '../widget/WidgetRenderer';

export function MessageBubble({ message }) {
  const isUser = message.role === MessageRole.USER;

  return (
    <div className={`message-bubble ${isUser ? 'user' : 'assistant'}`}>
      {message.content && (
        <div className="message-content">{message.content}</div>
      )}

      {/* Legacy widgets support (without formation) */}
      {message.widgets?.length > 0 && !message.formation && (
        <div className={`message-widgets formation-${message.formationType || 'list'}`}>
          {message.widgets.map((widget) => (
            <WidgetRenderer key={widget.id} widget={widget} />
          ))}
        </div>
      )}

      {/* New formation support */}
      {message.formation && (
        <FormationRenderer formation={message.formation} />
      )}
    </div>
  );
}
```

### 15. Add Pipeline API Function

File: `project/frontend/src/shared/api/apiClient.js`

Add to existing file:

```javascript
// Pipeline API - sends query through Agent 1 → Agent 2 → Formation
export async function sendPipelineQuery(sessionId, query) {
  const body = { query };
  if (sessionId) {
    body.sessionId = sessionId;
  }

  const response = await fetch(`${API_BASE_URL}/pipeline`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(body),
  });

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  // Response: { sessionId, formation, agent1Ms, agent2Ms, totalMs }
  return response.json();
}
```

### 16. Update useChatSubmit to Use Pipeline

File: `project/frontend/src/features/chat/useChatSubmit.js`

Replace sendChatMessage with sendPipelineQuery:

```javascript
import { useCallback } from 'react';
import { sendPipelineQuery } from '../../shared/api/apiClient';
import { MessageRole } from '../../entities/message/messageModel';

const SESSION_STORAGE_KEY = 'chatSessionId';

export function useChatSubmit({ sessionId, addMessage, setLoading, setError, setSessionId }) {
  const submit = useCallback(async (text) => {
    if (!text.trim()) return;

    // Add user message
    addMessage({
      id: Date.now().toString(),
      role: MessageRole.USER,
      content: text,
      timestamp: new Date(),
    });

    setLoading(true);
    setError(null);

    try {
      const response = await sendPipelineQuery(sessionId, text);

      // Save sessionId to localStorage if new
      if (response.sessionId && response.sessionId !== sessionId) {
        localStorage.setItem(SESSION_STORAGE_KEY, response.sessionId);
        setSessionId(response.sessionId);
      }

      // Add assistant message with formation
      addMessage({
        id: (Date.now() + 1).toString(),
        role: MessageRole.ASSISTANT,
        content: '', // No text content, just formation
        formation: response.formation,
        timestamp: new Date(),
      });
    } catch (err) {
      setError(err.message);
      // Add error message
      addMessage({
        id: (Date.now() + 1).toString(),
        role: MessageRole.ASSISTANT,
        content: 'Sorry, something went wrong. Please try again.',
        timestamp: new Date(),
      });
    } finally {
      setLoading(false);
    }
  }, [sessionId, addMessage, setLoading, setError, setSessionId]);

  return { submit };
}
```

### 17. Validation

Run validation commands:

```bash
cd project/backend && go build ./...
cd project/frontend && npm run build
cd project/frontend && npm run lint
```

Manual verification:
1. Start backend server
2. Start frontend dev server
3. Open chat, type "покажи ноутбуки"
4. Verify product cards appear in grid layout
5. Verify widget sizes are respected
6. Verify carousel scrolls horizontally

## Validation Commands

From ADW/adw.yaml:
- `cd project/backend && go build ./...` (required)
- `cd project/frontend && npm run build` (required)
- `cd project/frontend && npm run lint` (optional)

## Acceptance Criteria

- [ ] Pipeline handler created at POST /api/v1/pipeline
- [ ] Pipeline route wired in routes.go and main.go
- [ ] FormationRenderer renders grid/carousel/single/list modes
- [ ] AtomRenderer handles all 10 atom types with proper CSS
- [ ] WidgetRenderer applies size constraints (tiny/small/medium/large)
- [ ] Atom.css has styles for all types and variants
- [ ] Widget.css has size constraint classes (NO CSS Modules composes)
- [ ] Formation.css has layout mode styles
- [ ] MessageBubble supports BOTH legacy widgets AND new formation
- [ ] sendPipelineQuery function added to apiClient
- [ ] useChatSubmit uses sendPipelineQuery instead of sendChatMessage
- [ ] Grid layout shows correct number of columns
- [ ] Carousel scrolls horizontally with snap
- [ ] Backend builds without errors
- [ ] Frontend builds without errors

## Notes

- FormationRenderer receives `formation` from backend (FormationWithData structure)
- Backend sends: `{ mode, grid: {rows, cols}, widgets: [] }`
- Widgets have `size` field from backend template
- Atoms have `meta` with style/format/size/variant
- No need for productToWidget conversion - backend sends ready widgets
- CSS uses regular class combination (`widget widget-product-card size-medium`)
- Action handling (button clicks) is placeholder - implement in Phase 5+
- MessageBubble keeps backward compatibility for legacy widgets array

## Dependencies

- **Phase 3 (Agent 2 + Template)** must be complete - provides Formation JSON
- PipelineExecuteUseCase must exist (from Phase 3)
