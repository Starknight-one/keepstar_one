# Stop All

Stop all project servers (chat + admin).

## Instructions

1. Kill chat backend (port 8080):
   ```bash
   lsof -ti:8080 | xargs kill -9 2>/dev/null || true
   ```

2. Kill chat frontend (port 5173):
   ```bash
   lsof -ti:5173 | xargs kill -9 2>/dev/null || true
   ```

3. Kill dev-inspector (port 3457):
   ```bash
   lsof -ti:3457 | xargs kill -9 2>/dev/null || true
   ```

4. Kill admin backend (port 8081):
   ```bash
   lsof -ti:8081 | xargs kill -9 2>/dev/null || true
   ```

5. Kill admin frontend (port 5174):
   ```bash
   lsof -ti:5174 | xargs kill -9 2>/dev/null || true
   ```

6. Kill any lingering processes:
   ```bash
   pkill -f "node.*vite" 2>/dev/null || true
   pkill -f "backend/server" 2>/dev/null || true
   pkill -f "project_admin/backend/server" 2>/dev/null || true
   ```

## Output

```
All services stopped.

Killed:
- Chat Backend (port 8080)
- Chat Frontend (port 5173)
- Dev Inspector (port 3457)
- Admin Backend (port 8081)
- Admin Frontend (port 5174)
```
