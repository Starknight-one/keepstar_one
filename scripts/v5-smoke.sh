#!/usr/bin/env bash
# V5 prod smoke runner — chunk 15.
#
# Loops a prompt suite against deployed V5 + V4 backends, captures full
# pipeline responses + V4 trace metadata (cost / tokens), writes
# per-prompt JSONs and an aggregated summary.md.
#
# Usage:
#   ./scripts/v5-smoke.sh \
#     [--v5 https://v5-engine-production.up.railway.app] \
#     [--v4 https://v4-engine-production.up.railway.app] \
#     [--tenant hey-babes-cosmetics] \
#     [--prompts scripts/v5-smoke-prompts.json] \
#     [--out docs/v5-smoke/<auto>] \
#     [--limit N]                                # only first N prompts (debug)
#
# Output:
#   <out>/summary.md            — markdown table + aggregates (committed)
#   <out>/prompts.json          — snapshot of input suite (committed)
#   <out>/meta.json             — backend URLs, tenant, run timestamps (committed)
#   <out>/p<NN>.v5.json         — full V5 pipeline response (gitignored)
#   <out>/p<NN>.v4.json         — full V4 pipeline response (gitignored)
#   <out>/p<NN>.v4.trace.json   — matching V4 /debug/traces entry (gitignored)

set -euo pipefail

# Defaults.
V5_URL="https://v5-engine-production.up.railway.app"
V4_URL="https://v4-engine-production.up.railway.app"
TENANT="hey-babes-cosmetics"
PROMPTS_FILE="scripts/v5-smoke-prompts.json"
OUT_DIR=""
LIMIT=0   # 0 = all

# Parse flags.
while [[ $# -gt 0 ]]; do
  case "$1" in
    --v5)      V5_URL="$2"; shift 2;;
    --v4)      V4_URL="$2"; shift 2;;
    --tenant)  TENANT="$2"; shift 2;;
    --prompts) PROMPTS_FILE="$2"; shift 2;;
    --out)     OUT_DIR="$2"; shift 2;;
    --limit)   LIMIT="$2"; shift 2;;
    -h|--help)
      grep '^#' "$0" | sed 's/^# \?//' | head -50
      exit 0;;
    *) echo "unknown flag: $1" >&2; exit 1;;
  esac
done

# Auto out dir if not set.
if [[ -z "$OUT_DIR" ]]; then
  STAMP="$(date -u +'%Y-%m-%d_%H-%M')"
  OUT_DIR="docs/v5-smoke/$STAMP"
fi

mkdir -p "$OUT_DIR"
echo "smoke run → $OUT_DIR"

# Snapshot prompts + meta.
cp "$PROMPTS_FILE" "$OUT_DIR/prompts.json"
RUN_STARTED_AT="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
cat > "$OUT_DIR/meta.json" <<EOF
{
  "v5_url": "$V5_URL",
  "v4_url": "$V4_URL",
  "tenant": "$TENANT",
  "started_at": "$RUN_STARTED_AT",
  "prompts_file": "$PROMPTS_FILE",
  "limit": $LIMIT
}
EOF

# Init sessions on both backends.
echo "init V5 session..."
V5_SID=$(curl -sS -X POST "$V5_URL/api/v1/session/init" \
  -H 'Content-Type: application/json' \
  -H "X-Tenant-Slug: $TENANT" \
  -d '{}' | python3 -c "import json,sys; print(json.load(sys.stdin)['sessionId'])")
echo "  V5 session=$V5_SID"

echo "init V4 session..."
V4_SID=$(curl -sS -X POST "$V4_URL/api/v1/session/init" \
  -H 'Content-Type: application/json' \
  -H "X-Tenant-Slug: $TENANT" \
  -d '{}' | python3 -c "import json,sys; print(json.load(sys.stdin)['sessionId'])")
echo "  V4 session=$V4_SID"

# Persist sessions in meta.json (so per-prompt files can reference them).
python3 - "$OUT_DIR" "$V5_SID" "$V4_SID" <<'PY'
import json, sys
out_dir, v5_sid, v4_sid = sys.argv[1:]
p = f"{out_dir}/meta.json"
m = json.load(open(p))
m["v5_session_id"] = v5_sid
m["v4_session_id"] = v4_sid
json.dump(m, open(p, "w"), indent=2)
PY

# Build prompt list.
PROMPTS_TMP="$(mktemp)"
python3 - "$PROMPTS_FILE" "$LIMIT" "$PROMPTS_TMP" <<'PY'
import json, sys
src, lim, tmp = sys.argv[1], int(sys.argv[2]), sys.argv[3]
data = json.load(open(src))
prompts = data["prompts"]
if lim and lim > 0:
    prompts = prompts[:lim]
