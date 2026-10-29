# PASS — independent narrow nav validation

Run identity: run-4dddb9f288ca8da1b9e8bfee8121d60f
Exact candidate: 4566e906ecddf4ba02722d93ab3ffdc2e7f3136c
Compared: 936d67c9..4566e906. Checkout HEAD matches the assigned candidate.
Reviewed handoff: /home/hev/workspace/orders/luxir-nav-fix-report.md.

Diff contains only the two-line selector move in site/src/layouts/DocsLayout.astro and an 18-line regression addition in site/scripts/preview-pages.test.mjs. No Layer-client/Luxir feature, content, SDK, API, runtime, build configuration or other product changes are present. git diff --check passes.

Independently executed node --test scripts/*.test.mjs from site: 37 tests passed, 0 failed, including the sidebar/hidden-TOC regression and existing Luxir/CE isolation and SDK-gap source guards.

Artifact inspected directly in memory, without extraction or rebuild:
/home/hev/workspace/orders/luxir-nav-fix-artifacts/site-dist-4566e906ecddf4ba02722d93ab3ffdc2e7f3136c.tar.gz
SHA256 verified: 234fdbfeebb85eb14575bef53aa9d5980c246bc4b321a61db888c561f3c90e05
Archive dist/docs-preview-marker.json exactly matches schema hevlayer.docs-preview/v2, branch v0.8, sourceRepository hev/layer-pro and sourceSha 4566e906ecddf4ba02722d93ab3ffdc2e7f3136c. External marker and exactSHA agree.

Actual archived stylesheet dist/_astro/_slug_.C9uZSm0V.css contains:
```css
.docs-toc[data-astro-cid-mw7aashj] a[data-astro-cid-mw7aashj][hidden]{display:none}
.nav-group[data-astro-cid-mw7aashj] a[data-astro-cid-mw7aashj],.docs-toc[data-astro-cid-mw7aashj] a[data-astro-cid-mw7aashj]{display:block;…}
```
The shared anchor rule restores sidebar display:block. Hidden TOC specificity is (0,4,1), above the shared TOC selector (0,3,1), so display:none wins despite appearing earlier. Source equivalents are (0,2,1) versus (0,1,1). No shipped sidebar-anchor display:none rule was found. Both CE and Pro rendered navigation pages reference this stylesheet. This proves the CSS correction statically; browser visibility/clickability remains Mini’s separate assignment.

Independently checked all 8,353 internal docs links/anchors across 159 archived rendered pages: PASS.
Existing final-worker.bundle independently hashes to a4e828472688e8618f03f607b80046289dcd7f14b33ced9022d62b4ae7301677 and measures 8,348,875 bytes, matching final-bundle-size.json and below its 26,214,400-byte limit. This validates the supplied multipart file, without regenerating it.

No rebuild, runtime, browser, credentials, agents, commit, push or deploy performed. Checkout remains unchanged; only this requested external report was written. PASS is limited to the assigned source/artifact validation, not desktop/mobile browser acceptance.
