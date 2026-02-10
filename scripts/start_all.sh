#!/bin/bash
# Start everything (chat + admin)
DIR="$(cd "$(dirname "$0")" && pwd)"
"$DIR/stop_all.sh"
sleep 1
"$DIR/start.sh"
"$DIR/start_admin.sh"
echo "All services running"
