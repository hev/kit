# Luxir docs parity UAT: ACCEPT predeploy (bf6e6989, r2 tar)
Artifact: site.tar.gz sha256 0a0447daa6f3b08dd0dac8c4cfe0e148dc4fb20e768352525cbcd606aa82870b, marker sourceSha bf6e69893cbdc1deff669c7c4fa94ae3f2e7eaef.
First tar (sha256 305dae89...) was built without the Luxir feature flag and had no Luxir content, so it was rejected. Kept in evidence/bf6e6989-default-gated-*.

## Acceptance (focused.mjs, desktop 1440 + mobile 390): PASS
- Quickstart: same H2s under both backends (Sign up / Install in your AWS account / Switch your client). Gateway env and client snippets are identical.
- AWS installer gap: a styled, Luxir-only callout under step 2 that replaces the installer block.
- Python write response gap: a styled, Luxir-only callout on Write > Upsert and delete only. It is not visible on any other Pro page or under turbopuffer.
- No visible Luxir tutorial. The #luxir-first-write-and-query anchor stays as an invisible compatibility anchor.
- Luxir persists on every Pro page that has a backend switch. The sidebar is visible and clickable on desktop and mobile, Pro and CE (nav-regression.mjs).
- Theme unchanged: original Layer light/dark toggle, grid and footer. Theme acceptance was cancelled because that steering was meant for another session.

## Corrected expectations (harness assumptions, not product failures)
- Quickstart is now shared, so there are no store-specific TOC entries. nav-regression's "toc: no store-specific entries" check does not apply.
- The install code block legitimately differs: the installer has no Luxir option, so Luxir shows the gap callout instead. Compare client snippets by content, not by position.
- The warning copy says "after the write has committed", not "postcommit". Match on "Python write response gap".
- Luxir-only H2s on Install, Document model and Vectorstores are feature-local and acceptable.

Evidence: evidence/bf6e6989-r2/focused/{desktop,mobile}-{quickstart,write}-luxir-{top,full,callout}.png, evidence/bf6e6989-r2.{parity,nav,focused}.log.
No product edits, deploys or shared-service changes by this worker.

## Live (https://v0.8.x.hevlayer.com, marker bf6e6989): PASS
Quickstart + Write smoke only, desktop + mobile: callouts placed correctly, 41/41 sidebar links visible, no tutorial, Luxir persists Quickstart -> sidebar -> Write. evidence/live-bf6e6989{.log,-persist.log,/}, evidence/live-marker-bf6e6989.json. Pending: recheck on the final immutable deployment URL.

## Final deployment (https://3b56d7a5.hevlayer.pages.dev, marker bf6e6989; custom domain marker matches): PASS
Same Quickstart + Write smoke, desktop + mobile, plus persistence. evidence/final-3b56d7a5{.log,-persist.log,/}, evidence/final-markers.txt. UAT closed.
