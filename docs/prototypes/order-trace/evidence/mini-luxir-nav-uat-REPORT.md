# Luxir docs sidebar nav UAT: PASS (4566e906)
Bug (live 936d67c9): `.nav-group a, .docs-toc a[hidden]{display:none}` hid every sidebar link. 0/41 Pro, 0/25 CE visible, 0x0, desktop+mobile. evidence/live-repro.{json,log}, evidence/live-*-sidebar.png, evidence/regression-live.log (fails).
Artifact: site.tar.gz sha256 234fdbfeebb85eb14575bef53aa9d5980c246bc4b321a61db888c561f3c90e05, marker sourceSha 4566e906ecddf4ba02722d93ab3ffdc2e7f3136c.
Regression: nav-regression.mjs <origin> <outdir>. Checks all groups (Pro 5, CE 4) visible, nonzero size, desktop 1440 and mobile 390. Clicks one link per group. Luxir persists on every Pro page it reaches. Inactive-backend TOC entries stay hidden after each backend pick (TOC rail is display:none at <=1040px by design, so only the hidden-entry checks apply on mobile).
Predeploy (local, isolated server): PASS. evidence/regression-4566e906/
Live after deploy (marker 4566e906): PASS. evidence/regression-live-4566e906/
Closed: local server stopped, no browser processes left. No product source changed, no deploys by this worker.
