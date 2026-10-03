# Label-review acceptance evidence

All fixtures and notes in these checks are invented. No production archives, transcripts or audit journals are accessed.

The package integration suite passed with `go test -p 1 -timeout 45s ./pkg/dashboard`. It exercises the actual authenticated hosted handler, independent SQLite review records, pagination, two eval IDs in one session, save/revise/history, reopening the store, stale and concurrent revision conflicts, resume/status filters and totals, tenant separation, unauthenticated GET/POST denial, CSRF/identity validation, unknown-field impersonation denial, malformed writes and generic upstream errors.

The real Chromium walkthrough passed with an existing Playwright installation:

```sh
KIT_PLAYWRIGHT_MODULE=/absolute/path/to/node_modules/playwright \
  go test -v -p 1 -count=1 -run '^TestReviewBrowser$' -timeout 55s ./pkg/dashboard
```

It runs the real `/review` page and APIs in an authenticated loopback server with a temporary SQLite journal. Assertions cover:

- Two distinct labels from the same session; machine and prior agent provenance.
- Ambiguous context, preceding actions and repeated errors, with text-only rendering.
- Save, revise, both history entries, reload with the revised note intact, and resume the unreviewed queue after another reload.
- Per-kind coverage and visible truncation/shortage warnings.
- Separate tenant history, unauthenticated page and write denial, CSRF and reviewer impersonation denial.
- No browser JavaScript errors.

The Chromium check completed in 7.76 seconds after compilation. It saves no screenshots or evidence containing private data. These checks validate generic interfaces and browser behavior; deployed authentication, real source joins, population completeness and provider cursor binding remain deployment acceptance responsibilities. Human review does not remove source-ordering or sample-size requirements.
