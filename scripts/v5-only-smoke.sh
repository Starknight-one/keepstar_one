#!/usr/bin/env bash
# V5-only smoke — 30 English prompts in one shared session.
#
# Companion to scripts/v5-smoke.sh (which compares V4 vs V5). This one
# is V5-only and produces a narrative-style summary so we can SEE what
# V5 actually does per prompt — which tools fire, which preset Agent2
# picks, rebuild vs modify, ops count, document children, text content
# samples — instead of just metric aggregates.
#
# Usage:
#   ./scripts/v5-only-smoke.sh \
#     [--v5 https://v5-engine-production.up.railway.app] \
#     [--tenant hey-babes-cosmetics] \
#     [--prompts scripts/v5-only-smoke-prompts.json] \
#     [--out docs/v5-smoke/<auto>_v5only] \
#     [--limit N]
#
# Output:
#   <out>/summary.md           — narrative per-prompt + analysis (committed)
#   <out>/prompts.json         — input snapshot (committed)
#   <out>/meta.json            — session id, timestamps (committed)
#   <out>/pNN.v5.json          — full response (gitignored)

set -euo pipefail

V5_URL="https://v5-engine-production.up.railway.app"
TENANT="hey-babes-cosmetics"
PROMPTS_FILE="scripts/v5-only-smoke-prompts.json"
OUT_DIR=""
LIMIT=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --v5)      V5_URL="$2"; shift 2;;
    --tenant)  TENANT="$2"; shift 2;;
    --prompts) PROMPTS_FILE="$2"; shift 2;;
    --out)     OUT_DIR="$2"; shift 2;;
    --limit)   LIMIT="$2"; shift 2;;
    -h|--help) grep '^#' "$0" | sed 's/^# \?//' | head -25; exit 0;;
    *) echo "unknown flag: $1" >&2; exit 1;;
  esac
done

if [[ -z "$OUT_DIR" ]]; then
  STAMP="$(date -u +'%Y-%m-%d_%H-%M')"
  OUT_DIR="docs/v5-smoke/${STAMP}_v5only"
fi

mkdir -p "$OUT_DIR"
echo "smoke run → $OUT_DIR"

cp "$PROMPTS_FILE" "$OUT_DIR/prompts.json"
RUN_STARTED_AT="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"

# One shared session for all prompts.
SID=$(curl -sS -X POST "$V5_URL/api/v1/session/init" \
  -H 'Content-Type: application/json' \
  -H "X-Tenant-Slug: $TENANT" \
  -d '{}' | python3 -c "import json,sys; print(json.load(sys.stdin)['sessionId'])")
echo "shared session=$SID"

cat > "$OUT_DIR/meta.json" <<EOF
{
  "v5_url": "$V5_URL",
  "tenant": "$TENANT",
  "session_id": "$SID",
  "started_at": "$RUN_STARTED_AT",
  "single_session": true,
  "limit": $LIMIT
}
EOF

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

PROMPT_COUNT="$(wc -l < "$PROMPTS_TMP" | tr -d ' ')"
i=0

while IFS=$'\t' read -r PID TAG QUERY; do
  i=$((i + 1))
  echo "[$i/$PROMPT_COUNT] $PID [$TAG]: $QUERY"

  REQ_BODY=$(python3 -c "import json; print(json.dumps({'sessionId': '$SID', 'query': '''$QUERY'''}))")

  T0=$(python3 -c "import time; print(time.time())")
  RESP=$(curl -sS -w '\n__HTTP_STATUS__:%{http_code}' -X POST "$V5_URL/api/v1/pipeline" \
    -H 'Content-Type: application/json' \
    -H "X-Tenant-Slug: $TENANT" \
    -d "$REQ_BODY" || echo "__CURL_FAILED__")
  T1=$(python3 -c "import time; print(time.time())")
  WALL=$(python3 -c "print(int(($T1 - $T0) * 1000))")

  STATUS=$(echo "$RESP" | grep -oE '__HTTP_STATUS__:[0-9]+' | tail -1 | cut -d: -f2)
  BODY=$(echo "$RESP" | sed '/__HTTP_STATUS__/d')
  printf '%s' "$BODY" > "$OUT_DIR/$PID.v5.json"
  echo "  HTTP=$STATUS wall=${WALL}ms"

  sleep 1
