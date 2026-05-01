#!/usr/bin/env bash
# SessionEnd hook — auto-refresh vertical-expert YAMLs by re-reading current code.
#
# Fires once when user closes Claude Code (exit | logout | prompt_input_exit).
# Skips on /clear (user keeps working). Always exit 0 (never blocks close).
#
# Spawns a fresh headless `claude --print "/sync-experts --auto"` so the
# self-improve runs with a clean context budget — no need to /compact the
# user's session beforehand.

set -euo pipefail

INPUT=$(cat)
REASON=$(echo "$INPUT" | jq -r '.reason // "exit"' 2>/dev/null || echo "exit")

# /clear is just a context wipe — user is still here. Don't sync.
if [ "$REASON" = "clear" ]; then
  exit 0
fi

# Recursion guard #1 — env var. If our parent process marked this as a nested
# invocation (we set this when spawning the headless `claude --print`),
# this hook is firing inside our own child. Skip.
if [ "${EXPERTS_KEEPER_NESTED:-}" = "1" ]; then
  exit 0
fi

# Recursion guard #2 — process check. If a previous sync-experts headless
# claude is still running, don't spawn another one. Catches the case where
# env propagation didn't work and prevents pile-up across rapid SessionEnd
# events from multiple terminals closing at once.
if pgrep -f "claude.*--print.*sync-experts" > /dev/null 2>&1; then
  exit 0
fi

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$(pwd)}"
cd "$PROJECT_DIR" || exit 0

# Skip if repo has no changes vs origin/main and no working-tree edits.
# Fast no-op for read-only sessions.
if git diff --quiet origin/main 2>/dev/null && \
   git diff --quiet HEAD 2>/dev/null && \
   [ -z "$(git ls-files --others --exclude-standard)" ]; then
  exit 0
fi

# Lock via atomic mkdir (works on macOS where flock is absent).
# Only one process can create the dir; others get EEXIST and exit.
LOCK_DIR="${PROJECT_DIR}/.claude/.sync.lock.d"
if ! mkdir "$LOCK_DIR" 2>/dev/null; then
  # Another sync is already running — bail.
  exit 0
fi
# Cleanup lock dir on any exit.
trap 'rmdir "$LOCK_DIR" 2>/dev/null || true' EXIT

LOG_FILE="${PROJECT_DIR}/.claude/.last_sync.log"
echo "[$(date -u '+%Y-%m-%d %H:%M:%S UTC')] SessionEnd reason=$REASON — running /sync-experts --auto" > "$LOG_FILE"

# Resolve `claude` binary. Fallback to common installs if PATH is minimal.
CLAUDE_BIN="$(command -v claude 2>/dev/null || true)"
if [ -z "$CLAUDE_BIN" ]; then
  for p in /usr/local/bin/claude /opt/homebrew/bin/claude "$HOME/.local/bin/claude"; do
    if [ -x "$p" ]; then CLAUDE_BIN="$p"; break; fi
  done
fi
if [ -z "$CLAUDE_BIN" ]; then
  echo "[experts-keeper] claude binary not found in PATH or common locations — skipping" >> "$LOG_FILE"
  exit 0
fi

# Run in background to avoid stalling Claude Code's exit. Output to log.
# EXPERTS_KEEPER_NESTED=1 propagates to the spawned claude → its own
# SessionEnd hook (when --print finishes) will short-circuit on guard #1.
EXPERTS_KEEPER_NESTED=1 nohup "$CLAUDE_BIN" --print --dangerously-skip-permissions \
  "/sync-experts --auto" \
  >> "$LOG_FILE" 2>&1 &
disown 2>/dev/null || true

exit 0
