#!/usr/bin/env bash
# V5 prod smoke runner.
#
# Loops a prompt suite against the deployed V5 backend, captures full
# pipeline responses, writes per-prompt JSONs and an aggregated summary.md.
#
# Usage:
#   ./scripts/v5-smoke.sh \
#     [--v5 https://v5-engine-production.up.railway.app] \
#     [--tenant hey-babes-cosmetics] \
#     [--prompts scripts/v5-smoke-prompts.json] \
#     [--out docs/v5-smoke/<auto>] \
#     [--limit N]                                # only first N prompts (debug)
#
# Output:
#   <out>/summary.md       — markdown table + aggregates (committed)
#   <out>/prompts.json     — snapshot of input suite (committed)
#   <out>/meta.json        — backend URL, tenant, run timestamps (committed)
#   <out>/p<NN>.v5.json    — full V5 pipeline response (gitignored)

set -euo pipefail

# Defaults.
V5_URL="https://v5-engine-production.up.railway.app"
TENANT="hey-babes-cosmetics"
PROMPTS_FILE="scripts/v5-smoke-prompts.json"
OUT_DIR=""
LIMIT=0   # 0 = all

# Parse flags.
while [[ $# -gt 0 ]]; do
  case "$1" in
    --v5)      V5_URL="$2"; shift 2;;
    --tenant)  TENANT="$2"; shift 2;;
    --prompts) PROMPTS_FILE="$2"; shift 2;;
    --out)     OUT_DIR="$2"; shift 2;;
    --limit)   LIMIT="$2"; shift 2;;
    -h|--help)
      grep '^#' "$0" | sed 's/^# \?//' | head -30
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
  "tenant": "$TENANT",
  "started_at": "$RUN_STARTED_AT",
  "prompts_file": "$PROMPTS_FILE",
  "limit": $LIMIT
}
EOF

# Init V5 session.
echo "init V5 session..."
V5_SID=$(curl -sS -X POST "$V5_URL/api/v1/session/init" \
  -H 'Content-Type: application/json' \
  -H "X-Tenant-Slug: $TENANT" \
  -d '{}' | python3 -c "import json,sys; print(json.load(sys.stdin)['sessionId'])")
echo "  V5 session=$V5_SID"

# Persist session id in meta.json (per-prompt files reference it).
python3 - "$OUT_DIR" "$V5_SID" <<'PY'
import json, sys
out_dir, v5_sid = sys.argv[1:]
p = f"{out_dir}/meta.json"
m = json.load(open(p))
m["v5_session_id"] = v5_sid
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

ROW_TMP="$(mktemp)"
echo -n "" > "$ROW_TMP"

PROMPT_COUNT="$(wc -l < "$PROMPTS_TMP" | tr -d ' ')"
i=0

while IFS=$'\t' read -r PID TAG QUERY; do
  i=$((i + 1))
  echo "[$i/$PROMPT_COUNT] $PID ($TAG): $QUERY"

  REQ_BODY=$(python3 -c "import json; print(json.dumps({'sessionId': '$V5_SID', 'query': '''$QUERY'''}))")

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

  # Extract metrics for the summary row.
  python3 - "$OUT_DIR" "$PID" "$TAG" "$QUERY" "$V5_STATUS" "$V5_WALL" "$ROW_TMP" <<'PY'
import json, sys

out_dir, pid, tag, query, v5_status, v5_wall, row_tmp = sys.argv[1:]

def safe_load(p):
    try:
        return json.load(open(p))
    except Exception:
        return {}

v5 = safe_load(f"{out_dir}/{pid}.v5.json")

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

with open(row_tmp, "a") as f:
    f.write("\t".join(str(x) for x in [
        pid, tag, query,
        "1" if v5_ok else "0", v5_status, v5_lat, v5_wall, v5_doc_kids, v5_tool_summary,
        v5_in, v5_out, v5_cache_read, f"{v5_cost:.6f}",
    ]) + "\n")

print(f"  V5 {v5_status} {v5_lat}ms ({v5_wall} wall) doc={v5_doc_kids} tools={len(v5_tools)} cost=${v5_cost:.4f}")
PY

  sleep 1
done < "$PROMPTS_TMP"

