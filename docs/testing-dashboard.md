# Dashboard acceptance fixture

The fixture serves the actual embedded dashboard and Go API over an in-memory
HTTP implementation of Layer's read/write wire. It includes legacy duplicate
sessions, multiple projects/models/tools, prompt timestamps, and old/new grades.
Search scores are authored test data; this checks query scoping, grouping and
navigation, not hosted Layer relevance or embedding quality.

Run the regular suite first:

```sh
go build ./...
go vet ./...
go test -race -count=1 ./...
```

Start the fixture in one terminal:

```sh
HEV_DASHBOARD_FIXTURE_LISTEN=127.0.0.1:18787 \
  go test ./internal/serve -run '^TestDashboardFixtureServer$' -timeout 30m -v
```

In another terminal, compare store-filtered list/stats counts against an
independent `jq` over the unfiltered corpus:

```sh
scripts/check-dashboard-api.sh
```

For browser checks, install Playwright outside the repository. This does not
add an application build step or runtime dependency:

```sh
browser_deps=$(mktemp -d)
npm install --prefix "$browser_deps" playwright
"$browser_deps/node_modules/.bin/playwright" install chromium --only-shell
NODE_PATH="$browser_deps/node_modules" node scripts/test-dashboard.cjs
NODE_PATH="$browser_deps/node_modules" node scripts/test-fac11-perf.cjs
NODE_PATH="$browser_deps/node_modules" node scripts/test-fac11-ui.cjs
```

The browser check verifies Enter-only search, multi-value filters and URL
restoration, snippet-to-turn links, day/model/heatmap click counts, generic
marks and evidence links. It writes `docs/images/dashboard-wordmark.png`.
The API and browser scripts accept `HEV_DASHBOARD_URL` to select a different
fixture address.
The API script expects the fixture vocabulary; do not use it as a production
corpus assertion. Stop the fixture when finished.

The screenshot scripts accept `HEV_SCREENSHOT_DIR` to keep fresh evidence
outside the checkout.
For the actual stopped-server check, build a temporary `hev` binary and pass
its absolute path as `HEV_DASHBOARD_BINARY` to `test-fac11-perf.cjs`. It starts
its own server on an ephemeral port using the configured Layer namespace and
terminates only that child server.

The server negotiates gzip with `Accept-Encoding` and sends
`Vary: Accept-Encoding`. Measure both the plan's plain curl request and
`curl --compressed`; compressed transfer bytes do not establish the plan's
uncompressed `<2,000,000`-byte criterion. Browser timing against a live archive
is separate from the small fixture's paint measurements.

After merge, the operator separately performs the read-side backfill and the
plan's live checks. The grader integration and historical eval import belong
to that follow-on. Fixture tests never contact production; the optional
stopped-server mode reads the configured Layer namespace.
