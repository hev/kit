#!/bin/bash
# Pin site/'s docs to a published release before a production build:
#
#   site/scripts/pin-docs-release.sh v0.7.0 && (cd site && pnpm build)
#
# Production hevkit.com documents the latest published release. Features merge
# into the train branch (vX.Y), and planned features' docs live on the train's
# docs branch, which the preview on the mini follows; neither may appear on
# hevkit.com until Adam publishes the release that ships it. So the production
# build takes site/src/content/docs from the release tag and everything else
# (layout, home page, styles) from the checkout, which site.yml makes the same
# tag. It rewrites the checkout's
# site/src/content/docs: run it in CI or a scratch checkout, not your working
# tree. It also writes site/public/docs-release.json, which the release verify
# reads to see the release's docs are live.
set -euo pipefail

tag="${1:-}"
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "usage: $0 vX.Y.Z" >&2; exit 2; }
root="$(git rev-parse --show-toplevel)"
cd "$root"
git rev-parse --verify -q "refs/tags/$tag" >/dev/null || git fetch --quiet origin "refs/tags/$tag:refs/tags/$tag"
# Resolve the whole input before touching the checkout; no fallback to main.
if ! git cat-file -e "$tag:site/src/content/docs" 2>/dev/null; then
    echo "pin-docs-release: $tag has no site/src/content/docs (released before the site moved onto main)." >&2
    echo "Production stays on its last hand deploy until a release that carries site/ is published." >&2
    exit 1
fi
rm -rf site/src/content/docs
git restore --source="$tag" --worktree -- site/src/content/docs
printf '{"schema":"kit.docs-release/v1","tag":"%s"}\n' "$tag" > site/public/docs-release.json
echo "pin-docs-release: site/src/content/docs is $tag's"
