# Start

Start the project (backend + frontend + dev-inspector).

## Instructions

1. First, kill any existing processes to avoid port conflicts:
   ```bash
   lsof -ti:8080 | xargs kill -9 2>/dev/null || true
   lsof -ti:5173 | xargs kill -9 2>/dev/null || true
   lsof -ti:3457 | xargs kill -9 2>/dev/null || true
   ```

2. Wait for ports to be released:
   ```bash
   sleep 2
   ```

3. Build and start the Go backend in background (with log redirect):
   ```bash
   cd /Users/starknight/Keepstar_one_ultra/project/backend && go build -o server ./cmd/server/ && ./server > /tmp/backend.log 2>&1 &
   ```

4. Start the React frontend in background:
   ```bash
   cd /Users/starknight/Keepstar_one_ultra/project/frontend && npm run dev > /tmp/frontend.log 2>&1 &
   ```

5. Start dev-inspector (install deps if needed):
   ```bash
   cd /Users/starknight/Keepstar_one_ultra/ADW/dev-inspector && npm install --silent 2>/dev/null && npm start > /tmp/inspector.log 2>&1 &
   ```

6. Wait for services to start:
   ```bash
   sleep 5
   ```

7. Verify backend is running:
   ```bash
   curl -s http://localhost:8080/health
   ```

8. Report the URLs:
   - Frontend: http://localhost:5173
   - Frontend + Inspector: http://localhost:3457
   - Backend API: http://localhost:8080
   - Debug Console: http://localhost:8080/debug/session/

## Output

```
Project started!

Frontend: http://localhost:5173
Frontend + Inspector: http://localhost:3457 (Ctrl+Shift+K to activate)
Backend API: http://localhost:8080
Debug Console: http://localhost:8080/debug/session/

Logs:
- Backend: /tmp/backend.log
- Frontend: /tmp/frontend.log

Use /stop to shut down servers.
```
