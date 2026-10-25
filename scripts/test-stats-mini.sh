#!/usr/bin/env bash
# Run only on Mini, from a clean checkout of the candidate being validated.
set -euo pipefail
if [[ "$(uname -s)" != Darwin ]]; then
  echo 'Mini/macOS only: this procedure must not launch a browser on Oven.' >&2
  exit 2
fi
cd "$(dirname "$0")/.."
if [[ -n "$(git status --porcelain=v1)" ]]; then
  echo 'Use a clean candidate checkout.' >&2
  exit 2
fi
: "${HEV_STATS_EXPECTED_CANDIDATE:?Set this to the exact report candidate SHA}"
if [[ "$(git rev-parse HEAD)" != "$HEV_STATS_EXPECTED_CANDIDATE" ]]; then
  echo 'Candidate mismatch.' >&2
  exit 2
fi
artifact_dir="$(mktemp -d "${TMPDIR:-/tmp}/kit-stats-mini.XXXXXX")"
fixture_pid=''
cleanup() {
  if [[ -n "$fixture_pid" ]]; then
    kill "$fixture_pid" 2>/dev/null || true
    wait "$fixture_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
git rev-parse HEAD > "$artifact_dir/candidate.txt"
go test -c -o "$artifact_dir/serve.test" ./internal/serve
# The test owns a synthetic localhost Layer fixture. Never run hev serve or
# point this procedure at a configured archive. A random local port avoids
# accidentally validating another process on a fixed port.
HEV_DASHBOARD_FIXTURE_LISTEN=127.0.0.1:0 "$artifact_dir/serve.test" -test.run '^TestDashboardFixtureServer$' -test.v -test.timeout 5m > "$artifact_dir/fixture.log" 2>&1 &
fixture_pid=$!
fixture_url=''
for ((attempt=0; attempt<100; attempt++)); do
  if ! kill -0 "$fixture_pid" 2>/dev/null; then cat "$artifact_dir/fixture.log"; exit 1; fi
  fixture_url="$(sed -n 's/.*fixture URL: \(http:\/\/127\.0\.0\.1:[0-9]*\).*/\1/p' "$artifact_dir/fixture.log" | head -1)"
  if [[ -n "$fixture_url" ]] && curl --fail --silent "$fixture_url/" -o /dev/null; then break; fi
  sleep 0.1
done
if [[ -z "$fixture_url" ]]; then echo 'Fixture did not become ready.' >&2; exit 1; fi
HEV_DASHBOARD_URL="$fixture_url" HEV_STATS_SCREENSHOT="$artifact_dir/stats.png" node scripts/test-stats-browser.cjs 2>&1 | tee "$artifact_dir/browser.log"
echo "Synthetic Mini artifacts: $artifact_dir"
echo 'Automated interaction evidence only; inspect layout and values separately for visual acceptance.'
