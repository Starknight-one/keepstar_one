#!/bin/bash
# Start V4 Pencil Engine backend on port 8082

cd "$(dirname "$0")/../project_v4/backend" || exit 1

echo "Starting V4 backend on port 8082..."
go run ./cmd/server/ &
V4_PID=$!
echo "V4 backend PID: $V4_PID"
echo $V4_PID > /tmp/keepstar_v4.pid
echo "V4 backend started. Test: curl http://localhost:8082/health"
