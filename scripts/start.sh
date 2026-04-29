#!/bin/bash
# Start chat widget dev (V4 backend + widget dev server + inspector)
DIR="$(cd "$(dirname "$0")/.." && pwd)"

lsof -ti:8082 | xargs kill -9 2>/dev/null || true
lsof -ti:5173 | xargs kill -9 2>/dev/null || true
lsof -ti:3457 | xargs kill -9 2>/dev/null || true
sleep 1

cd "$DIR/project_v4/backend" && go build -o server ./cmd/server/ && ./server > /tmp/backend.log 2>&1 &
cd "$DIR/project/frontend" && VITE_API_URL=http://localhost:8082/api/v1 npm run dev > /tmp/frontend.log 2>&1 &
cd "$DIR/ADW/dev-inspector" && npm install --silent 2>/dev/null && npm start > /tmp/inspector.log 2>&1 &

sleep 12
curl -sf http://localhost:8082/health > /dev/null && echo "Chat started: backend :8082 (V4), widget :5173, inspector :3457" || echo "WARNING: backend health check failed"