with open(tmp, "w") as f:
    for p in prompts:
        f.write(f'{p["id"]}\t{p["tag"]}\t{p["query"]}\n')
PY

ROW_TMP="$(mktemp)"   # tab-separated rows for summary
echo -n "" > "$ROW_TMP"

PROMPT_COUNT="$(wc -l < "$PROMPTS_TMP" | tr -d ' ')"
i=0

while IFS=$'\t' read -r PID TAG QUERY; do
  i=$((i + 1))
  echo "[$i/$PROMPT_COUNT] $PID ($TAG): $QUERY"

  REQ_BODY=$(python3 -c "import json; print(json.dumps({'sessionId': '$V5_SID', 'query': '''$QUERY'''}))")

  # ---- V5 ----
  V5_T0=$(python3 -c "import time; print(time.time())")
  V5_RESP=$(curl -sS -w '\n__HTTP_STATUS__:%{http_code}' -X POST "$V5_URL/api/v1/pipeline" \
    -H 'Content-Type: application/json' \
    -H "X-Tenant-Slug: $TENANT" \
    -d "$REQ_BODY" || echo "__CURL_FAILED__")
  V5_T1=$(python3 -c "import time; print(time.time())")
  V5_WALL=$(python3 -c "print(int(($V5_T1 - $V5_T0) * 1000))")

  V5_STATUS=$(echo "$V5_RESP" | grep -oE '__HTTP_STATUS__:[0-9]+' | tail -1 | cut -d: -f2)
  V5_BODY=$(echo "$V5_RESP" | sed '/__HTTP_STATUS__/d')
  printf '%s' "$V5_BODY" > "$OUT_DIR/$PID.v5.json"

  # ---- V4 ----
  REQ_BODY_V4=$(python3 -c "import json; print(json.dumps({'sessionId': '$V4_SID', 'query': '''$QUERY'''}))")
  V4_T0=$(python3 -c "import time; print(time.time())")
  V4_RESP=$(curl -sS -w '\n__HTTP_STATUS__:%{http_code}' -X POST "$V4_URL/api/v1/pipeline" \
    -H 'Content-Type: application/json' \
    -H "X-Tenant-Slug: $TENANT" \
    -d "$REQ_BODY_V4" || echo "__CURL_FAILED__")
  V4_T1=$(python3 -c "import time; print(time.time())")
  V4_WALL=$(python3 -c "print(int(($V4_T1 - $V4_T0) * 1000))")

  V4_STATUS=$(echo "$V4_RESP" | grep -oE '__HTTP_STATUS__:[0-9]+' | tail -1 | cut -d: -f2)
  V4_BODY=$(echo "$V4_RESP" | sed '/__HTTP_STATUS__/d')
  printf '%s' "$V4_BODY" > "$OUT_DIR/$PID.v4.json"

  # ---- V4 trace lookup ----
  # /debug/traces returns array of in-memory traces; latest first usually.
  V4_TRACE_RESP=$(curl -sS "$V4_URL/debug/traces/?format=json" || echo "[]")
  printf '%s' "$V4_TRACE_RESP" | python3 -c "
import json, sys, os
out_dir, pid, sid, query = sys.argv[1:]
try:
    raw = sys.stdin.read()
    traces = json.loads(raw)
except Exception as e:
    open(f'{out_dir}/{pid}.v4.trace.json', 'w').write('{}')
    sys.exit(0)
matching = [t for t in traces if t.get('sessionId') == sid and t.get('query') == query]
matching.sort(key=lambda t: t.get('timestamp',''), reverse=True)
trace = matching[0] if matching else {}
open(f'{out_dir}/{pid}.v4.trace.json', 'w').write(json.dumps(trace, indent=2, ensure_ascii=False))
" "$OUT_DIR" "$PID" "$V4_SID" "$QUERY"

  # ---- Extract metrics for this row ----
  python3 - "$OUT_DIR" "$PID" "$TAG" "$QUERY" "$V5_STATUS" "$V5_WALL" "$V4_STATUS" "$V4_WALL" "$ROW_TMP" <<'PY'
import json, sys, os

out_dir, pid, tag, query, v5_status, v5_wall, v4_status, v4_wall, row_tmp = sys.argv[1:]

def safe_load(p):
    try:
        return json.load(open(p))
    except Exception:
        return {}

v5 = safe_load(f"{out_dir}/{pid}.v5.json")
v4 = safe_load(f"{out_dir}/{pid}.v4.json")
v4t = safe_load(f"{out_dir}/{pid}.v4.trace.json")

