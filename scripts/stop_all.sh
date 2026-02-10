#!/bin/bash
# Stop everything (chat + admin)
lsof -ti:8080 | xargs kill -9 2>/dev/null || true
lsof -ti:5173 | xargs kill -9 2>/dev/null || true
lsof -ti:3457 | xargs kill -9 2>/dev/null || true
lsof -ti:8081 | xargs kill -9 2>/dev/null || true
lsof -ti:5174 | xargs kill -9 2>/dev/null || true
pkill -f "node.*vite" 2>/dev/null || true
pkill -f "backend/server" 2>/dev/null || true
pkill -f "project_admin/backend/server" 2>/dev/null || true
echo "All stopped (8080, 5173, 3457, 8081, 5174)"
