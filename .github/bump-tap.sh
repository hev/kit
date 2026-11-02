#!/bin/bash
# Generate Formula/hev.rb and the kit compatibility formula for a release.
#
#   GITHUB_REF_NAME=v0.1.0 ./.github/bump-tap.sh --dry-run   # render, push nothing
#
# The formula is written through the GitHub Contents API rather than by cloning
# and pushing, so the token is never embedded in a git remote and never reaches
# a log line. HOMEBREW_TAP_TOKEN needs exactly one permission on exactly one
# repo: Contents, read and write, on hev/homebrew-tap.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TAP="${KIT_TAP_REPO:-hev/homebrew-tap}"
OUTPUT_DIR="${FORMULA_OUTPUT_DIR:-$ROOT_DIR/dist/formula}"
DRY_RUN=""
[[ "${1:-}" == "--dry-run" ]] && DRY_RUN=1

VERSION="${GITHUB_REF_NAME:-}"
[[ -n "$VERSION" ]] || { echo "bump-tap: set GITHUB_REF_NAME to the tag (v0.1.0)" >&2; exit 1; }
VERSION="${VERSION#v}"

CHECKSUMS="${DIST_DIR:-$ROOT_DIR/dist}/checksums.txt"
[[ -f "$CHECKSUMS" ]] || { echo "bump-tap: no $CHECKSUMS — run build-release.sh first" >&2; exit 1; }

# Pull each digest out by the filename it belongs to. Reading them positionally
# would silently swap the two architectures the day the build loop reorders,
# and the symptom is a formula that installs an Intel binary on Apple silicon.
mkdir -p "$OUTPUT_DIR"
for formula in hev kit; do
    rendered="$(cat "$ROOT_DIR/.github/$formula.rb.tmpl")"
    rendered="${rendered//@@VERSION@@/$VERSION}"
    for os in darwin linux; do
        for arch in arm64 amd64; do
            sha="$(awk -v want="kit_${VERSION}_${os}_${arch}.tar.gz" '{ n=$2; sub(/^\.\//, "", n); if (n == want) print $1 }' "$CHECKSUMS")"
            [[ -n "$sha" ]] || { echo "missing checksum: $os/$arch" >&2; exit 1; }
            key="@@SHA_$(echo "$os" | tr '[:lower:]' '[:upper:]')_$(echo "$arch" | tr '[:lower:]' '[:upper:]')@@"
            rendered="${rendered//$key/$sha}"
        done
    done
    if grep -q '@@' <<<"$rendered"; then
        echo "bump-tap: unsubstituted placeholder" >&2; exit 1
    fi
    printf '%s\n' "$rendered" > "$OUTPUT_DIR/$formula.rb"
done
if [[ -n "$DRY_RUN" ]]; then
    cat "$OUTPUT_DIR/hev.rb" "$OUTPUT_DIR/kit.rb"
    echo "bump-tap: dry run — $TAP not touched" >&2
    exit 0
fi
for formula in hev kit; do
    FORMULA_PATH="Formula/$formula.rb"
    RENDERED="$(cat "$OUTPUT_DIR/$formula.rb")"

    # The Contents API updates a file in place, but only if it is told which blob
    # it is replacing. Absent means the tap has no formula yet, which is the first
    # release and not an error.
    EXISTING="$(gh api "repos/$TAP/contents/$FORMULA_PATH" --jq .sha 2>/dev/null || true)"

    args=(--method PUT "repos/$TAP/contents/$FORMULA_PATH"
          -f "message=kit ${VERSION}"
          -f "content=$(printf '%s\n' "$RENDERED" | base64 | tr -d '\n')")
    [[ -n "$EXISTING" ]] && args+=(-f "sha=$EXISTING")

    gh api "${args[@]}" --jq '.commit.html_url'
    echo "bump-tap: $TAP now serves $formula ${VERSION}" >&2

done
