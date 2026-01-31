# Start

Start the project (backend + frontend + dev-inspector).

## Instructions

1. Build and start the Go backend in background:
   ```bash
   cd /Users/starknight/Keepstar_one_ultra/project/backend && go build -o server ./cmd/server/ && ./server &
   ```

2. Start the React frontend in background:
   ```bash
   cd /Users/starknight/Keepstar_one_ultra/project/frontend && npm run dev &
   ```

3. Start dev-inspector (install deps if needed):
   ```bash
   cd /Users/starknight/Keepstar_one_ultra/ADW/dev-inspector && npm install --silent && npm start &
   ```

4. Report the URLs:
   - Frontend: http://localhost:5173
   - Frontend + Inspector: http://localhost:3457
   - Backend: http://localhost:8080

## Output

```
Project started!

Frontend: http://localhost:5173
Frontend + Inspector: http://localhost:3457 (Ctrl+Shift+K to activate)
Backend: http://localhost:8080

Use /stop to shut down servers.
```
