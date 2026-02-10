# Stop Admin

Stop the admin panel servers.

## Instructions

1. Kill the admin backend server (port 8081):
   ```bash
   lsof -ti:8081 | xargs kill -9 2>/dev/null || true
   ```

2. Kill the admin frontend dev server (port 5174):
   ```bash
   lsof -ti:5174 | xargs kill -9 2>/dev/null || true
   ```

3. Also kill any lingering admin processes:
   ```bash
   pkill -f "project_admin/backend/server" 2>/dev/null || true
   ```

## Output

```
Admin panel stopped.

Killed:
- Admin Backend (port 8081)
- Admin Frontend (port 5174)
```
