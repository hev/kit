# Final narrow independent validation: PASS

Run: run-f8a3be1d457b238ffa86e3a1a3cddd29.
Exact assigned candidate and checkout HEAD: 936d67c9f47337642cdb5a3f9d0c00f66cf87006.
Baseline: 2f903686f9ad6a663d8221575e7e4fb823d33427.

Artifact: /home/hev/workspace/orders/luxir-docs-picker-fix-artifacts/site-dist-936d67c9f47337642cdb5a3f9d0c00f66cf87006.tar.gz
Independently calculated SHA256: e1fb7185ef30a75cc023dd25bf058210b3f3287d5f1b13686e970498e61c9921.

Read picker-fix report and prior correction validation. Independently inspected the complete baseline..HEAD diff: only Search's inline StoreSwitch/import removal, exactly-one rendered picker/Luxir-button assertions, and digest timestamp/removal of the obsolete import literal. Digest contentHash/context remain unchanged. No other source changes; diff whitespace check passes and checkout is clean.

Exact packaged marker equals schema hevlayer.docs-preview/v2, sourceRepository hev/layer-pro, branch v0.8, sourceSha 936d67c9f47337642cdb5a3f9d0c00f66cf87006. Maintained workspace-semver-branch.sh independently resolves v0.8.

Checks rerun against the extracted actual final tar:
- Rendered acceptance PASS: all 53 Pro content pages have exactly one selector and exactly one Luxir button; no-JS fallback, prepaint registry, CE isolation, SDK imports and redirects pass.
- Independent CE scan: all 55 shipped CE HTML/text/JSON files contain no Luxir.
- Internal links/anchors PASS: 8,353 across 159 rendered pages.
- Regression control: the same tightened acceptance run against the prior 2f903686 tar fails specifically on Search's backend picker count, actual 2 versus expected 1. Final artifact passes that assertion.
- Packaged introduction retains full Layer API through Layer clients for CE and Pro; concepts retains compatible migration plus enhancements for Turbopuffer and Luxir. AGENTS.md is unchanged and explicitly retains both goals and full API through Layer SDKs.

Acceptance harness caveat: as in prior validation, the tar excludes the source-only .hev-ask/editions/ce.json. A temporary script copy omitted only that unavailable-file assertion; independent scanning of all shipped CE HTML/text/JSON replaces it. Candidate files were untouched. Extracted artifacts and regression failure log: /tmp/luxir-final-validation-2d4zxwai/ (old-regression.log).

Prior broad product/artifact evidence remains valid within its stated boundaries; this report does not repeat native tests or rebuilds. PASS covers the narrow source change and exact packaged artifact. Exhaustive browser validation on Mini is a separate parallel gate, not claimed here. No source edits, commits, pushes, deployment, secrets, browser or agents used.
