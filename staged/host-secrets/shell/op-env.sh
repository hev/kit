#!/usr/bin/env bash
# Resolve secrets from 1Password into the environment. Source this; don't run it.
#
#   . ~/shell/op-env.sh
#
# Staged replacement (kit staged/host-secrets). It reads through kit's local
# secrets cache: every missing variable in one `hev secrets env` request, which
# the host's hev daemon answers from memory within OP_ENV_TTL (default 1h), or
# resolves in one batched `op inject` when no daemon runs. Each real request is
# a line in the host read log (~/.hev/secrets/op-reads.log, labels only).
#
# What changed from the previous version:
# - The exports land in the calling shell. The old batch path piped `op inject`
#   output into a `while read; export` loop; bash runs the last stage of a
#   pipeline in a subshell, so under bash every value was lost and the
#   per-secret `op read` fallback then spent one request per variable.
# - No template file: the template goes to op on stdin; values exist only in
#   this shell's memory and environment.
# - One attempt. On a rate limit it prints the retry time and stops; it never
#   falls back to per-secret reads, which would multiply the requests.
#
# Headless by contract: without OP_SERVICE_ACCOUNT_TOKEN (or the token file)
# op falls back to the desktop app and blocks on Touch ID, so this refuses.
#
# Keep the MAP block in sync with the live file (the README's install steps
# carry the live block over).

if [ -z "${OP_SERVICE_ACCOUNT_TOKEN:-}" ] && [ -r "$HOME/.config/op/token" ]; then
  OP_SERVICE_ACCOUNT_TOKEN="$(cat "$HOME/.config/op/token")"
  export OP_SERVICE_ACCOUNT_TOKEN
fi

_ope_map() {
  cat <<'MAP'
EXAMPLE_API_KEY=op://example/example-api-key/credential
EXAMPLE_CHANNEL_ID=op://example/example-channel/credential
EXAMPLE_ALIAS_CHANNEL_ID=op://example/example-channel/credential
MAP
}

# _ope_export_text exports NAME=value lines held in $1 in the current shell,
# using parameter expansion only: no pipeline, no subshell, no here-document
# (bash may back one with a temporary file).
_ope_export_text() {
  _ope_rest=$1
  _ope_nl='
'
  while [ -n "$_ope_rest" ]; do
    case $_ope_rest in
      *"$_ope_nl"*) _ope_line=${_ope_rest%%"$_ope_nl"*}; _ope_rest=${_ope_rest#*"$_ope_nl"} ;;
      *) _ope_line=$_ope_rest; _ope_rest= ;;
    esac
    case $_ope_line in *=*) ;; *) continue ;; esac
    _ope_k=${_ope_line%%=*}
    _ope_v=${_ope_line#*=}
    [ -n "$_ope_v" ] || continue
    export "$_ope_k=$_ope_v"
  done
  unset _ope_rest _ope_nl _ope_line _ope_k _ope_v
}

# A hev new enough to have `secrets env`. --help makes no request.
_ope_hev_ok() {
  command -v hev >/dev/null 2>&1 || return 1
  hev secrets env --help 2>/dev/null | grep -- '--format' >/dev/null
}

op_env_load() {
  if [ -z "${OP_SERVICE_ACCOUNT_TOKEN:-}" ]; then
    echo "op-env: OP_SERVICE_ACCOUNT_TOKEN unset (expected ~/.config/op/token); refusing to call op, which would block on Touch ID" >&2
    return 0
  fi
  _ope_args=''
  _ope_tpl=''
  # The map holds references only, so a here-document is fine for it.
  while IFS= read -r _ope_l; do
    [ -n "$_ope_l" ] || continue
    _ope_name=${_ope_l%%=*}
    eval "_ope_cur=\${$_ope_name:-}"
    [ -n "$_ope_cur" ] && continue
    _ope_args="$_ope_args --ref $_ope_l --ttl $_ope_name=${OP_ENV_TTL:-1h}"
    _ope_tpl="$_ope_tpl$_ope_name={{ ${_ope_l#*=} }}
"
  done <<OPE_MAP
$(_ope_map)
OPE_MAP
  unset _ope_l _ope_name _ope_cur
  if [ -z "$_ope_args" ]; then unset _ope_args _ope_tpl; return 0; fi
  if _ope_hev_ok; then
    # References hold no spaces or glob characters; word splitting is intended.
    # shellcheck disable=SC2086
    if _ope_out=$(hev secrets env --caller "op-env.sh:${HEV_SECRETS_CALLER:-${0##*/}}" $_ope_args); then
      eval "$_ope_out"
    else
      echo "op-env: secrets not loaded (one attempt; see the message above; do not loop)" >&2
    fi
  elif command -v op >/dev/null 2>&1; then
    # No hev: one batched op inject, template on stdin, exports in this shell.
    if _ope_out=$(printf '%s' "$_ope_tpl" | OP_BIOMETRIC_UNLOCK_ENABLED=false op inject 2>/dev/null) && [ -n "$_ope_out" ]; then
      _ope_export_text "$_ope_out"
    else
      echo "op-env: op inject failed (one attempt; not retrying per secret)" >&2
    fi
  else
    echo "op-env: neither hev nor op found" >&2
  fi
  unset _ope_out _ope_args _ope_tpl
}

op_env_load
