#!/bin/bash
# Stop admin panel
lsof -ti:8081 | xargs kill -9 2>/dev/null || true
lsof -ti:5174 | xargs kill -9 2>/dev/null || true
pkill -f "project_admin/backend/server" 2>/dev/null || true
echo "Admin stopped (8081, 5174)"
