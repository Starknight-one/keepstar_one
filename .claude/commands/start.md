# Start

Start the project (backend + frontend).

## Instructions

1. Build and start the Go backend in background:
   ```bash
   cd /Users/starknight/Keepstar_one_ultra/project/backend && go build -o server . && ./server &
   ```

2. Start the React frontend in background:
   ```bash
   cd /Users/starknight/Keepstar_one_ultra/project/frontend && npm run dev &
   ```

3. Report the URLs:
   - Frontend: http://localhost:5173
   - Backend: http://localhost:8080

## Output

```
Project started!

Frontend: http://localhost:5173
Backend: http://localhost:8080

Use /stop to shut down servers.
```
