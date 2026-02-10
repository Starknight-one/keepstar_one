# Start All

Start both the chat project and admin panel.

## Instructions

1. First, kill all existing processes:
   ```bash
   lsof -ti:8080 | xargs kill -9 2>/dev/null || true
   lsof -ti:5173 | xargs kill -9 2>/dev/null || true
   lsof -ti:3457 | xargs kill -9 2>/dev/null || true
   lsof -ti:8081 | xargs kill -9 2>/dev/null || true
   lsof -ti:5174 | xargs kill -9 2>/dev/null || true
   ```

2. Wait for ports to be released:
   ```bash
   sleep 2
   ```

3. Build and start the chat Go backend:
   ```bash
   cd /Users/starknight/Keepstar_one_ultra/project/backend && go build -o server ./cmd/server/ && ./server > /tmp/backend.log 2>&1 &
   ```

4. Start the chat React frontend:
   ```bash
   cd /Users/starknight/Keepstar_one_ultra/project/frontend && npm run dev > /tmp/frontend.log 2>&1 &
   ```

5. Start dev-inspector:
   ```bash
   cd /Users/starknight/Keepstar_one_ultra/ADW/dev-inspector && npm install --silent 2>/dev/null && npm start > /tmp/inspector.log 2>&1 &
   ```

6. Build and start the admin Go backend:
   ```bash
   cd /Users/starknight/Keepstar_one_ultra/project_admin/backend && go build -o server ./cmd/server/ && ./server > /tmp/admin-backend.log 2>&1 &
   ```

7. Start the admin React frontend:
   ```bash
   cd /Users/starknight/Keepstar_one_ultra/project_admin/frontend && npm run dev > /tmp/admin-frontend.log 2>&1 &
   ```

8. Wait for all services to start:
   ```bash
   sleep 5
   ```

9. Verify both backends are running:
   ```bash
   curl -s http://localhost:8080/health
   curl -s http://localhost:8081/health
   ```

10. Report all URLs.

## Output

```
All services started!

Chat:
  Frontend: http://localhost:5173
  Frontend + Inspector: http://localhost:3457 (Ctrl+Shift+K to activate)
  Backend API: http://localhost:8080
  Pipeline Traces: http://localhost:8080/debug/traces/

Admin:
  Frontend: http://localhost:5174
  Backend API: http://localhost:8081

Logs:
- Chat Backend: /tmp/backend.log
- Chat Frontend: /tmp/frontend.log
- Admin Backend: /tmp/admin-backend.log
- Admin Frontend: /tmp/admin-frontend.log

Use /stop_all to shut down everything.
```
