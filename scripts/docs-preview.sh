#!/bin/bash
# The release train's docs preview: hevkit.com's site built from a git ref and
# served on the private network. It runs on the mini; see
# docs/runbooks/release-train.md "The docs preview".
#
#   scripts/docs-preview.sh build [REF]   # build site/ at REF (default origin/main) and make it current
#   scripts/docs-preview.sh refresh       # fetch origin; build origin/main only if it moved
#   scripts/docs-preview.sh serve         # serve the current build (foreground)
#   scripts/docs-preview.sh status        # what is current
#
# Builds land in $KIT_PREVIEW_DIR/builds/<sha>; "current" is a symlink swapped
# only after a build succeeds, so the server never shows a half-built site and
# a broken commit leaves the last good build up. The server reads through the
# symlink on every request: refresh needs no restart.
#
# Settings (environment):
#   KIT_PREVIEW_DIR   state directory      (default ~/.cache/kit-docs-preview)
#   KIT_PREVIEW_HOST  address to bind      (default 127.0.0.1; on the mini, its tailnet address)
#   KIT_PREVIEW_PORT  port                 (default 4342; `pnpm dev` in site/ uses 4341)
#   KIT_PREVIEW_REF   ref refresh follows  (default origin/main; origin/release/vX.Y for a patch train)
#   KIT_PREVIEW_REPO  kit checkout to read (default: the checkout holding this script)
set -euo pipefail

REPO="${KIT_PREVIEW_REPO:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
DIR="${KIT_PREVIEW_DIR:-$HOME/.cache/kit-docs-preview}"
HOST="${KIT_PREVIEW_HOST:-127.0.0.1}"
PORT="${KIT_PREVIEW_PORT:-4342}"
FOLLOW="${KIT_PREVIEW_REF:-origin/main}"
KEEP=3

log() { printf 'docs-preview: %s\n' "$*" >&2; }
die() { log "$*"; exit 1; }

current_sha() {
    [[ -f "$DIR/current/preview.json" ]] || return 0
    python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["sha"])' "$DIR/current/preview.json"
}

swap_current() {
    # ln -sfn is not atomic; os.replace over a temporary link is, on macOS and Linux.
    ln -sfn "builds/$1" "$DIR/current.tmp"
    python3 -c 'import os,sys; os.replace(sys.argv[1], sys.argv[2])' "$DIR/current.tmp" "$DIR/current"
}

prune() {
    local keep cur
    cur="$(current_sha)"
    # Newest first; keep the current build and the KEEP newest.
    # shellcheck disable=SC2010 # build names are commit SHAs
    keep="$(ls -1t "$DIR/builds" 2>/dev/null | grep -v '\.tmp$' | head -n "$KEEP" || true)"
    for b in "$DIR"/builds/*; do
        [[ -e "$b" ]] || continue
        name="$(basename "$b")"
        [[ "$name" == "$cur" ]] && continue
        grep -qx "$name" <<<"$keep" && continue
        rm -rf "$b"
    done
    rm -rf "$DIR/src"
}

build() {
    local ref="${1:-$FOLLOW}" sha src out
    sha="$(git -C "$REPO" rev-parse --verify "$ref^{commit}")" || die "unknown ref $ref"
    git -C "$REPO" cat-file -e "$sha:site/package.json" 2>/dev/null || die "$ref ($sha) has no site/"
    mkdir -p "$DIR/builds"
    # One build at a time; a refresh that finds a build running just skips.
    if ! mkdir "$DIR/lock" 2>/dev/null; then
        log "another build holds $DIR/lock; skipping"
        return 0
    fi
    # shellcheck disable=SC2064 # expand now: $sha is local to this function
    trap "rm -rf '$DIR/lock' '$DIR/src/$sha' '$DIR/builds/$sha.tmp'" EXIT
    out="$DIR/builds/$sha"
    if [[ ! -f "$out/preview.json" ]]; then
        src="$DIR/src/$sha"
        rm -rf "$src" "$out.tmp"
        mkdir -p "$src"
        git -C "$REPO" archive "$sha" site | tar -x -C "$src"
        log "building site/ at ${sha:0:7} ($ref)"
        (cd "$src/site" && pnpm install --frozen-lockfile --silent && pnpm build) >&2
        python3 - "$src/site/dist/preview.json" "$sha" "$ref" <<'PY'
import datetime, json, sys
path, sha, ref = sys.argv[1:]
json.dump({"schema": "kit.docs-preview/v1", "sha": sha, "ref": ref,
           "built_at": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")},
          open(path, "w"), indent=2)
PY
        mv "$src/site/dist" "$out.tmp"
        mv "$out.tmp" "$out"
    fi
    swap_current "$sha"
    prune
    log "current is ${sha:0:7} ($ref)"
}

refresh() {
    git -C "$REPO" fetch --quiet origin
    local want
    want="$(git -C "$REPO" rev-parse --verify "$FOLLOW^{commit}")"
    if [[ "$want" == "$(current_sha)" ]]; then
        log "up to date at ${want:0:7} ($FOLLOW)"
        return 0
    fi
    build "$FOLLOW"
}

serve() {
    [[ -L "$DIR/current" ]] || die "nothing built yet; run: $0 build"
    log "serving $DIR/current on http://$HOST:$PORT/docs"
    exec python3 -m http.server --bind "$HOST" "$PORT" --directory "$DIR/current"
}

status() {
    if [[ -f "$DIR/current/preview.json" ]]; then
        cat "$DIR/current/preview.json"
    else
        echo "nothing built yet"
    fi
}

case "${1:-}" in
    build) shift; build "$@" ;;
    refresh) refresh ;;
    serve) serve ;;
    status) status ;;
    *) sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
