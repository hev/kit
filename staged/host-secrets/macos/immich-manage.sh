#!/bin/bash
# Staged replacement for ~/.local/share/lab/immich/manage.sh (kit
# staged/host-secrets). Changes from the live file:
# - `up` returns before resolving any secret when every compose service is
#   already running, so the 15-minute launchd job no longer reads 1Password
#   each time it fires while Immich is healthy.
# - When it does start containers, the extra key is read through kit's secrets
#   cache (`hev secrets read`), falling back to `op read` only without hev.
# The reference below is a placeholder; the README's install step copies the
# live one in.
set -euo pipefail
export PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin"
ROOT="$(cd "$(dirname "$0")" && pwd)"
ANTHROPIC_REF='op://example/claude-api-key/credential'
compose=(docker --context colima-immich compose --project-name lab-immich --env-file "$ROOT/.env" -f "$ROOT/compose.yml")

# all_running succeeds when the VM answers and every service defined in
# compose.yml has a running container. It reads no secret: unset variables in
# compose.yml only produce warnings, which are discarded.
all_running() {
  local want have
  want=$("${compose[@]}" config --services 2>/dev/null | sort) || return 1
  have=$("${compose[@]}" ps --services --status running 2>/dev/null | sort) || return 1
  [[ -n $want && $want == "$have" ]]
}

case "${1:-status}" in
  up)
    # Do not create a replacement /Volumes/EXTERNAL when the disk is absent.
    [[ -d '/Volumes/EXTERNAL/2024 Croatia' && -d /Volumes/EXTERNAL/.immich ]] || {
      echo 'Immich: external drive unavailable; skipping startup' >&2; exit 1;
    }
    if all_running; then
      exit 0
    fi
    # Separate VM: never restart CI's default Colima or change its context.
    colima start immich --activate=false --cpu 2 --memory 6 --disk 20 --root-disk 10 \
      --mount /Volumes/EXTERNAL:w --mount-type virtiofs --vm-type vz
    set -a
    . "$ROOT/edit.env"
    set +a
    . "$HOME/shell/op-env.sh"
    if command -v hev >/dev/null 2>&1 && hev secrets read --help 2>/dev/null | grep -- '--cache-only' >/dev/null; then
      ANTHROPIC_API_KEY="$(hev secrets read --caller immich-manage --label ANTHROPIC_API_KEY --ttl 1h "$ANTHROPIC_REF")"
    else
      ANTHROPIC_API_KEY="$(op read "$ANTHROPIC_REF")"
    fi
    export ANTHROPIC_API_KEY
    [[ -n "$ANTHROPIC_API_KEY" ]] || exit 1
    "${compose[@]}" up -d
    ;;
  status) "${compose[@]}" ps ;;
  logs) "${compose[@]}" logs --tail 100 ;;
  stop) "${compose[@]}" stop ;;
  *) echo 'usage: manage.sh up|status|logs|stop' >&2; exit 2 ;;
esac
