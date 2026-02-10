#!/bin/bash
# Start admin panel (backend + frontend)
DIR="$(cd "$(dirname "$0")/.." && pwd)"

lsof -ti:8081 | xargs kill -9 2>/dev/null || true
lsof -ti:5174 | xargs kill -9 2>/dev/null || true
sleep 1

cd "$DIR/project_admin/backend" && go build -o server ./cmd/server/ && ./server > /tmp/admin-backend.log 2>&1 &
cd "$DIR/project_admin/frontend" && npm run dev > /tmp/admin-frontend.log 2>&1 &

sleep 8
curl -sf http://localhost:8081/health > /dev/null && echo "Admin started: backend :8081, frontend :5174" || echo "WARNING: admin backend health check failed"
