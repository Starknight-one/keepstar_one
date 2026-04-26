#!/bin/bash
# Start curator service (backend :8082 + frontend :5175)
DIR="$(cd "$(dirname "$0")/.." && pwd)"

lsof -ti:8082 | xargs kill -9 2>/dev/null || true
lsof -ti:5175 | xargs kill -9 2>/dev/null || true
sleep 1

cd "$DIR/curator/backend" && go build -o server ./cmd/server/ && ./server > /tmp/curator-backend.log 2>&1 &
cd "$DIR/curator/frontend" && npm run dev > /tmp/curator-frontend.log 2>&1 &

sleep 6
curl -sf http://localhost:8082/health > /dev/null && echo "Curator started: backend :8082, frontend :5175" || echo "WARNING: curator backend health check failed (logs: /tmp/curator-backend.log)"
