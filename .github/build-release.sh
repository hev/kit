#!/bin/bash
# Build the release tarballs into dist/.
#
# Kept out of the workflow so it can be run locally before tagging:
#
#   GITHUB_REF_NAME=v0.1.0 ./.github/build-release.sh
#
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

VERSION="${GITHUB_REF_NAME:-}"
[[ -n "$VERSION" ]] || { echo "build-release: set GITHUB_REF_NAME to the tag (v0.1.0)" >&2; exit 1; }
VERSION="${VERSION#v}"

DIST_DIR="${DIST_DIR:-$ROOT_DIR/dist}"
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"
DIST_DIR="$(cd "$DIST_DIR" && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# The version is what picks the dashboard image `hev up` pulls, so a binary
# built without it would pair itself with whatever :latest is that day.
LDFLAGS="-s -w -X github.com/hev/kit/internal/version.Version=${VERSION}"
for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64 windows/amd64; do
    os="${target%/*}"; arch="${target#*/}"
    binary=hev
    [[ "$os" != windows ]] || binary=hev.exe
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
        go build -trimpath -ldflags "$LDFLAGS" -o "$work/$binary" ./cmd/hev
    if [[ "$os" == windows ]]; then
        (cd "$work" && zip -q "$DIST_DIR/kit_${VERSION}_${os}_${arch}.zip" "$binary")
    else
        tar -czf "$DIST_DIR/kit_${VERSION}_${os}_${arch}.tar.gz" -C "$work" "$binary"
    fi
    rm "$work/$binary"
done
if command -v sha256sum >/dev/null; then
    (cd "$DIST_DIR" && sha256sum kit_"${VERSION}"_*.tar.gz kit_"${VERSION}"_*.zip > checksums.txt)
else
    (cd "$DIST_DIR" && shasum -a 256 kit_"${VERSION}"_*.tar.gz kit_"${VERSION}"_*.zip > checksums.txt)
fi
cat "$DIST_DIR/checksums.txt"
