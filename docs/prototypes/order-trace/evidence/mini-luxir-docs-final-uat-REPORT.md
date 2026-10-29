# Luxir docs final UAT: PASS

Artifact: sourceSha 936d67c9f47337642cdb5a3f9d0c00f66cf87006 (dist/docs-preview-marker.json), site.tar.gz sha256 e1fb7185ef30a75cc023dd25bf058210b3f3287d5f1b13686e970498e61c9921 (matches).
Server: owned python http.server on 127.0.0.1, free port (stopped after the run). External requests aborted.
Playwright: @playwright/test symlinked read-only from ~/jobs/oven-903/app/node_modules (same as yesterday); headless Chromium.

## Original acceptance script: PASS
`site/scripts/luxir-picker.acceptance.mjs`, unmodified (sha256 4fb31675…a5a1, identical to the copy that failed on 2f903686). No duplicate tolerance.
Output: `PASS exhaustive persisted Luxir navigation, keyboard, CE round trip and no-JS notes` (exit 0). Log: evidence/acceptance.log.
Covers all 53 Pro pages (selection persistence), Home/End keyboard, CE round trip, no-JS notes.

## Visuals (Luxir selected): evidence/
- Search desktop + mobile: evidence/{desktop,mobile}-api-search-{top,full}.png. One store selector now (was 2).
- Query and write desktop + mobile: evidence/{desktop,mobile}-api-{query,write}-{top,full}.png.
- Content report vs. yesterday's 2f903686 run: identical visible Luxir blocks, code, nav links and turbopuffer-only counts on query, write, quickstart and search. The only difference is the selector count on Search (2 → 1). Layer SDK, inline-gap and two-goals content is unchanged from the reviewed version.

## Notes (same as yesterday, not blockers)
- Screenshots taken right after navigation show the turbopuffer tab highlighted while Luxir content is showing. After the page settles (networkidle + 800ms), Luxir is aria-selected and highlighted on search, query and write (evidence/settle-probe.mjs, settled-*-switch.png). This is a first-paint flash.
- Sidebar group links are collapsed (display:none) in both stores, as before.

## Live check (https://v0.8.x.hevlayer.com): PASS
- Marker: sourceSha 936d67c9f47337642cdb5a3f9d0c00f66cf87006, branch v0.8 (evidence/live-marker.json).
- Redirects: /docs/luxir, /docs/luxir/, /docs/pro/luxir, /docs/pro/luxir/ each return 301 to /docs/pro/quickstart/#luxir-first-write-and-query. The browser lands on that anchor (evidence/live-redirects.txt).
- Search: exactly 1 backend picker and 1 Luxir tab.
- Persistence: picking Luxir on Search carries over to /docs/pro/api/write/ (data-store=luxir, Luxir aria-selected=true; evidence/live-write-luxir.png).
- Script and log: evidence/live-check.mjs, evidence/live-check.log. Headless Chromium, requests to other origins blocked.
