#!/bin/bash
# A clean Linux user with a real systemd user manager (in a container with
# systemd as PID 1, no docker inside): hev up with no key installs hevd and
# the dashboard as systemd user units.
# usage: systemd.sh <hev binary>
set -u
HEV=$1
export DOCKER_HOST=unix:///run/user/$(id -u)/docker.sock
C=kit07-systemd-accept
docker rm -f $C >/dev/null 2>&1
docker run -d --name $C --privileged --cgroupns=host --tmpfs /run --tmpfs /run/lock kit07-systemd >/dev/null
for i in $(seq 1 60); do docker exec $C systemctl is-system-running 2>/dev/null | grep -q -E "running|degraded" && break; sleep 1; done
docker cp "$HEV" $C:/usr/local/bin/hev
docker exec $C chmod 755 /usr/local/bin/hev
U=$(docker exec $C id -u kit)
for i in $(seq 1 30); do docker exec $C test -S /run/user/$U/bus && break; docker exec $C systemctl start user@$U.service >/dev/null 2>&1; sleep 1; done
as() { echo "\$ $*"; docker exec -u kit -e HOME=/home/kit -e XDG_RUNTIME_DIR=/run/user/$U -e DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/$U/bus -e PATH=/home/kit/.local/bin:/usr/local/bin:/usr/bin:/bin -e NO_COLOR=1 -w /home/kit $C bash -c "$*" 2>&1; echo "[exit $?]"; }
SID=5a6b7c8d-1111-4222-8333-944455556666
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
as "command -v docker || echo 'docker: not installed'; id; systemctl --user is-system-running"
as "mkdir -p ~/.claude/projects/-home-kit-app && cat > ~/.claude/projects/-home-kit-app/$SID.jsonl <<'EOF'
{\"type\":\"user\",\"uuid\":\"u1\",\"sessionId\":\"$SID\",\"cwd\":\"/home/kit/app\",\"timestamp\":\"$NOW\",\"message\":{\"role\":\"user\",\"content\":\"the systemd unit for the dashboard keeps restarting\"}}
{\"type\":\"assistant\",\"uuid\":\"a1\",\"parentUuid\":\"u1\",\"sessionId\":\"$SID\",\"cwd\":\"/home/kit/app\",\"timestamp\":\"$NOW\",\"message\":{\"role\":\"assistant\",\"model\":\"claude-sonnet-4-6\",\"content\":[{\"type\":\"text\",\"text\":\"The unit restarts because the port was taken; freeing 8099 fixed it.\"}],\"usage\":{\"input_tokens\":10,\"output_tokens\":20}}}
EOF"
as "hev up"
as "systemctl --user --no-pager status hevd.service hev-serve.service | grep -E '●|Active:|ExecStart|Main PID'"
as "cat ~/.config/systemd/user/hev-serve.service"
as "for i in \$(seq 1 60); do test -f ~/.hev/daemon-status.json && break; sleep 1; done; hev traces ls; hev traces search 'port taken'"
as "curl -s 'http://127.0.0.1:8099/api/search?q=port+8099&window=7d' | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d[\"storage\"][\"mode\"], d[\"storage\"][\"engine\"], [s[\"id\"] for s in d[\"sessions\"]])'"
as "hev up"
as "hev down; systemctl --user is-active hevd.service hev-serve.service; ls ~/.config/systemd/user/"
docker rm -f $C >/dev/null
