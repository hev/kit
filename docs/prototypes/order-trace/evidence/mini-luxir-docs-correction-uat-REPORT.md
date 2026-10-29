# Luxir docs correction UAT — candidate 2f903686f9ad6a663d8221575e7e4fb823d33427
Artifact: /Users/hev/jobs/luxir-docs-correction-uat/site/dist (from site-dist-2f903686.tar.gz). Server (127.0.0.1:52537, pid 6851) stopped after report; no browser left running.
Final status: candidate 2f903686 NOT deployed; duplicate picker fix not produced (Oven dispatch failed before run creation, per parent). No rerun done.
Playwright: @playwright/test 1.64.1 symlinked read-only from ~/jobs/oven-903/app/node_modules; headless Chromium, external requests aborted.

## RESULT: FAIL (supplied script)
evidence/acceptance.log: strict-mode violation on /docs/pro/api/search/ — two [data-store-switch] tablists (header + second unstyled one under the "In preview" callout). Only Pro page with a duplicate (static grep of dist).

## Diagnostic (copy with .first() on the aria-selected check only): PASS
evidence/diagnostic-tolerate-dup.log, evidence/diag-tolerate-dup.acceptance.mjs. Every Pro page persists Luxir; keyboard Home/End; quickstart TOC; CE has no Luxir tab and not luxir; license page; no-JS shows both notes, hides switch.

## Visual/content (Luxir selected)
Screenshots desktop/mobile query, write, quickstart, search: evidence/*-top.png, *-full.png. content-report.json.
- Gap notes are section-local (query: Stable reads, Query by id, Rank expressions, Batch query, Hybrid text fusion, Query routing, Collapse, Counting, Single/Batch fetch; write: Upsert/delete incl. Python response gap, Patches/conditions, Stage).
- Examples are Layer client calls (client.query_namespace / write_namespace / batch_query_namespace / search_namespace).
- No standalone Luxir guide in nav; only in-page anchors (#luxir-first-write-and-query etc.).
Observations, not confirmed:
- Search page repeats the same "Enhanced /search ... rejected" note in Request, Collapse and Stores.
- First visuals pass captured desktop query with turbopuffer tab highlighted while Luxir content showed; on reload + 500ms the Luxir tab is aria-selected and highlighted correctly. May be a first-paint flash. Not investigated per parent.
- Sidebar group headings show with links display:none (11-12 of 53 visible) in both stores; may be intended collapse.
