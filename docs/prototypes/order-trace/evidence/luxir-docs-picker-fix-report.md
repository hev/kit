# Picker fix: PASS, ready for independent Mini artifact UAT

Run identity: run-9696fcb5fe02e8da50dd074cda8d1335.
Assigned source: 2f903686f9ad6a663d8221575e7e4fb823d33427.
Final committed source: 936d67c9f47337642cdb5a3f9d0c00f66cf87006. Local commit only; checkout clean.

Removed only the obsolete inline StoreSwitch and its unused import from Search. Standard layout supplies the picker. Rendered preview acceptance now counts exactly one picker element and exactly one Luxir button on every Pro content page. Existing no-JS fallback assertions and behavior are unchanged. No product prose, framing, SDK examples, runtime, root AGENTS or its two goals changed. Prior independent source/product review remains at /home/hev/workspace/orders/luxir-docs-correction-validation-report.md.

Required digest regeneration refreshed its timestamp and removed the obsolete StoreSwitch.astro literal; contentHash, summaries, glossary and product context are unchanged. Strict verification restored source projections and passed before final builds.

Validation:
- Focused Luxir source tests: 5/5 PASS.
- Regression evidence: original exact assigned tar Search has two pickers and two Luxir tabs; new count assertions reject it (regression-before.log).
- Digest build/status/assemble/strict verify: PASS; 4 distilled shards, 0 pending/stale; 445 anchors verified.
- Fresh default build and default rendered acceptance: PASS; CE/public Luxir exclusion and no-JS assertions preserved.
- Default links: 8,489 links/anchors across 159 pages PASS.
- Final preview rebuilt AFTER digest/default builds with PUBLIC_PREVIEW_NAV=1 PUBLIC_DOCS_LUXIR=1 and PUBLIC_DOCS_SLOP_SCORE unset.
- Final preview rendered acceptance: all 53 Pro content pages have exactly one picker/Luxir tab; CE isolation, no-JS fallback, redirects and SDK imports PASS.
- Final preview links: 8,353 links/anchors across 159 pages PASS.
- Marker written and verified: hevlayer.docs-preview/v2, hev/layer-pro, branch v0.8 (maintained semver helper), exact final committed SHA above.
- git diff --check PASS; final git status clean.

Artifact: /home/hev/workspace/orders/luxir-docs-picker-fix-artifacts/site-dist-936d67c9f47337642cdb5a3f9d0c00f66cf87006.tar.gz
SHA256: e1fb7185ef30a75cc023dd25bf058210b3f3287d5f1b13686e970498e61c9921

Actual Wrangler multipart measured using the prior measure-bundle.py helper, not directory size:
- Multipart bytes: 8,348,767
- Module count: 331
- Module bytes: 8,283,118
- Limit: 26,214,400; headroom: 17,865,633
- Multipart SHA256: e26c0c0d4ae749cb763f819780e8792702ece78aa8079dd5377f89fdd7bfa16d
- Payload: /home/hev/workspace/orders/luxir-docs-picker-fix-artifacts/final-worker.bundle
- Measurement: /home/hev/workspace/orders/luxir-docs-picker-fix-artifacts/final-bundle-size.json

All logs retained in /home/hev/workspace/orders/luxir-docs-picker-fix-artifacts. Final site/dist remains the stamped preview. No push, merge, deploy, agents, browser, secrets, native engine or Pangram calls. Independent Mini validation of this actual final artifact remains the parent's gate before deployment.

Diff (digest metadata refresh described above):
```diff
diff --git a/site/scripts/luxir-docs.acceptance.mjs b/site/scripts/luxir-docs.acceptance.mjs
index 67ea7cd6..4a931fbc 100644
--- a/site/scripts/luxir-docs.acceptance.mjs
+++ b/site/scripts/luxir-docs.acceptance.mjs
@@ -11,8 +11,8 @@ for (const file of files(path.join(root,'docs')).filter(f => /\.(html|txt)$/.tes
  if (file.includes('/docs/ce/') || !preview) assert.doesNotMatch(html, /luxir/i, file);
  if (preview && file.includes('/docs/pro/') && file.endsWith('.html') && html.includes('class="docs-content"')) {
   pro++;
-  assert.match(html, /data-store="luxir"/, file);
-  assert.match(html, />Luxir<\/button>/, file);
+  assert.equal([...html.matchAll(/<[^>]+\bdata-store-switch(?:\s|>)/g)].length, 1, `${file}: exactly one backend picker`);
+  assert.equal([...html.matchAll(/<button\b[^>]*\bdata-store="luxir"[^>]*>\s*Luxir\s*<\/button>/g)].length, 1, `${file}: exactly one Luxir tab`);
   assert.doesNotMatch(html, /data-store="pgvector"/, file);
   assert.match(html, /<noscript>/, file);
   assert.match(html, /All backend notes are shown/, file);
diff --git a/site/src/content/docs/api/search.mdx b/site/src/content/docs/api/search.mdx
index 2e8d9ddf..c4e27534 100644
--- a/site/src/content/docs/api/search.mdx
+++ b/site/src/content/docs/api/search.mdx
@@ -12,13 +12,11 @@ import Edition from "../../../components/docs/Edition.astro";
 import Preview from "../../../components/docs/Preview.astro";
 
 import CodeTabs from "../../../components/docs/CodeTabs.astro";
-import StoreSwitch from "../../../components/docs/StoreSwitch.astro";
 import StoreNote from "../../../components/docs/StoreNote.astro";
 import StoreMatrixLink from "../../../components/docs/StoreMatrixLink.astro";
 
 <Preview />
 
-<StoreSwitch />
 
 `POST /v2/namespaces/{namespace}/search` takes a query string and returns ranked,
 scored rows. In one request, inside one namespace, the gateway:
```
