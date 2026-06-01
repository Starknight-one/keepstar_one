#!/bin/bash
# Start everything (V5 chat engine)
DIR="$(cd "$(dirname "$0")" && pwd)"
"$DIR/stop_all.sh"
sleep 1
"$DIR/start.sh"
echo "All services running"
