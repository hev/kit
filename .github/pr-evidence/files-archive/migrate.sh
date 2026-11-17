#!/bin/bash
# Migration acceptance: a machine set up by kit main's keyless pgvector lane
# (Layer CE on Postgres in Docker), then upgraded to this branch's hev.
# usage: migrate.sh <old hev> <new hev> <work dir>
set -u
OLD=$1 NEW=$2 W=$3
PROJECT=kit07-migr PORT=18480
# This host runs rootless docker; the test HOME has no docker context, so name
# the daemon's socket (a real user's shell has it configured).
export DOCKER_HOST=unix:///run/user/$(id -u)/docker.sock
docker compose -p $PROJECT down >/dev/null 2>&1   # a previous run's containers (this project only)
docker volume rm ${PROJECT}_postgres ${PROJECT}_gateway-state >/dev/null 2>&1
rm -rf "$W"; mkdir -p "$W/home" "$W/oldbin" "$W/newbin"
cp "$OLD" "$W/oldbin/hev"; cp "$NEW" "$W/newbin/hev"
export HOME="$W/home"
# No user service manager (nothing touches this machine's real units).
envrun() { local bin=$1; shift; env -i HOME="$HOME" USER=accept LANG=C.UTF-8 PATH="$bin:/usr/local/bin:/usr/bin:/bin" NO_COLOR=1 DOCKER_HOST="$DOCKER_HOST" \
  HEV_LOCAL_PROJECT=$PROJECT HEV_LOCAL_PORT=$PORT HEV_LOCAL_SERVE_PORT=18499 "$@" 2>&1; }
run() { local bin=$1; shift; echo "\$ $*"; envrun "$bin" "$@"; echo "[exit $?]"; }

mkfix() { # session id, project dir, text
  mkdir -p "$HOME/.claude/projects/$2"
  NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  cat > "$HOME/.claude/projects/$2/$1.jsonl" <<EOF
{"type":"user","uuid":"u-$1","sessionId":"$1","cwd":"/work/$2","timestamp":"$NOW","message":{"role":"user","content":"$3"}}
{"type":"assistant","uuid":"a-$1","parentUuid":"u-$1","sessionId":"$1","cwd":"/work/$2","timestamp":"$NOW","message":{"role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"Answer: $3"}],"usage":{"input_tokens":10,"output_tokens":20}}}
EOF
}
mkfix 11111111-aaaa-4bbb-8ccc-000000000001 -work-alpha "the alpha migration keeps its postgres rows"
mkfix 22222222-aaaa-4bbb-8ccc-000000000002 -work-beta "the beta transcript will be deleted before the upgrade"
mkdir -p "$HOME/.codex/sessions/2026/11/17"
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
cat > "$HOME/.codex/sessions/2026/11/17/rollout-2026-11-17T10-00-00-0199cccc-dddd-7eee-8fff-000000000003.jsonl" <<EOF
{"timestamp":"$NOW","type":"session_meta","payload":{"id":"0199cccc-dddd-7eee-8fff-000000000003","cli_version":"0.152.1","cwd":"/work/gamma"}}
{"timestamp":"$NOW","type":"event_msg","payload":{"type":"user_message","message":"gamma codex session about the export","turn_id":"t1"}}
{"timestamp":"$NOW","type":"turn_context","payload":{"turn_id":"t1","model":"gpt-5.6","cwd":"/work/gamma"}}
{"timestamp":"$NOW","type":"response_item","payload":{"type":"message","id":"m1","role":"assistant","content":[{"type":"output_text","text":"gamma export answer"}]}}
EOF

echo "== 1. kit main (before this change): hev up with no key runs Layer CE on Postgres in Docker"
run "$W/oldbin" hev up --no-dashboard
echo "\$ hev daemon   (main's hevd, until its first scan is written)"
envrun "$W/oldbin" hev daemon > "$W/old-hevd.log" 2>&1 &
D=$!
for i in $(seq 1 600); do [ -f "$HOME/.hev/daemon-status.json" ] && break; sleep 0.5; done
kill $D; wait $D 2>/dev/null
run "$W/oldbin" sh -c "cat $HOME/.hev/daemon-status.json; echo"
run "$W/oldbin" hev traces ls
run "$W/oldbin" sh -c "python3 -c 'import json; print(len(json.load(open(\"$HOME/.hev/index-state.json\"))[\"units\"]), \"units in index-state.json\")'"
run "$W/oldbin" sh -c "grep -v -i 'salt\|api_key' $HOME/.hev/config.toml"
echo; echo "== the beta transcript ages off disk; the containers are stopped (as after a reboot)"
rm "$HOME/.claude/projects/-work-beta/22222222-aaaa-4bbb-8ccc-000000000002.jsonl"
docker compose -p $PROJECT stop 2>&1 | tail -3
docker ps -a --filter label=com.docker.compose.project=$PROJECT --format '{{.Names}} {{.State}}'
cp -a "$HOME" "$W/home-snapshot"
# main's hevd recorded no units above (its pgvector session write fails with
# 422 on the pinned gateway, so the scan stays migration_pending). Record the
# three transcripts it read in the snapshot's index state, as a working scan
# would have, so scenario 3 has an old archive to account for.
python3 - "$W/home-snapshot/.hev/index-state.json" "$W/home-snapshot" <<'PY'
import json, sys
path, home = sys.argv[1], sys.argv[2]
key = "0" * 64
units = {key + ":" + home.replace("home-snapshot", "home") + p: "sig" for p in (
    "/.claude/projects/-work-alpha/11111111-aaaa-4bbb-8ccc-000000000001.jsonl",
    "/.claude/projects/-work-beta/22222222-aaaa-4bbb-8ccc-000000000002.jsonl",
    "/.codex/sessions/2026/11/17/rollout-2026-11-17T10-00-00-0199cccc-dddd-7eee-8fff-000000000003.jsonl")}
json.dump({"units": units}, open(path, "w"))
PY

echo; echo "== 2. this branch, docker on PATH: hev up exports through the old gateway"
run "$W/newbin" hev up --no-dashboard
run "$W/newbin" hev traces ls
run "$W/newbin" hev traces search "beta transcript deleted"
run "$W/newbin" hev traces show 22222222
run "$W/newbin" hev index
run "$W/newbin" hev traces ls
run "$W/newbin" sh -c "grep -v -i 'salt' $HOME/.hev/config.toml"
docker ps -a --filter label=com.docker.compose.project=$PROJECT --format '{{.Names}} {{.State}}'
docker volume ls --filter label=com.docker.compose.project=$PROJECT --format '{{.Name}}'

echo; echo "== 3. this branch, the same machine state but docker unavailable (no docker on PATH): re-index from disk"
rm -rf "$HOME"; cp -a "$W/home-snapshot" "$HOME"
mkdir -p "$W/nodocker"
for f in /usr/bin/* /usr/local/bin/*; do n=$(basename $f); case $n in docker*) continue;; esac; ln -sf $f "$W/nodocker/$n" 2>/dev/null; done
nd() { echo "\$ $*"; env -i HOME="$HOME" USER=accept LANG=C.UTF-8 PATH="$W/newbin:$W/nodocker" NO_COLOR=1 "$@" 2>&1; echo "[exit $?]"; }
nd sh -c 'command -v docker || echo "docker: not on PATH"'
nd hev up --no-dashboard
nd sh -c "ls $HOME/.hev/index-state.json 2>&1 | sed 's|$HOME|~|'"
nd hev index
nd hev traces ls

echo; echo "== cleanup of this test's own containers and volume"
docker compose -p $PROJECT down -v 2>&1 | tail -2
