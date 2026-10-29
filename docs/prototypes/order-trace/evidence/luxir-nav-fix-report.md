# Nav regression fixed; artifact ready for Mini validation

Run identity: run-5e6f33b3f95230432c7941921760924c.
Assigned source: 936d67c9f47337642cdb5a3f9d0c00f66cf87006.
sourceSHA: 4566e906ecddf4ba02722d93ab3ffdc2e7f3136c
Owned branch: luxir-docs-deploy-20261023. Local commit only; no push or deployment.

DocsLayout.astro restores `.nav-group a` to the shared `.docs-toc a` visible styling (`display: block`). `.docs-toc a[hidden]` stands alone with `display: none`; its attribute specificity continues to override the shared TOC rule. Only the layout and existing preview-pages source test changed. Full Layer SDK examples, all 53 Luxir switches, inline gaps, CE isolation and the server minify hook are preserved. No corpus/prose changes, so no digest or Pangram refresh was needed.

Checks:
- Read root AGENTS.md; exact assigned revision and clean checkout confirmed before edits.
- Maintained scripts/workspace-semver-branch.sh resolves v0.8. Released https://hevlayer.com/docs and edge https://v0.8.x.hevlayer.com/docs respond/redirect to /docs/. Relevant source is DocsLayout sidebar and TOC; no product-content changes.
- New regression fails against the original selector (regression-before.log), passes after fix. Asserts shared visible rule, no sidebar-wide hidden rule, standalone attribute-specific hidden TOC rule.
- All site source tests: 37/37 PASS (source-tests.log).
- Final build after commit: PUBLIC_PREVIEW_NAV=1 PUBLIC_DOCS_LUXIR=1 PUBLIC_DOCS_SLOP_SCORE=0 pnpm run build PASS (preview-build.log). Pangram off; no Pangram calls.
- Rendered preview acceptance PASS: all 53 Pro pages exactly one picker and Luxir button, no-JS fallback, CE isolation, SDK imports, inline committed-write gap and redirects (preview-acceptance.log).
- Compiled scoped CSS assertions PASS: shared sidebar/TOC display:block, standalone attribute-specific hidden TOC display:none, no sidebar-wide display:none (rendered-css.log).
- 8,353 internal docs links/anchors across 159 rendered pages PASS (preview-links.log).
- Source marker written and verified: hevlayer.docs-preview/v2, hev/layer-pro, branch v0.8, exact sourceSHA above (marker.log). Marker is in dist and copied into artifacts; exactSHA file also present.
- Wrangler pages functions build --build-output-directory dist --outfile produced actual multipart: 8,348,875 bytes, 331 modules, 8,283,226 module bytes; limit 26,214,400, headroom 17,865,525. Existing astro:build:setup server esbuild minify hook retained. Measurement uses prior measure-bundle.py procedure; details final-bundle-size.json.
- Multipart SHA256: a4e828472688e8618f03f607b80046289dcd7f14b33ced9022d62b4ae7301677.
- git diff --check PASS; final checkout clean.

Artifact: /home/hev/workspace/orders/luxir-nav-fix-artifacts/site-dist-4566e906ecddf4ba02722d93ab3ffdc2e7f3136c.tar.gz
Tar SHA256: 234fdbfeebb85eb14575bef53aa9d5980c246bc4b321a61db888c561f3c90e05
Archive contains dist/ including exact-SHA docs-preview-marker.json. Final site/dist is the stamped preview.
All logs and exactSHA marker: /home/hev/workspace/orders/luxir-nav-fix-artifacts.

Mini must separately validate computed sidebar/hidden-TOC visibility and screenshots against this exact artifact. No browser or runtime suite run here, as assigned. No agents, credential changes, shared-service changes, push or deploy.
