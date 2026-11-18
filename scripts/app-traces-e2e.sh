#!/usr/bin/env bash
# End-to-end check of app capture against a local Layer store.
#
# Needs a running local stack (`hev up --store pgvector`, gateway on
# http://127.0.0.1:8080 by default), Go, Node 22+ and npm. It builds hev,
# starts `hev collect` on scratch namespaces allowing two app namespaces, runs
# the real Vercel AI SDK (mock model) through the packed @hevmind/trace for two
# users in namespace A, the same user in namespace B, and one turn in a
# namespace the collector does not allow; checks the app API (user and
# namespace isolation, refusals, conversation, story-memory board by query and
# tag), checks that refused spans were written and indexed nowhere and that
# the service key never reached the collector's log, and deletes the scratch
# namespaces. Nothing here talks to hosted hev layer.
#
#   scripts/app-traces-e2e.sh
#   HEVLAYER_ENDPOINT=http://127.0.0.1:8080 scripts/app-traces-e2e.sh
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
endpoint=${HEVLAYER_ENDPOINT:-http://127.0.0.1:8080}
case "$endpoint" in
  http://127.0.0.1:*|http://localhost:*) ;;
  *) echo "refusing a non-local Layer endpoint: $endpoint" >&2; exit 2 ;;
esac
port=${E2E_PORT:-14319}
run=$(date +%s)
work=$(mktemp -d)
key="e2e-$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')"
traces_prefix="e2e-app-traces-$run"
boards_ns="e2e-app-boards-$run"
ns_a="story-e2e-a"
ns_b="story-e2e-b"
ns_rogue="story-e2e-rogue"
traces_a="$traces_prefix-$ns_a"
traces_b="$traces_prefix-$ns_b"
traces_rogue="$traces_prefix-$ns_rogue"
collect_pid=

cleanup() {
  [ -n "$collect_pid" ] && kill "$collect_pid" 2>/dev/null && wait "$collect_pid" 2>/dev/null || true
  for ns in "$traces_a" "$traces_b" "$traces_rogue" "$boards_ns"; do
    curl -fsS -X DELETE -H "Authorization: Bearer local" "$endpoint/v2/namespaces/$ns" >/dev/null 2>&1 || true
  done
  rm -rf "$work"
}
trap cleanup EXIT

echo "== build hev"
(cd "$root" && go build -o "$work/hev" ./cmd/hev)

echo "== hev collect refuses to start without namespaces"
if out=$(HEV_CONFIG="$work/config.toml" HOME="$work/home" HEV_COLLECT_SERVICE_KEY="$key" HEV_COLLECT_NAMESPACES= \
    "$work/hev" collect --listen "127.0.0.1:$port" 2>&1); then
  echo "FAIL: hev collect started without HEV_COLLECT_NAMESPACES" >&2; exit 1
fi
echo "$out" | tail -1
case "$out" in *"$key"*) echo "FAIL: the refusal printed the key" >&2; exit 1 ;; esac

echo "== start hev collect ($endpoint, namespaces $ns_a,$ns_b -> $traces_a, $traces_b; boards $boards_ns)"
HEV_CONFIG="$work/config.toml" HOME="$work/home" \
HEVLAYER_ENDPOINT="$endpoint" HEVLAYER_API_KEY=local HEVLAYER_STORE=pgvector \
HEV_COLLECT_SERVICE_KEY="$key" HEV_COLLECT_NAMESPACES="$ns_a,$ns_b" \
  "$work/hev" collect --listen "127.0.0.1:$port" \
    --traces-namespace "$traces_prefix" --boards-namespace "$boards_ns" \
    --envelopes "$work/envelopes" >"$work/collect.log" 2>&1 &
collect_pid=$!
for _ in $(seq 50); do
  curl -fsS "http://127.0.0.1:$port/healthz" >/dev/null 2>&1 && break
  sleep 0.2
done
curl -fsS "http://127.0.0.1:$port/healthz"; echo

echo "== pack @hevmind/trace and install it beside ai and zod"
(cd "$root/packages/trace-js" && npm pack --silent --pack-destination "$work" >/dev/null)
mkdir -p "$work/app"
cp "$root/packages/trace-js/e2e/ai-sdk-e2e.mjs" "$work/app/"
(cd "$work/app" && npm init -y >/dev/null && npm install --silent --no-audit --no-fund "$work"/hevmind-trace-*.tgz ai@6 zod@4)

echo "== run the AI SDK app"
(cd "$work/app" && HEV_COLLECT_URL="http://127.0.0.1:$port" HEV_COLLECT_SERVICE_KEY="$key" E2E_RUN="$run" \
  E2E_NS_A="$ns_a" E2E_NS_B="$ns_b" E2E_NS_ROGUE="$ns_rogue" node ai-sdk-e2e.mjs)

echo "== envelopes"
find "$work/envelopes" -type f | sed "s|$work/||"
for ns in "$ns_a" "$ns_b"; do
  echo "$ns spans written: $(cat "$work/envelopes/ns=$ns"/*/*/envelopes.jsonl | wc -l)"
done
if [ -e "$work/envelopes/ns=$ns_rogue" ] || find "$work/envelopes" -mindepth 1 -maxdepth 1 ! -name "ns=$ns_a" ! -name "ns=$ns_b" | grep -q .; then
  echo "FAIL: envelopes written outside the allowed namespaces" >&2; exit 1
fi
if grep -q "rogue" "$work"/envelopes/*/*/*/envelopes.jsonl; then
  echo "FAIL: a refused span reached an envelope" >&2; exit 1
fi
echo "refused spans: no envelopes"
for ns in "$ns_a" "$ns_b"; do
  head -1 "$(find "$work/envelopes/ns=$ns" -name envelopes.jsonl | head -1)" | python3 -c 'import json,sys; e=json.load(sys.stdin); print(json.dumps({k:e[k] for k in ("ingest_source","harness","session_id","turn_id","event_kind","redaction_policy","resource")}, indent=1))'
done

echo "== Layer namespaces"
for ns in "$traces_a" "$traces_b" "$traces_rogue"; do
  rows=$(curl -sS -X POST -H "Authorization: Bearer local" -H "Content-Type: application/json" \
    -d '{"rank_by":["ts","asc"],"top_k":100,"include_attributes":["turn_id","namespace"]}' \
    "$endpoint/v2/namespaces/$ns/query" | python3 -c 'import json,sys
try: d=json.load(sys.stdin)
except Exception: d={}
print(" ".join(sorted(r.get("namespace","?")+"/"+r.get("turn_id","?") for r in d.get("rows") or [])) or "(none)")')
  echo "$ns: $rows"
  case "$ns:$rows" in
    "$traces_rogue:(none)") ;;
    "$traces_rogue:"*) echo "FAIL: the refused namespace reached Layer" >&2; exit 1 ;;
    "$traces_a:"*"rogue"*|"$traces_a:"*"staging"*|"$traces_b:"*"rogue"*) echo "FAIL: turns crossed namespaces" >&2; exit 1 ;;
  esac
done

echo "== collector log"
cat "$work/collect.log"
if [ "$(grep -c 'refused spans' "$work/collect.log")" -ne 2 ]; then
  echo "FAIL: want one refusal line each for $ns_rogue and the missing namespace" >&2; exit 1
fi
if grep -qF "$key" "$work/collect.log" "$work"/envelopes/*/*/*/envelopes.jsonl; then
  echo "FAIL: the service key reached the log or an envelope" >&2
  exit 1
fi
echo "service key absent from log and envelopes"
echo "E2E PASS"