done < "$PROMPTS_TMP"

# Render narrative summary.
RUN_FINISHED_AT="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
python3 - "$OUT_DIR" "$V5_URL" "$TENANT" "$SID" "$RUN_STARTED_AT" "$RUN_FINISHED_AT" <<'PY'
import json, sys, statistics, os
from collections import Counter, defaultdict

out_dir, v5_url, tenant, sid, started, finished = sys.argv[1:]
prompts = json.load(open(f"{out_dir}/prompts.json"))["prompts"]

def safe_load(p):
    try: return json.load(open(p))
    except Exception: return {}

def collect_text_samples(node, samples, max_n=3):
    """Walk doc; collect first N text contents (literal, not bound)."""
    if len(samples) >= max_n: return
    t = node.get("type")
    if t == "text":
        c = node.get("content")
        if isinstance(c, str) and c.strip() and not node.get("__bound"):
            samples.append(c[:60])
    if t == "frame" and node.get("reusable"):
        return  # skip component definitions
    for child in (node.get("children") or []):
        collect_text_samples(child, samples, max_n)

def count_nodes(doc):
    """Count non-reusable nodes by type."""
    counts = Counter()
    def walk(n):
        if n.get("reusable"): return
        counts[n.get("type", "?")] += 1
        for c in (n.get("children") or []): walk(c)
    for c in (doc.get("children") or []): walk(c)
    return counts

def card_count(doc):
    """Estimate visible card/widget count: top-level children of first frame
    if it has direction=row or wrap, else top-level frames count."""
    kids = doc.get("children") or []
    if not kids: return 0
    first = kids[0]
    if first.get("type") == "frame" and (first.get("layout") or {}).get("wrap"):
        return len([c for c in (first.get("children") or []) if c.get("type") == "frame"])
    return len([c for c in kids if c.get("type") == "frame"])

rows = []
for p in prompts:
    pid = p["id"]
    fpath = f"{out_dir}/{pid}.v5.json"
    if not os.path.exists(fpath):
        # Skipped by --limit; not a failure.
        continue
    d = safe_load(fpath)
    if not d:
        rows.append({"pid": pid, "tag": p["tag"], "query": p["query"], "ok": False, "err": "no response"})
        continue

    err = d.get("error")
    doc = d.get("document") or {}
    tools = d.get("toolCalls") or []
    usage = d.get("usage") or {}
    text_samples = []
    for c in (doc.get("children") or []):
        collect_text_samples(c, text_samples)
    nc = count_nodes(doc)

    rows.append({
        "pid": pid, "tag": p["tag"], "query": p["query"],
        "ok": err is None,
        "err": err,
        "lat": d.get("latencyMs", 0),
        "a1": d.get("agent1Ms", 0),
        "a2": d.get("agent2Ms", 0),
        "tools": [{
            "name": t.get("name"),
            "preset": (t.get("input") or {}).get("preset"),
            "mode":   (t.get("input") or {}).get("mode"),
            "replicate": (t.get("input") or {}).get("replicate"),
            "ops":   len((t.get("input") or {}).get("ops") or []),
        } for t in tools],
        "card_count": card_count(doc),
        "node_counts": dict(nc),
        "text_samples": text_samples,
        "cost": usage.get("cost_usd", 0),
        "in":   usage.get("input_tokens", 0),
        "out":  usage.get("output_tokens", 0),
        "cache":usage.get("cache_read_input_tokens", 0),
    })

# Render summary.
def fmt_tools(tools):
    if not tools: return "(no tools)"
    parts = []
    for t in tools:
        bits = [t.get("name", "?")]
        if t.get("preset"): bits.append(f"preset={t['preset']}")
        if t.get("mode"):   bits.append(f"mode={t['mode']}")
        if t.get("replicate"): bits.append(f"replicate={t['replicate']}")
        if t.get("ops"):    bits.append(f"ops={t['ops']}")
        parts.append(" ".join(bits))
    return "; ".join(parts)

