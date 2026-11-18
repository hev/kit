#!/usr/bin/env bash
# End-to-end check of app capture against a local Layer store.
#
# Needs a running local stack (`hev up --store pgvector`, gateway on
# http://127.0.0.1:8080 by default), Go, Node 22+ and npm. It builds hev,
# starts `hev collect` on scratch namespaces, runs the real Vercel AI SDK
# (mock model) for two users through the packed @hevkit/trace, checks the app
# API (search isolation, conversation, story-memory board by query and tag),
# checks the service key never reached the collector's log, and deletes the
# scratch namespaces. Nothing here talks to hosted hev layer.
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
traces_ns="e2e-app-traces-$run"
boards_ns="e2e-app-boards-$run"
collect_pid=

cleanup() {
  [ -n "$collect_pid" ] && kill "$collect_pid" 2>/dev/null && wait "$collect_pid" 2>/dev/null || true
  for ns in "$traces_ns" "$boards_ns"; do
    curl -fsS -X DELETE -H "Authorization: Bearer local" "$endpoint/v2/namespaces/$ns" >/dev/null 2>&1 || true
  done
  rm -rf "$work"
}
trap cleanup EXIT

echo "== build hev"
(cd "$root" && go build -o "$work/hev" ./cmd/hev)

echo "== start hev collect ($endpoint, $traces_ns, $boards_ns)"
HEV_CONFIG="$work/config.toml" HOME="$work/home" \
HEVLAYER_ENDPOINT="$endpoint" HEVLAYER_API_KEY=local HEVLAYER_STORE=pgvector \
HEV_COLLECT_SERVICE_KEY="$key" \
  "$work/hev" collect --listen "127.0.0.1:$port" \
    --traces-namespace "$traces_ns" --boards-namespace "$boards_ns" \
    --envelopes "$work/envelopes" >"$work/collect.log" 2>&1 &
collect_pid=$!
for _ in $(seq 50); do
  curl -fsS "http://127.0.0.1:$port/healthz" >/dev/null 2>&1 && break
  sleep 0.2
done
curl -fsS "http://127.0.0.1:$port/healthz"; echo

echo "== pack @hevkit/trace and install it beside ai and zod"
(cd "$root/packages/trace-js" && npm pack --silent --pack-destination "$work" >/dev/null)
mkdir -p "$work/app"
cp "$root/packages/trace-js/e2e/ai-sdk-e2e.mjs" "$work/app/"
(cd "$work/app" && npm init -y >/dev/null && npm install --silent --no-audit --no-fund "$work"/hevkit-trace-*.tgz ai@6 zod@4)

echo "== run the AI SDK app"
(cd "$work/app" && HEV_COLLECT_URL="http://127.0.0.1:$port" HEV_COLLECT_SERVICE_KEY="$key" E2E_RUN="$run" node ai-sdk-e2e.mjs)

echo "== envelopes"
find "$work/envelopes" -type f | sed "s|$work/||"
env_file=$(find "$work/envelopes" -name envelopes.jsonl | head -1)
echo "spans written: $(cat "$work"/envelopes/*/*/envelopes.jsonl | wc -l)"
head -1 "$env_file" | python3 -c 'import json,sys; e=json.load(sys.stdin); print(json.dumps({k:e[k] for k in ("ingest_source","harness","session_id","turn_id","event_kind","redaction_policy","resource")}, indent=1))'

echo "== collector log"
cat "$work/collect.log"
if grep -qF "$key" "$work/collect.log" "$work"/envelopes/*/*/envelopes.jsonl; then
  echo "FAIL: the service key reached the log or an envelope" >&2
  exit 1
fi
echo "service key absent from log and envelopes"
echo "E2E PASS"
