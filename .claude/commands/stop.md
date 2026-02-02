# Stop

Stop the project servers.

## Instructions

1. Kill the backend server (port 8080):
   ```bash
   lsof -ti:8080 | xargs kill -9 2>/dev/null || true
   ```

2. Kill the frontend dev server (port 5173):
   ```bash
   lsof -ti:5173 | xargs kill -9 2>/dev/null || true
   ```

3. Kill the dev-inspector (port 3457):
   ```bash
   lsof -ti:3457 | xargs kill -9 2>/dev/null || true
   ```

4. Also kill any lingering node/vite processes:
   ```bash
   pkill -f "node.*vite" 2>/dev/null || true
   pkill -f "backend/server" 2>/dev/null || true
   ```

## Output

```
Project stopped.

Killed:
- Backend (port 8080)
- Frontend (port 5173)
- Dev Inspector (port 3457)
```
