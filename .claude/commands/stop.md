# Stop

Stop the project servers.

## Instructions

1. Kill the backend server:
   ```bash
   pkill -f "project/backend/server" || true
   ```

2. Kill the frontend dev server:
   ```bash
   pkill -f "vite" || true
   ```

3. Kill the dev-inspector:
   ```bash
   pkill -f "dev-inspector/server.js" || true
   ```

## Output

```
Project stopped.
```
