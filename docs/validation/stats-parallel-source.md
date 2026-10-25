# Stats parallel projected source reads

`/api/stats` and the paired sessions/Stats route now acquire their complete
session revision set through Layer `POST /v2/namespaces/{base}-sessions/scans/rows`.
The request uses the existing schema-intersected dashboard projection, including
prompt timestamps and summary only when needed. It carries finite global limits
of 1,000,000 source rows and 268,435,456 decoded source response bytes and observed
consistency. Reaching a limit fails the request; it never truncates Stats.

Layer uses its shared namespace fanout and resolver (ordinary default 8), with
one complete ordered cursor per physical shard and a disjoint residual cursor
for missing/null/out-of-range tags. The residual protects complete acquisition
when cached topology is stale. Unsharded/unready sources retain one complete
cursor. Physical IDs are validated and sorted globally; the namespace marker
is excluded without a string comparison on uint IDs. The source adapter never
widens a failed projection to all attributes. Source errors, authorization,
throttles, budgets and cancellation reject the whole result.

Kit still performs latest selection before mutable predicates, unfiltered facets,
all existing Stats folds, prompt coverage/timezone mapping, project bins and
latest-eval joins with the existing authorized eval reader. It neither changes
eligibility nor approximates or drops outputs. The same request-owned projected
session result serves paired list/Stats and facets. There is no speculative full
scan and no client-side split of dependent cursors.

Only structured HTTP 422 `UnsupportedByStore` with feature
`projected_source_rows` permits the existing sequential projected compatibility
reader. A missing route, auth failure, throttle, malformed response or source
failure is an error. Deploy the new gateway route before consumers. Older
unupgraded gateways cannot be inferred safe from a 404. This branch does not
merge, deploy or release either component.

This is parallel projected source reading, not backend-native aggregation or
compact source reduction. Every projected revision still crosses the source and
consumer transports. Source rows/pages/decoded bytes do not measure provider
physical reads or billing. Observed pages are not a snapshot. Kit wire timing
counts the single source response's rows and HTTP call; gateway source counters
retain the provider pages/bytes. No operational detail is added to product UI.

Run `go test -race -count=1 ./...` and `node scripts/test-stats-path.cjs`.
`TestStatsSelectsProjectedSourceAndPreservesCompleteOutput` compares every output
field of `/api/stats` and paired list/Stats over filters, windows, facets and
grades and asserts exactly one projected acquisition per request. The opt-in
`TestStatsRustProjectedSource` runs through Layer's maintained
`projected_source_kit_http_bridge`: actual Kit HTTP handler -> actual Rust gateway
HTTP route -> fixture HTTP source, with the same rich revision/eval data as its
sequential reference. Set `KIT_STATS_CHECKOUT` explicitly when running that
ignored Layer test. Separate Kit and Rust tests cover downstream cancellation,
budget failures, global query admission and errors without partial results.
