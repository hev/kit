#!/bin/bash
# Acceptance: hev up with no key on a clean HOME, no docker on PATH.
# usage: accept.sh <hev binary> <sysbin dir (with or without rg)> <work dir>
set -u
HEV=$1 SYSBIN=$2 W=$3
rm -rf "$W"; mkdir -p "$W/home" "$W/bin"
cp "$HEV" "$W/bin/hev"
export HOME="$W/home"
# A clean environment: no hev layer key, no user service manager (so nothing
# touches this machine's real systemd units), no docker on PATH.
run() { echo "\$ $*"; env -i HOME="$HOME" USER=accept LANG=C.UTF-8 PATH="$W/bin:$SYSBIN" NO_COLOR=1 "$@" 2>&1; echo "[exit $?]"; }
echo "== environment"
run sh -c 'command -v docker || echo "docker: not on PATH"; command -v rg || echo "rg: not on PATH"; env | grep -c HEVLAYER || true'

SID=7d3f2a10-4b5c-4e6f-8a9b-0c1d2e3f4a5b
mkdir -p "$HOME/.claude/projects/-work-demo"
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
cat > "$HOME/.claude/projects/-work-demo/$SID.jsonl" <<EOF
{"type":"user","uuid":"u1","sessionId":"$SID","cwd":"/work/demo","timestamp":"$NOW","message":{"role":"user","content":"why does the preflight fail with HEVLAYER_API_KEY 401? my key is sk-ant-api03-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}}
{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"$SID","cwd":"/work/demo","timestamp":"$NOW","message":{"role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"The preflight failed because the gateway rejected a stale key; exporting a fresh one fixed it."}],"usage":{"input_tokens":10,"output_tokens":20}}}
EOF
mkdir -p "$HOME/.codex/sessions/2026/11/17"
CODEX="$HOME/.codex/sessions/2026/11/17/rollout-2026-11-17T10-00-00-0199aaaa-bbbb-7ccc-8ddd-eeeeffff0000.jsonl"
cat > "$CODEX" <<EOF
{"timestamp":"$NOW","type":"session_meta","payload":{"id":"0199aaaa-bbbb-7ccc-8ddd-eeeeffff0000","cli_version":"0.152.1","cwd":"/work/deploy","git":{"branch":"main"}}}
{"timestamp":"$NOW","type":"event_msg","payload":{"type":"user_message","message":"the deploy is flaky on friday, find out why","turn_id":"turn-1"}}
{"timestamp":"$NOW","type":"turn_context","payload":{"turn_id":"turn-1","model":"gpt-5.6","cwd":"/work/deploy"}}
{"timestamp":"$NOW","type":"response_item","payload":{"type":"message","id":"msg-1","role":"assistant","content":[{"type":"output_text","text":"The flaky deploy comes from a helm rollout timeout; raising the deadline fixed the retries."}]}}
{"timestamp":"$NOW","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"output_tokens":20}}}}
EOF

echo; echo "== hev up (no key)"
run hev up
echo; echo "== capture: hevd's first scan (no service manager here, so hevd runs in the foreground as up says)"
env -i HOME="$HOME" USER=accept LANG=C.UTF-8 PATH="$W/bin:$SYSBIN" hev daemon > "$W/hevd.log" 2>&1 &
D=$!
for i in $(seq 1 300); do [ -f "$HOME/.hev/daemon-status.json" ] && break; sleep 0.2; done
kill $D; wait $D 2>/dev/null
run sh -c "cat $HOME/.hev/daemon-status.json | head -c 400; echo"
run sh -c "find $HOME/.hev/archive -type f -not -path '*/blobs/*' | sed 's|$HOME|~|' | sort"
run sh -c "grep -rl 'sk-ant-api03-AAAA' $HOME/.hev/archive || echo 'secret: not in the archive'"
echo; echo "== traces"
run hev traces ls
run hev traces show 7d3f2a10
run hev traces search "preflight 401"
run hev traces search "rollout timeout" --harness codex
run hev traces search "nothing matches these words" --also "flaky deploy" --also "stale key"
run hev traces search --regex '(?i)rejected an? \w+ key'
run sh -c 'hev traces search --json "helm deadline" | head -c 900; echo'
echo; echo "== hev serve /api/search"
env -i HOME="$HOME" USER=accept LANG=C.UTF-8 PATH="$W/bin:$SYSBIN" hev serve --port 18431 > "$W/serve.log" 2>&1 &
S=$!
for i in $(seq 1 50); do curl -s -o /dev/null http://127.0.0.1:18431/ && break; sleep 0.2; done
run sh -c "curl -s 'http://127.0.0.1:18431/api/search?q=flaky+deploy&window=7d' | python3 -c 'import json,sys; d=json.load(sys.stdin); print(json.dumps({\"storage\":d[\"storage\"],\"sessions\":[{k:s.get(k) for k in (\"id\",\"harness\",\"match_source\",\"snippet\") if k in s} for s in d[\"sessions\"]]}, indent=1))'"
kill $S; wait $S 2>/dev/null
echo; echo "== redaction delete: the Claude transcript is gone, and a new redaction salt makes the archive redaction upgrade run again"
rm "$HOME/.claude/projects/-work-demo/$SID.jsonl"
run sh -c "grep -n redact_salt $HOME/.hev/config.toml | sed 's/=.*/= <salt>/'"
sed -i 's/^redact_salt = .*/redact_salt = "3feda16a3e3387a5f4e14eaa5b1b3f09056a165ea15a40d21ef26ab49e5482ad"/' "$HOME/.hev/config.toml"
run hev index  # refuses: the upgrade would remove a session whose transcript is gone
run hev index --allow-source-loss
run sh -c "find $HOME/.hev/archive -path '*$SID*' | sed 's|$HOME|~|'; echo \"files left for $SID: \$(find $HOME/.hev/archive -path '*$SID*' | wc -l)\"; grep -c $SID $HOME/.hev/archive/hev-traces/index.jsonl || true"
run hev traces ls
run hev down
