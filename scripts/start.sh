#!/bin/bash
# Start chat widget dev (backend + widget dev server + inspector)
DIR="$(cd "$(dirname "$0")/.." && pwd)"

lsof -ti:8080 | xargs kill -9 2>/dev/null || true
lsof -ti:5173 | xargs kill -9 2>/dev/null || true
lsof -ti:3457 | xargs kill -9 2>/dev/null || true
sleep 1

cd "$DIR/project/backend" && go build -o server ./cmd/server/ && ./server > /tmp/backend.log 2>&1 &
cd "$DIR/project/frontend" && npm run dev > /tmp/frontend.log 2>&1 &
cd "$DIR/ADW/dev-inspector" && npm install --silent 2>/dev/null && npm start > /tmp/inspector.log 2>&1 &

sleep 12
curl -sf http://localhost:8080/health > /dev/null && echo "Chat started: backend :8080, widget :5173, inspector :3457" || echo "WARNING: backend health check failed"
