# Bounded docs and theme handoff

Committed `b4e59c4cdb00be2ef5982030f6537c9bddc9f3da` on `luxir-docs-deploy-20261023`, directly on assigned checkpoint `04d75d1159321b32a81b3224139640b20b1d3696`.
Run: `run-de3c290977b3b212177871e5b8cf8cca`.

Preserved the checkpoint's 21-file docs parity work, shared SDK examples, nav fix, all 53 Pro backend switches, CE isolation and server minify hook. Added store-scoped styled Callout warnings for the Write Python post-commit response validation hazard and Quickstart AWS installer gap. AWS has no Luxir installer flag; LYR327 remains separate. Installation tutorial is not wrapped in a warning.

Fixed flat beige palette uses the supplied six hevmind colors with Layer orange accents and readable dark code blocks. Removed theme toggle, theme persistence/restoration, system-theme selection and graph-paper layer. Set theme-color to #f4e5c6. Footer uses exact https://hevmind.com/mascots/fall/footer-oktoberfest.png URL at 64px wide, natural height and descriptive alt. No seasonal banners or other redesign.

Validation: 39 source tests PASS; default and final preview builds PASS; default render checks PASS (159 docs pages); final preview static render PASS (53 Pro selectors with Luxir, CE isolation, local warnings, header nav, fixed theme, mascot); default and preview internal links PASS (8,351 links / 159 pages); digest build/status/assemble/verify PASS (445 anchors); git diff --check PASS. Browser and visual validation intentionally deferred to independent validator and Mini. No Rust builds, agents, credentials, Pangram, push or deploy used.

Final build flags: PUBLIC_PREVIEW_NAV=1 PUBLIC_DOCS_LUXIR=1 PUBLIC_DOCS_SLOP_SCORE=0. Final preview rebuilt sequentially after digest generation to ensure generated edition inputs match preview flags. A temporary render assertion mistakenly checked body demo links as header links; corrected scoped-header assertion passes. No product change was needed.

Docs baseline inherited from checkpoint evidence in luxir-docs-parity-artifacts/released-docs.html and edge-docs.html, relevant Quickstart, Install and Write sections. Maintained workspace helper resolves current edge train v0.8: https://v0.8.x.hevlayer.com/docs; released https://hevlayer.com/docs. No repeated broad corpus audit.

Artifacts: `/home/hev/workspace/orders/luxir-docs-theme-artifacts`. preview.tar.gz contains final dist and source-marker.json; source.tar is exact committed tree. EXACT_SHA and source-marker.json identify revision, parent and run. SHA256SUMS:

```text
054cae9dc5a63fa7d8cb5ac5e19bdf41e49ba86fa485cfa1247cc74680d6d50c  preview.tar.gz
1f2891af6045e3e8bd72a2697376f8993cc3a951e4cecce7bfce5bfcd3f04572  source.tar
```

Independent validator should verify hashes and exact SHA, then inspect theme/readability, responsive nav and footer mascot on Mini. Remote mascot availability and visual appearance have not been browser-validated here.
