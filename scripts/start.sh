#!/bin/bash
# Start chat widget dev (V5 backend + widget dev server + inspector)
DIR="$(cd "$(dirname "$0")/.." && pwd)"

lsof -ti:8084 | xargs kill -9 2>/dev/null || true
lsof -ti:5173 | xargs kill -9 2>/dev/null || true
lsof -ti:3457 | xargs kill -9 2>/dev/null || true
sleep 1

# project_v5/.env supplies DATABASE_URL / ANTHROPIC_API_KEY / OPENAI_API_KEY /
# TENANT_SLUG / LLM_MODEL. PORT defaults to 8084.
cd "$DIR/project_v5/backend" && set -a && source "$DIR/project_v5/.env" && set +a && go build -o server ./cmd/server/ && ./server > /tmp/backend.log 2>&1 &
cd "$DIR/project_v5/frontend" && VITE_API_URL=http://localhost:8084/api/v1 npm run dev > /tmp/frontend.log 2>&1 &
cd "$DIR/ADW/dev-inspector" && npm install --silent 2>/dev/null && npm start > /tmp/inspector.log 2>&1 &

sleep 12
curl -sf http://localhost:8084/readyz > /dev/null && echo "Chat started: backend :8084 (V5), widget :5173, inspector :3457" || echo "WARNING: backend ready check failed (logs: /tmp/backend.log)"
