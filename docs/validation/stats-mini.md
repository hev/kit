# Synthetic Stats validation on Mini

Use a clean, owned checkout at the exact corrective report candidate SHA on
Mini. This is a prepared procedure; creating it does not establish browser or
visual acceptance. Do not run browsers on Oven.

With Go, Node, Playwright and its Chromium already available on Mini:

```sh
export HEV_STATS_EXPECTED_CANDIDATE='<exact candidate from corrective report>'
# If Playwright is outside local module resolution:
export NODE_PATH='<existing Playwright node_modules directory>'
./scripts/test-stats-mini.sh
```

The executable compiles the candidate's fixture test, starts it on a random
loopback port, runs the browser checks against that process, and stops its own
fixture on exit. It does not read archive configuration, credentials or live
sessions. Its temporary artifact directory retains the candidate SHA, fixture
and browser logs, test executable, and an optional screenshot. No install or
service/configuration change is part of the procedure. The fixture shuts down
at the five-minute test timeout even if its invoking shell disappears.

The browser script checks a Stats-first load without session/picker requests,
all six cards, complete project table, 168 heat cells, Denver timezone, rates,
project drilldown, and immediate paired list-to-Stats reuse. Node production
code tests additionally cover obsolete Stats/list/search cancellation and
response races; Go tests cover repeated DST prompt instances and cancellation
at every local source stage. These checks are distinct from real browser
layout/interaction acceptance.

For manual review, use the existing opt-in fixture test in a separate Mini
terminal and open only the printed loopback URL:

```sh
HEV_DASHBOARD_FIXTURE_LISTEN=127.0.0.1:0 go test -v -run '^TestDashboardFixtureServer$' -timeout 10m ./internal/serve
```

Review card/tooltip values, all project rows, model folding, daily spend/token
charts, timezone heatmap and coverage notice, filter chips, custom dates,
project/day/model/cell drilldowns, loading/error behavior, refresh and tab
navigation. DevTools should show obsolete filter requests cancelled, a cold
Stats request without a list scan, and no extra Stats request after a paired
list response. Navigate while paired loading to confirm its useful response
is reused. Record exact candidate, commands, results and screenshots; stop
only the owned fixture when finished. No visual acceptance is claimed until
this separate review actually runs.
