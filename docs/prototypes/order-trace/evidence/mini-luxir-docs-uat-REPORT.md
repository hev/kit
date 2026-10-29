# Luxir docs post-deploy UAT — PASS

- Target: https://v0.8.x.hevlayer.com/docs/pro/luxir/
- Source: hev/layer-pro@e7f93576d3574b8a78a6668308fee5fa375cc07b (confirmed at /docs-preview-marker.json, branch v0.8)
- Run: 2026-10-23, mini, headless Chromium, fresh profile. No forms, messages, purchases or changes.

| Check | Result |
|---|---|
| Luxir page renders, desktop 1440x900 (200, h1, no JS errors) | PASS |
| Luxir page renders, mobile 390x844 (0px horizontal overflow) | PASS |
| Visible "IN PREVIEW" label plus private/default-off preview copy | PASS |
| Pro Query: "Luxir (private preview)" tab swaps in Luxir-scoped text ("Luxir serves explicit ANN and BM25 ... Read the private preview query contract") | PASS |
| Pro Write: Luxir tab swaps in scoped text ("Luxir accepts full-document upsert_rows and string-ID deletes ... See preview writes") | PASS |
| CE /docs/ce, /ce/api/query, /ce/api/write: store tabs are turbopuffer + Postgres, no "luxir" anywhere in HTML | PASS |
| No Pangram badge or reference (all pages checked) | PASS |

Notes
- The store choice persists across pages: picking Luxir on Query keeps it selected on Write. Looks intended.
- On mobile the full sidebar nav renders above the article, so the h1 starts about 600px down. That's existing site layout, not something this deploy introduced.

Artifacts (~/jobs/luxir-docs-uat/)
- screens/luxir-desktop.png, luxir-desktop-full.png, luxir-mobile.png, luxir-mobile-full.png
- screens/pro-query-luxir-tab.png, pro-write-luxir-tab.png, ce-desktop.png
- results.json, scripts/uat.cjs
- evidence-001/recording.mp4, Studio UAT: http://100.126.12.95:8800/#uat/156

Harness issues (not product failures)
- The first run of scripts/uat.cjs logged 2 FAILs in results.json, on the Pro query/write "scoped text" checks. The cause was the test locator: it read the first visible `[role=tabpanel]`, which was a code-sample tab, not the store-scoped prose. On write, the stored Luxir choice from the query page also hid the change.
- Corrected check (scripts/diff.cjs, fresh context for each page) diffs the article text before and after clicking the Luxir tab. Both pages swap in the Luxir-scoped text quoted above. The Studio recording (evidence-001) asserts that same text.
