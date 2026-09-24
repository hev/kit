# Factory accounting view

`/api/factory` reads the factory repository's content-free accounting export.
Existing trace routes and the Layer dashboard remain available. Configure it
with ordinary files, then run:

> The dashboard's Factory tab was removed on 2026-09-23 while the traces view is
> reworked; how kit and factory accounting meet in one UI is still open. The
> API, `--factory-dir` and their Go tests stay. The tab's browser check
> (`scripts/test-factory-cost.cjs`) went with it and is in git history.

```sh
hev serve --factory-dir "$HOME/.factory/costs" \
  --factory-team example --factory-repos example/api,example/web
```

Copy `prices.toml` and `subscriptions.toml` into that directory. The price
file contains observed standard API rates and separately verified historical
rows. [Dated sources and exact boundaries](cost-rate-sources.md) document each
derivation and the remaining unknown categories. Each row links its provider
source. Current observations alone do not backdate rates. Subscription configuration
is an editable schema; only add plans actually held. Omit `monthly_usd` when
the actual bill is unknown: allocation and savings remain unavailable, not zero.
An entitlement or provider list price alone does not establish the net bill.
Missing `monthly_usd` is displayed as an operator-editable placeholder; for
Codex, an omitted or empty `tier` is likewise labelled. Fill these fields in
`subscriptions.toml` and reload the view; no restart is needed. Tier is a label,
not a guessed price or token allowance. Missing figures do not block source
completion, and savings stays unavailable even if an unpriced plan has no rows.

Prices are USD per million tokens and match model names exactly. Unknown
models/dates produce null API prices and incomplete totals. Per-request usage
preserves date, five-minute/one-hour cache creation and long-context thresholds.
Legacy aggregate usage cannot establish a long-context request and remains
unpriced when that distinction matters. Reasoning is included in output tokens.
The shipped table covers standard service; alternate service tiers require
separately verified rates and accounting evidence before acceptance is claimed.

`hev cost-price --dir "$HOME/.factory/costs" --output
"$HOME/.factory/costs/priced.jsonl"` writes the priced session snapshot for
factory's exact beat lookup. Factory's runnable `scripts/costs/refresh.py`
produces outcomes, sessions, attribution audit and pricing in one invocation,
suitable for an existing hourly scheduler. No scheduler or service is activated.

A subscription week begins on `reset_day` (Sunday=0), with UTC `reset_hour`,
`reset_minute` and `reset_second` (all default zero). Weekly spend is
`monthly_usd * 12 / 365.2425 * 7`. Each session receives its share of that
week's tokens, including role `other`. Overlapping plan selectors fail rather
than double bill. Allocation happens before window/filter selection. Include
complete weeks in the input; incomplete coverage cannot establish a complete
bill. A session spanning reset is allocated to its start week; per-request dates
are used for API pricing, not to split subscription allocation.

The arbitrage tile uses each plan's current billing week, independent of the
selected history window; its role/instance/model filters still apply.
The factory view shares window, date, project, model, harness, role and instance
filters. Trace-only predicates fail with an explicit message rather than
silently showing an unfiltered accounting result. Issue cost-to-date ignores
the selected window and shows the beginning of recorded history. Shipping
counts use outcome timestamps within the selected window and deduplicate PRs
and deploys. Unknown outcome data is displayed as unavailable.

## Scoped outcome cache

Factory's `scripts/costs/outcomes.py` builds `outcomes.json` from the parent's
scoped issue snapshot and explicit repository GitHub records. Its JSON schema
and `--validate-cache` command are in the factory companion PR. The hourly
refresh runs the producer automatically; future gaffers need not handcraft
outcome JSON. This reader makes no Linear calls. It rejects a wrong team,
out-of-scope repository, duplicate issue or invalid refresh time. A cache older
than an hour is labelled stale. All timestamps are Unix milliseconds.

Issue completion comes from the supplied `completedAt`. PR references are
explicit issue IDs. Successful deployments join equal merge SHAs or verified
GitHub ancestry, with merge-before-landing checks and per-merge proofs. A CI
build is not a deployment: configured workflows also require the exact deploy
job to succeed. Refreshing GitHub preserves the older issue snapshot timestamp.
The producer scans all PR pages and reports unassociated deployments and
comparison results. Global PRs and verified deployments are counted even without
an issue link; session attribution filters only count associated deployments.
Missing historical deploy records, unconfigured jobs and
issue-snapshot coverage remain limitations; ancestry does not verify artifacts.

## Plan usage

Codex weekly percentage comes from a rollout rate-limit window of 10,080
minutes (primary or secondary). It is never inferred from model tokens.
Factory's `scripts/costs/claude_usage.py` can read an existing GUI-domain grant and save its measured OAuth usage
response as `claude-usage.json`, adding `plan` and `observed_at` (milliseconds):

```json
{"plan":"your-claude-plan","observed_at":1788900000000,
 "limits":[{"kind":"weekly_all","percent":77,"resets_at":"2026-09-13T00:00:00Z"}]}
```

Only the measured response is stored; never write a credential here. This
reader does not access the keychain or provision an identity. Old/reset
observations are labelled stale. A plan without a measured observation uses
`weekly_token_basis` only when configured, prominently labelled a token
estimate; otherwise usage is unavailable.

## Acceptance

Run `go test ./...` and `go vet ./...`. The optional synthetic browser fixture:

```sh
HEV_FACTORY_FIXTURE_LISTEN=127.0.0.1:18789 \
  go test ./internal/serve -run '^TestFactoryFixtureServer$' -timeout 30m -v
```

`hev trace list --factory-dir ~/.factory/costs --harness codex --since 7d
--json` exposes all accounting rows. Optional `--require-complete` is a strict
diagnostic that fails for incomplete model, usage or attribution; the amended
plan accepts these visible historical gaps. It does not hide zero-token sessions to make an
acceptance count pass. This accounting list is separate from the existing
Layer-backed `hev ls` and transcript inspection.

Fixture screenshots are not deployed previews. The scoped live cache and measured usage adapters have been exercised locally. Historical gaps and missing bill/tier placeholders are accepted source limitations. Remaining live acceptance requires rollout through the operator gate, serving-host checks, and the separate same-week provider-UI comparison. Unsupported rates remain unknown. See the evidence matrix for every tail.

## Disposable replay acceptance

The supported public path replaces the absent `evals/layer-row.jq` with
factory's `python3 scripts/costs/eval_rows.py /private/evals.jsonl`, then feeds
that output to `hev eval put --namespace NAME`. Factory's historical findings
are objects: the adapter preserves them as canonical JSON strings accepted
by the generic eval schema. Direct raw historical replay fails; accounting
fields are not eval columns.

Build a branch binary and run `python3 scripts/test-eval-replay.py --hev
/path/to/hev --input /private/adapted-evals.jsonl`. The script always checks
synthetic eval input in a disposable loopback HTTP contract store and can
replay private adapted input twice, checking exact distinct IDs and unchanged
stored rows. It has no real credentials, hosted embedding or production
namespace. Synthetic checks cover 31 rows replaying unchanged, file/stdin
equivalence, timestamp normalization, insert-only grades, a new timestamp and
invalid input. Hosted-store acceptance and production backfill are separate
rollout gates.