# Render summary.md.
RUN_FINISHED_AT="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
python3 - "$OUT_DIR" "$ROW_TMP" "$V5_URL" "$TENANT" "$RUN_STARTED_AT" "$RUN_FINISHED_AT" <<'PY'
import sys, statistics

out_dir, row_tmp, v5_url, tenant, started, finished = sys.argv[1:]
rows = []
with open(row_tmp) as f:
    for line in f:
        parts = line.rstrip("\n").split("\t")
        if len(parts) < 13:
            continue
        rows.append({
            "pid": parts[0], "tag": parts[1], "query": parts[2],
            "v5_ok": parts[3] == "1", "v5_status": parts[4],
            "v5_lat": int(parts[5]), "v5_wall": int(parts[6]),
            "v5_doc": int(parts[7]), "v5_tools": parts[8],
            "v5_in": int(parts[9]), "v5_out": int(parts[10]),
            "v5_cache": int(parts[11]), "v5_cost": float(parts[12]),
        })

def p(xs, q):
    if not xs: return 0
    s = sorted(xs)
    k = max(0, min(len(s) - 1, int(round(q * (len(s) - 1)))))
    return s[k]

v5_lats = [r["v5_lat"] for r in rows if r["v5_ok"] and r["v5_lat"] > 0]
v5_costs = [r["v5_cost"] for r in rows]
v5_in = [r["v5_in"] for r in rows if r["v5_in"] > 0]
v5_cache = [r["v5_cache"] for r in rows]

v5_ok = sum(1 for r in rows if r["v5_ok"])

with open(f"{out_dir}/summary.md", "w") as f:
    f.write(f"# V5 prod smoke — {started}\n\n")
    f.write(f"- V5: {v5_url}\n- Tenant: `{tenant}`\n")
    f.write(f"- Started: {started}\n- Finished: {finished}\n")
    f.write(f"- Prompts: {len(rows)} from `{out_dir}/prompts.json`\n\n")

    f.write("## Per-prompt\n\n")
    f.write("| # | tag | prompt | V5 | lat | doc | tools | in/out | cache | cost |\n")
    f.write("|---|---|---|---|---|---|---|---|---|---|\n")
    for r in rows:
        v5_mark = "✅" if r["v5_ok"] else f"❌ {r['v5_status']}"
        q = r["query"].replace("|", "\\|")
        if len(q) > 40:
            q = q[:38] + "…"
        f.write(f"| {r['pid']} | {r['tag']} | {q} | {v5_mark} | {r['v5_lat']} | {r['v5_doc']} | {r['v5_tools']} | {r['v5_in']}/{r['v5_out']} | {r['v5_cache']} | ${r['v5_cost']:.4f} |\n")

    f.write("\n## Aggregates\n\n")
    f.write("| metric | value |\n|---|---|\n")
    f.write(f"| Success rate | {v5_ok}/{len(rows)} |\n")
    if v5_lats:
        f.write(f"| Latency p50 (ms) | {p(v5_lats, 0.5):.0f} |\n")
        f.write(f"| Latency p95 (ms) | {p(v5_lats, 0.95):.0f} |\n")
        f.write(f"| Latency mean (ms) | {statistics.mean(v5_lats):.0f} |\n")
    f.write(f"| Total cost (USD) | ${sum(v5_costs):.4f} |\n")
    if v5_costs:
        f.write(f"| Avg cost/turn (USD) | ${statistics.mean(v5_costs):.4f} |\n")
    if v5_in:
        f.write(f"| Avg input tokens | {statistics.mean(v5_in):.0f} |\n")
    if v5_cache:
        f.write(f"| Avg cache_read tokens | {statistics.mean(v5_cache):.0f} |\n")

    failures = [r for r in rows if not r["v5_ok"]]
    if failures:
        f.write("\n## V5 failures\n\n")
        for r in failures:
            f.write(f"- **{r['pid']}** ({r['tag']}): HTTP {r['v5_status']} on `{r['query']}` — see `{r['pid']}.v5.json`\n")

print(f"\nsummary → {out_dir}/summary.md")
print(f"V5 success: {v5_ok}/{len(rows)}")
if v5_lats:
    print(f"V5 lat p50: {p(v5_lats, 0.5):.0f}ms")
print(f"V5 total cost: ${sum(v5_costs):.4f}")
PY

rm -f "$PROMPTS_TMP" "$ROW_TMP"
echo "done."