# V5 extracts.
v5_doc_kids = len((v5.get("document") or {}).get("children", []))
v5_tools = v5.get("toolCalls") or []
v5_tool_summary = ", ".join(
    f"{t.get('name','?')}({(t.get('input') or {}).get('preset','-')}/{(t.get('input') or {}).get('mode','-')})"
    for t in v5_tools
) or "-"
v5_usage = v5.get("usage") or {}
v5_in = v5_usage.get("input_tokens", 0)
v5_out = v5_usage.get("output_tokens", 0)
v5_cache_read = v5_usage.get("cache_read_input_tokens", 0)
v5_cost = v5_usage.get("cost_usd", 0.0)
v5_lat = v5.get("latencyMs", 0)
v5_ok = v5_status == "200" and "error" not in v5

# V4 extracts.
v4_widgets = len(((v4.get("formation") or {}).get("widgets")) or [])
v4_lat_self = v4.get("totalMs", 0)
a1 = v4t.get("agent1") or {}
a2 = v4t.get("agent2") or {}
v4_in = (a1.get("inputTokens",0) or 0) + (a2.get("inputTokens",0) or 0)
v4_out = (a1.get("outputTokens",0) or 0) + (a2.get("outputTokens",0) or 0)
# V4 trace JSON uses short field names: `cacheRead` / `cacheWrite`
# (NOT `cacheReadInputTokens` / `cacheCreationInputTokens` like V5).
v4_cache_read = (a1.get("cacheRead",0) or 0) + (a2.get("cacheRead",0) or 0)
v4_cache_write = (a1.get("cacheWrite",0) or 0) + (a2.get("cacheWrite",0) or 0)
v4_cost = (a1.get("costUsd",0.0) or 0.0) + (a2.get("costUsd",0.0) or 0.0)
v4_ok = v4_status == "200"

# Tab-separated row.
with open(row_tmp, "a") as f:
    f.write("\t".join(str(x) for x in [
        pid, tag, query,
        "1" if v5_ok else "0", v5_status, v5_lat, v5_wall, v5_doc_kids, v5_tool_summary,
        v5_in, v5_out, v5_cache_read, f"{v5_cost:.6f}",
        "1" if v4_ok else "0", v4_status, v4_lat_self, v4_wall, v4_widgets,
        v4_in, v4_out, v4_cache_read, f"{v4_cost:.6f}",
    ]) + "\n")

print(f"  V5 {v5_status} {v5_lat}ms ({v5_wall} wall) doc={v5_doc_kids} tools={len(v5_tools)} cost=${v5_cost:.4f}")
print(f"  V4 {v4_status} {v4_lat_self}ms ({v4_wall} wall) widgets={v4_widgets} cost=${v4_cost:.4f}")
PY

  sleep 1
done < "$PROMPTS_TMP"

# ---- Render summary.md ----
RUN_FINISHED_AT="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
python3 - "$OUT_DIR" "$ROW_TMP" "$V5_URL" "$V4_URL" "$TENANT" "$RUN_STARTED_AT" "$RUN_FINISHED_AT" <<'PY'
import sys, statistics, json
from datetime import datetime, timezone

out_dir, row_tmp, v5_url, v4_url, tenant, started, finished = sys.argv[1:]
rows = []
with open(row_tmp) as f:
    for line in f:
        parts = line.rstrip("\n").split("\t")
        if len(parts) < 22:
            continue
        rows.append({
            "pid": parts[0], "tag": parts[1], "query": parts[2],
            "v5_ok": parts[3] == "1", "v5_status": parts[4],
            "v5_lat": int(parts[5]), "v5_wall": int(parts[6]),
            "v5_doc": int(parts[7]), "v5_tools": parts[8],
            "v5_in": int(parts[9]), "v5_out": int(parts[10]),
            "v5_cache": int(parts[11]), "v5_cost": float(parts[12]),
            "v4_ok": parts[13] == "1", "v4_status": parts[14],
            "v4_lat": int(parts[15]), "v4_wall": int(parts[16]),
            "v4_widgets": int(parts[17]),
            "v4_in": int(parts[18]), "v4_out": int(parts[19]),
            "v4_cache": int(parts[20]), "v4_cost": float(parts[21]),
        })

def p(xs, q):
    if not xs: return 0
    s = sorted(xs)
    k = max(0, min(len(s) - 1, int(round(q * (len(s) - 1)))))
    return s[k]

v5_lats = [r["v5_lat"] for r in rows if r["v5_ok"] and r["v5_lat"] > 0]
v4_lats = [r["v4_lat"] for r in rows if r["v4_ok"] and r["v4_lat"] > 0]
v5_costs = [r["v5_cost"] for r in rows]
v4_costs = [r["v4_cost"] for r in rows]
v5_in = [r["v5_in"] for r in rows if r["v5_in"] > 0]
v4_in = [r["v4_in"] for r in rows if r["v4_in"] > 0]
v5_cache = [r["v5_cache"] for r in rows]
v4_cache = [r["v4_cache"] for r in rows]

