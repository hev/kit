# Luxir docs final preview

Run identity: run-ecee5f9b5686b78d65380a1008eeaa9a
Assigned source: b4e59c4cdb00be2ef5982030f6537c9bddc9f3da
Final source commit: bf6e69893cbdc1deff669c7c4fa94ae3f2e7eaef

Normal reversal commit restores Header.astro, Footer.astro, layouts/Base.astro and styles/global.css byte for byte from both 04d75d11 and 4566e906, and deletes fixed-theme.test.mjs. These are the only five paths changed. StoreWarning, feature-filter, docs, docs tests and digest remain intact. Working tree is clean. No new mascot URL or beige requirement was introduced.

Validation:
- Source tests: 37 passed, zero failures.
- Preview build: PASS with PUBLIC_PREVIEW_NAV=1 PUBLIC_DOCS_LUXIR=1 PUBLIC_DOCS_SLOP_SCORE=0.
- Render acceptance: PASS all 53 Pro backend selectors, no-JS fallback, CE isolation, public gate, redirects and SDK code imports. Shared Quickstart signup/install/client flow and local Write Python warning are covered by retained source/render tests; concise AWS installer gap remains in Quickstart.
- Links: PASS 8,351 internal docs links/anchors across 159 pages.
- Digest: up to date, four distilled shards, zero pending/stale; verification PASS 445 anchors. No prose edits required digest regeneration.
- Initial build failed resolving Astro's generated content module. Moving the generated .astro cache out of the source tree and rebuilding resolved it without source changes.

Public marker: dist/docs-preview-marker.json, verified inside the archive:
```json
{
  "schema": "hevlayer.docs-preview/v2",
  "branch": "v0.8",
  "sourceRepository": "hev/layer-pro",
  "sourceSha": "bf6e69893cbdc1deff669c7c4fa94ae3f2e7eaef"
}
```

Artifact: /home/hev/workspace/orders/luxir-docs-final-artifacts/luxir-docs-preview-bf6e6989.tar.gz
SHA-256: 305dae891b41bcb74986143197e67d84671a0e22ced52be03d70e913006cbd4c

Archive contains dist/ including the public marker. Build and source-test logs are beside the archive. Independent review and Mini browser validation are next; neither was run here. No push, deployment, runtime checks or credential access occurred.
