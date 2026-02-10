# Start Admin

Start the admin panel (backend + frontend).

## Instructions

1. First, kill any existing admin processes to avoid port conflicts:
   ```bash
   lsof -ti:8081 | xargs kill -9 2>/dev/null || true
   lsof -ti:5174 | xargs kill -9 2>/dev/null || true
   ```

2. Wait for ports to be released:
   ```bash
   sleep 2
   ```

3. Build and start the admin Go backend in background:
   ```bash
   cd /Users/starknight/Keepstar_one_ultra/project_admin/backend && go build -o server ./cmd/server/ && ./server > /tmp/admin-backend.log 2>&1 &
   ```

4. Start the admin React frontend in background:
   ```bash
   cd /Users/starknight/Keepstar_one_ultra/project_admin/frontend && npm run dev > /tmp/admin-frontend.log 2>&1 &
   ```

5. Wait for services to start:
   ```bash
   sleep 5
   ```

6. Verify admin backend is running:
   ```bash
   curl -s http://localhost:8081/health
   ```

7. Report the URLs:
   - Admin Frontend: http://localhost:5174
   - Admin Backend API: http://localhost:8081

## Output

```
Admin panel started!

Admin Frontend: http://localhost:5174
Admin Backend API: http://localhost:8081

Logs:
- Backend: /tmp/admin-backend.log
- Frontend: /tmp/admin-frontend.log

Use /stop_admin to shut down admin servers.
```
