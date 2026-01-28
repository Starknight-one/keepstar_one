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

## Output

```
Project stopped.
```