v5_ok = sum(1 for r in rows if r["v5_ok"])
v4_ok = sum(1 for r in rows if r["v4_ok"])

with open(f"{out_dir}/summary.md", "w") as f:
    f.write(f"# V5 prod smoke — {started}\n\n")
    f.write(f"- V5: {v5_url}\n- V4: {v4_url}\n- Tenant: `{tenant}`\n")
    f.write(f"- Started: {started}\n- Finished: {finished}\n")
    f.write(f"- Prompts: {len(rows)} from `{out_dir}/prompts.json`\n\n")

    f.write("## Per-prompt\n\n")
    f.write("| # | tag | prompt | V5 | V5 lat | V5 doc | V5 tools | V5 in/out | V5 cache | V5 cost | V4 | V4 lat | V4 widgets | V4 in/out | V4 cache | V4 cost |\n")
    f.write("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
    for r in rows:
        v5_mark = "✅" if r["v5_ok"] else f"❌ {r['v5_status']}"
        v4_mark = "✅" if r["v4_ok"] else f"❌ {r['v4_status']}"
        q = r["query"].replace("|", "\\|")
        if len(q) > 40:
            q = q[:38] + "…"
        f.write(f"| {r['pid']} | {r['tag']} | {q} | {v5_mark} | {r['v5_lat']} | {r['v5_doc']} | {r['v5_tools']} | {r['v5_in']}/{r['v5_out']} | {r['v5_cache']} | ${r['v5_cost']:.4f} | {v4_mark} | {r['v4_lat']} | {r['v4_widgets']} | {r['v4_in']}/{r['v4_out']} | {r['v4_cache']} | ${r['v4_cost']:.4f} |\n")

    f.write("\n## Aggregates\n\n")
    f.write("| metric | V5 | V4 | Δ |\n|---|---|---|---|\n")
    def row(label, v5, v4, fmt="{:.0f}", pct=True):
        if not v5 or not v4:
            f.write(f"| {label} | {fmt.format(v5) if v5 else '-'} | {fmt.format(v4) if v4 else '-'} | - |\n")
            return
        delta = (v5 - v4) / v4 * 100 if pct else v5 - v4
        sign = "+" if delta >= 0 else ""
        f.write(f"| {label} | {fmt.format(v5)} | {fmt.format(v4)} | {sign}{delta:.1f}% |\n")

    f.write(f"| Success rate | {v5_ok}/{len(rows)} | {v4_ok}/{len(rows)} | {v5_ok - v4_ok:+d} |\n")
    if v5_lats and v4_lats:
        row("Latency p50 (ms)", p(v5_lats, 0.5), p(v4_lats, 0.5))
        row("Latency p95 (ms)", p(v5_lats, 0.95), p(v4_lats, 0.95))
        row("Latency mean (ms)", statistics.mean(v5_lats), statistics.mean(v4_lats))
    f.write(f"| Total cost (USD) | ${sum(v5_costs):.4f} | ${sum(v4_costs):.4f} | - |\n")
    if v5_costs and v4_costs:
        row("Avg cost/turn (USD)", statistics.mean(v5_costs), statistics.mean(v4_costs), fmt="${:.4f}")
    if v5_in and v4_in:
        row("Avg input tokens", statistics.mean(v5_in), statistics.mean(v4_in))
    if v5_cache and v4_cache:
        row("Avg cache_read tokens", statistics.mean(v5_cache), statistics.mean(v4_cache))

    failures = [r for r in rows if not r["v5_ok"]]
    if failures:
        f.write("\n## V5 failures\n\n")
        for r in failures:
            f.write(f"- **{r['pid']}** ({r['tag']}): HTTP {r['v5_status']} on `{r['query']}` — see `{r['pid']}.v5.json`\n")

    v4_failures = [r for r in rows if not r["v4_ok"]]
    if v4_failures:
        f.write("\n## V4 failures\n\n")
        for r in v4_failures:
            f.write(f"- **{r['pid']}** ({r['tag']}): HTTP {r['v4_status']} on `{r['query']}` — see `{r['pid']}.v4.json`\n")

print(f"\nsummary → {out_dir}/summary.md")
print(f"V5 success: {v5_ok}/{len(rows)}; V4 success: {v4_ok}/{len(rows)}")
if v5_lats and v4_lats:
    print(f"V5 lat p50: {p(v5_lats, 0.5):.0f}ms; V4 lat p50: {p(v4_lats, 0.5):.0f}ms")
print(f"V5 total cost: ${sum(v5_costs):.4f}; V4 total cost: ${sum(v4_costs):.4f}")
PY

rm -f "$PROMPTS_TMP" "$ROW_TMP"
echo "done."
