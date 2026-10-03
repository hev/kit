# Hosted label review

`pkg/dashboard.NewHosted` can enable `/review` inside the existing authenticated dashboard. Set `Config.Review` to a `ReviewConfig` with configured kinds, a `LabelSource`, a `ReviewStore`, and a verified `ReviewIdentity`. The dashboard exposes a label-review navigation link only when configured. Existing deployments keep their current behavior.

`ReviewIdentity` runs after the existing resolver on every review page, asset, read, and write. It must verify a human session and return a stable reviewer ID and tenant equal to the resolver's archive namespace. Never derive these identities from browser query parameters, JSON, or unverified headers. A service/agent session must not resolve as a human. The extension does not supply credentials, hosting, or an authentication mechanism.

## Provider contract

`LabelSource.Queue` receives a verified tenant and validated kind/status/uncertainty filters, an opaque cursor, and a limit between 1 and 100 (default 25). Retain every applicable eval; never deduplicate by session. Give each label an opaque stable ID and retain eval ID, source reference, policy, session, turn, kind, prediction and score. Distinguish genuine samples, controlled probes and synthetic samples; do not pool their precision denominators. Prior judgments declare `actor_type` (`agent` or `human`) and actor. Machine predictions remain separate.

Index once or use bounded storage pagination. Bind cursors to tenant, filter selection, and index version. Reject mismatches. Do not scan the entire archive for each card. Apply review-status filtering against the same durable review store that handles writes. Return totals for all configured kinds over the indexed population, independent of page/filter selection: reviewed, remaining, uncertain, required and shortage. Separate `KindTotals` rows by `SampleClass`. Set each row’s `State` to `complete`, `lower_bound` or `unavailable`; unavailable counts display as unavailable. Explain the population, unavailable sources and any uncertainty definition in `Coverage`; set `Truncated` whenever the index or source is incomplete. Unknown totals must be explained as unavailable rather than represented as complete zero counts. Sample shortages do not establish quality or remove source-ordering gates.

`Context` authorizes tenant ownership for every item, including before history and saves. Return `ErrReviewNotFound` for nonexistent or foreign items. Fetch only the selected item's context. Return chronological preceding assistant/tool actions and relevant repeated errors, source references, and the target event. Mark missing, ambiguous or truncated context explicitly with an explanation. Never fabricate missing conversation. Queue calls must not return full transcripts.

`ReviewStore` preserves machine predictions and prior evidence. `Save` atomically compares the expected revision with the tenant/item's latest revision, appends a new human review using the server-supplied principal, and returns `ErrReviewConflict` on stale writes. Retain all revisions and timestamps; `History` returns oldest first. Verdicts are `correct`, `incorrect` or `uncertain`; notes are optional. Item ownership is checked through the source before store access. Store implementations must additionally partition by tenant.

`OpenReviewStore(privatePersistentDirectory)` provides an optional SQLite implementation with atomic revision comparison, tenant partitioning, and append-only history. Close it on shutdown. Keep its database and directory private, back them up as private records, and use persistent local disk. Separate processes can open the same database. The wrapper can instead implement `ReviewStore` in existing authorized storage. The extension does not write producer evals or audit journals.

## Browser and security

Open `/review`, filter kinds/status/uncertainty, then load context and history for a label. Save a verdict and optional note. Reload context/history to revisit or revise it; conflicts require reloading before saving. Filters and the current page cursor persist in the URL; saved judgments persist in the store. Applying `unreviewed` resumes the remaining queue. Coverage refreshes on reload or Apply filters.

Writes require same-origin `Origin`, JSON, and `X-Kit-Review: 1`. Missing Origin fails closed. Reverse proxies must preserve the browser-facing Host and TLS; do not trust forwarded headers without wrapper validation. The browser sends no tenant, reviewer, actor type or timestamp fields. Unknown fields are rejected. Pages use a restrictive CSP, text-only rendering for evidence/notes, private no-store caching, and generic storage errors. Providers must also avoid logging private context, notes, credentials or upstream error bodies.

## Synthetic acceptance checks

Run `go test -race -count=1 ./pkg/dashboard`. Integration coverage includes two evals in one session, bounded pagination, stale-revision conflicts, concurrent store writers, reopening durable history, resume filters, tenant partitioning, unauthenticated read/write denial, CSRF, impersonation denial and generic error bodies.

For an actual browser check using an already installed Playwright and Chromium:

```sh
KIT_PLAYWRIGHT_MODULE=/absolute/path/to/node_modules/playwright \
  go test -count=1 -run '^TestReviewBrowser$' -timeout 60s ./pkg/dashboard
```

The test starts an authenticated loopback dashboard with invented fixtures and a temporary review database. It checks the real page and APIs through Chromium; no downloads or private archives are used. Production access and archive adapters belong to the authenticated deployment and need their own tenant-isolation and source-completeness checks.
