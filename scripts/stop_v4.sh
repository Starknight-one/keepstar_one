#!/bin/bash
# Stop V4 backend

if [ -f /tmp/keepstar_v4.pid ]; then
    PID=$(cat /tmp/keepstar_v4.pid)
    if kill -0 "$PID" 2>/dev/null; then
        kill "$PID"
        echo "V4 backend (PID $PID) stopped"
    else
        echo "V4 backend not running"
    fi
    rm -f /tmp/keepstar_v4.pid
else
    echo "No PID file found. Trying to find process..."
    pkill -f "project_v4/backend" && echo "Killed" || echo "Not running"
fi