with open(f"{out_dir}/summary.md", "w") as f:
    f.write(f"# V5-only smoke — {started}\n\n")
    f.write(f"- V5: {v5_url}\n- Tenant: `{tenant}`\n- Session (single, shared): `{sid}`\n")
    f.write(f"- Started: {started}\n- Finished: {finished}\n")
    f.write(f"- Prompts: {len(rows)} from `prompts.json`\n\n")

    f.write("## Per-prompt narrative\n\n")
    f.write("Format: each turn shows what V5 did (tool calls + preset choice + render result + visible text samples). State accumulates across turns — drill/refine/modify/continuation see prior context.\n\n")

    for r in rows:
        f.write(f"### {r['pid']} `[{r['tag']}]` — «{r['query']}»\n\n")
        if not r["ok"]:
            f.write(f"❌ ERROR: {r.get('err','no response')}\n\n")
            continue
        f.write(f"- **Tools**: {fmt_tools(r['tools'])}\n")
        nc_summary = ", ".join(f"{k}={v}" for k, v in sorted(r["node_counts"].items()))
        f.write(f"- **Render**: {r['card_count']} card(s); nodes — {nc_summary or '(empty)'}\n")
        if r["text_samples"]:
            f.write(f"- **Visible text**: {' / '.join(repr(t) for t in r['text_samples'])}\n")
        f.write(f"- **Latency**: {r['lat']}ms (a1 {r['a1']} + a2 {r['a2']}) · cost ${r['cost']:.4f} · cache_read {r['cache']}\n\n")

    # Aggregate analysis.
    ok_rows = [r for r in rows if r["ok"]]
    f.write("\n## Pattern analysis\n\n")

    by_tag = defaultdict(list)
    for r in ok_rows: by_tag[r["tag"]].append(r)

    f.write("### Per-tag behaviour summary\n\n")
    f.write("| tag | N | preset picks | mode mix | rebuild→cards (avg) | misclass count |\n")
    f.write("|---|---|---|---|---|---|\n")
    for tag, group in sorted(by_tag.items()):
        presets = Counter()
        modes = Counter()
        rebuilds_with_cards = []
        miscls = 0
        for r in group:
            for t in r["tools"]:
                if t.get("preset"): presets[t["preset"]] += 1
                if t.get("mode"):   modes[t["mode"]] += 1
                if t.get("mode") == "rebuild":
                    rebuilds_with_cards.append(r["card_count"])
                # Heuristic misclassification flags:
                # - greeting/conversational tag → empty_not_found preset = wrong
                # - drill/continuation tag → mode=modify with ops=0 = no-op
                if tag in ("greeting", "conversational") and t.get("preset") == "empty_not_found":
                    miscls += 1
                if tag in ("drill", "continuation") and t.get("mode") == "modify" and t.get("ops", 0) == 0:
                    miscls += 1
        avg_cards = (sum(rebuilds_with_cards) / len(rebuilds_with_cards)) if rebuilds_with_cards else 0
        preset_str = ", ".join(f"{k}×{v}" for k, v in presets.most_common(3)) or "—"
        mode_str = ", ".join(f"{k}×{v}" for k, v in modes.most_common(3)) or "—"
        f.write(f"| {tag} | {len(group)} | {preset_str} | {mode_str} | {avg_cards:.1f} | {miscls} |\n")

    # Aggregates.
    if ok_rows:
        lats = [r["lat"] for r in ok_rows if r["lat"] > 0]
        costs = [r["cost"] for r in ok_rows]
        caches = [r["cache"] for r in ok_rows]
        f.write("\n### Numbers\n\n")
        if lats:
            s = sorted(lats)
            p50 = s[len(s)//2]
            p95 = s[int(len(s)*0.95)] if len(s) > 1 else s[0]
            f.write(f"- Latency: p50 {p50}ms, p95 {p95}ms, mean {statistics.mean(lats):.0f}ms\n")
        f.write(f"- Total cost: ${sum(costs):.4f} (avg {statistics.mean(costs):.4f}/turn)\n")
        if caches:
            f.write(f"- Cache_read: avg {statistics.mean(caches):.0f} tokens (range {min(caches)}-{max(caches)})\n")
        f.write(f"- Success: {len(ok_rows)}/{len(rows)}\n")

print(f"\n✓ summary → {out_dir}/summary.md")
print(f"  {len(ok_rows)}/{len(rows)} ok")
PY

rm -f "$PROMPTS_TMP"
echo "done."
