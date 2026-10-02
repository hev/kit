# Explicit phrase search acceptance

Public API: `GET /api/search?q=...&mode=phrase`, shared by local and hosted
`pkg/dashboard.NewHosted`. Default/explicit `hybrid` retains ANN + BM25/RRF.
The concrete contract is in [hosted-dashboard.md](../../hosted-dashboard.md#explicit-phrase-search).

Verified on the mac mini, 2026-10-02:

- `go test -race -count=1 -skip TestAgainstRealTranscripts ./...` passes.
  The skipped test scans the machine's unrelated real transcript archive;
  CI has no such archive. Build, vet, tidy (no module diff), gofmt and diff
  whitespace checks pass.
- `TestPhraseSearchBeforeDedupAndLimit` uses the actual server/client JSON
  path with an HTTP store fixture that evaluates filters and ordered scans.
  An own-session irrelevant ANN hit wins default hybrid; the lower contiguous
  exact hit wins phrase with `top=1`. Reversed/noncontiguous and foreign-session
  rows are absent. Project/model/host/harness/grade/time filters retain behavior.
- `TestPhraseEvalTextAndInvalidMode` covers actual evaluator `text`, latest eval
  selection, reversed/noncontiguous text and HTTP 400 for invalid modes.
- `TestPhraseSearchReportsIncompleteScan` proves matches beyond the 10,000-row
  budget are omitted with explicit truncation rather than an exhaustive claim.
- `TestTenantsAcrossDashboardRoutes` verifies hosted request credentials and
  namespace isolation sequentially and concurrently with identical session IDs.
  Tenant A's own semantic noise survives hybrid and disappears in phrase;
  tenant B's exact phrase remains. No empty-result mock substitutes for ANN.
- `TestPhraseWirePaginationBudgetAndFilters` exercises the real Client over
  HTTP for both supported capability tables, validating ID rank, page budget,
  identity and filter preservation. Capability refusals and upstream failures
  never fall back to ANN. Approximate/undeclared scans are refused.
- `HEV_PHRASE_LIVE=1 go test ./internal/layer -run '^TestPhraseLiveOrderedScan$'
  -count=1 -v` passes against the configured real gateway/Turbopuffer store.
  Two job-owned synthetic namespaces use plain string text without FTS or
  embedding. Actual scans cross 1,000 rows, preserve session/eval filters and
  test overflow boundaries. Both namespaces are deleted by test cleanup.
  See [real-wire.txt](real-wire.txt). This does not claim a live pgvector probe.
- Headless Chromium `scripts/test-phrase-search.cjs` passes: explicit mode is
  sent to the API and survives filters, timeframe, reload, copied URL and
  browser history; the UI displays the incomplete-result notice.

Reproduce the browser check with a job-owned local fixture port:

```sh
HEV_DASHBOARD_FIXTURE_LISTEN=127.0.0.1:19723 go test ./internal/serve \
  -run '^TestDashboardFixtureServer$' -count=1 -timeout=5m
# In another shell, with Playwright installed outside the repository:
NODE_PATH=/path/to/playwright/node_modules \
  HEV_DASHBOARD_URL=http://127.0.0.1:19723 node scripts/test-phrase-search.cjs
```

The real-wire test reads existing configured credentials internally, creates
only uniquely named `scratch-kit-phrase-*` namespaces and deletes only those
namespaces. It does not inspect or mutate tester/operator fixtures.

Private wrapper pin/rebuild and live phrase A0/Bpositive acceptance on unchanged
fixtures remain with 9g9ghn. Their retained hybrid own hits are separate evidence.
No release or deployment is part of this public change.
