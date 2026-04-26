#!/bin/bash
lsof -ti:8082 | xargs kill -9 2>/dev/null || true
lsof -ti:5175 | xargs kill -9 2>/dev/null || true
echo "Curator stopped"
