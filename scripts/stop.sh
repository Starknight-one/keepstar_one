#!/bin/bash
# Stop chat
lsof -ti:8080 | xargs kill -9 2>/dev/null || true
lsof -ti:5173 | xargs kill -9 2>/dev/null || true
lsof -ti:3457 | xargs kill -9 2>/dev/null || true
pkill -f "backend/server" 2>/dev/null || true
echo "Chat stopped (8080, 5173, 3457)"
