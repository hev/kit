# PASS — independent Luxir docs deployment validation

Run: `run-2576454c4d45199b9fa733fdf3fa95a1`  
Date: 2026-10-23 UTC  
Candidate: `4559d0dd623ce87f0a8ead29a3ff2e3f3ae5a8a8`  
Baseline main: `2f81bd0fad101abb9f3ed285d5f3599346d4d6f5`

**PASS for deployment of the supplied docs preview artifact. No blocking findings.** This is independent source/artifact validation, not evidence of a live deployment.

## Identity and scope

Read repository AGENTS.md and `/home/hev/workspace/orders/luxir-docs-deploy-report.md`. HEAD exactly matches the assignment. Maintained `scripts/workspace-semver-branch.sh` returns `v0.8`. Read released and edge docs landing pages at https://hevlayer.com/docs and https://v0.8.x.hevlayer.com/docs as context; reviewed candidate source sections covering dedicated configuration, SDK endpoint, writes/replacements, query/filter, unsupported surfaces, discovery, and edition/store filtering.

Reviewed the complete baseline-to-candidate delta: 24 files, 379 insertions, 35 deletions; only `site/` and `.github/workflows/continuous-mirror.yml`. The two added commits are `b8199735` and `4559d0dd`. No Rust, gateway, API spec, generated SDK, CLI, operator, infrastructure, runtime or demo changes. Explicit diff checks confirm Cargo.toml, apps/, crates/, clients/, infra/, public/, released-stores.json and site-deploy.yml are unchanged. `git diff --check` passes and candidate worktree remains clean.

## Exact supplied artifact

File: `/home/hev/workspace/orders/luxir-docs-deploy-artifacts/site-dist-4559d0dd.tar.gz`

Independently computed SHA256 matches the assigned value:
`d4b878e3bea1294d80f692332629d49bebcd84d61a280aa3908b5e41b25cbbcf`.

Extracted outside the candidate source. Maintained marker verifier passes with these exact values:

```json
{"schema":"hevlayer.docs-preview/v2","branch":"v0.8","sourceRepository":"hev/layer-pro","sourceSha":"4559d0dd623ce87f0a8ead29a3ff2e3f3ae5a8a8"}
```

Independent rendered-artifact inspection passes:

- Pro `/docs/pro/luxir` contains the private/default-off/dedicated Pro qualification, standalone configuration and engine/gateway origin distinction, official Python `base_url` and TypeScript `baseURL` examples, telemetry settings and tested SDK versions.
- Fixed-size schema, string IDs, finite/base64 vectors, full replacement semantics, ANN/BM25, scalar-filter bounds, depth 16, 400/422 behavior and unsupported operations remain explicit. No hosted, shipped, full SDK/wire compatibility, performance or cross-version persistence promise is introduced. Missing generated Luxir capability coverage is stated rather than invented.
- Pro quickstart, stores, API introduction, query and write expose the Luxir selector, scoped notes and correct preview links/anchors. Maintained acceptance additionally checks selectors/notes across install, concepts, vectorstores, document model, scans, embed and search. Pro llms.txt and llms-full.txt discover the preview.
- All 55 CE HTML/text/JSON files contain no Luxir; no CE Luxir route exists. CE edition corpora contain no Luxir. Pro preview edition switching does not link to a nonexistent CE preview page. Existing CE Postgres configuration remains present.
- All 168 artifact HTML pages have no rendered Pangram/slop badge (`details.slop-score`/`data-slop-score`) or unscored badge message.
- Maintained docs-link check: **8,440 internal links/anchors across 161 rendered docs pages pass**.
- Additional independent HTML asset-reference check: **877 local script/image/media/stylesheet/preload/icon references resolve**.

## Workflow and release isolation

The only workflow change replaces preview PUBLIC_DOCS_SLOP_SCORE with PUBLIC_DOCS_LUXIR=1. The flag occurs only in the versioned preview workflow. The preview builds site alone, derives branch v0.8, stamps source identity, deploys to the hevlayer Pages project with the train branch and exact commit hash, and verifies alias/custom-domain marker afterward. Existing release pinning remains unchanged; production site-deploy still pins released docs/catalog and does not enable Luxir. Released-store catalog remains byte-for-byte unchanged (`turbopuffer`, `pgvector`). Luxir defaults off and is scoped to Pro even when opted in.

## Independent execution

Used a temporary source copy, first a dependency symlink and then a private copy of the implementation node_modules; implementation dependencies and both source worktrees were not edited.

- Focused store/feature/edition/discovery/workflow/slop tests: **23/23 pass**.
- Complete `node --test scripts/*.test.mjs`: **33/33 pass**.
- Maintained marker tests: **3/3 pass**; supplied marker verification passes.
- `node scripts/ask.mjs digest verify`: **448 anchors verified**.
- Supplied artifact `luxir-docs.acceptance.mjs --preview`: **PASS**.
- Supplied artifact `docs-links.acceptance.mjs`: **PASS**, counts above.
- Separate clean default Astro build: **PASS**; default Luxir isolation **PASS**; **8,477 internal links/anchors across 159 docs pages pass**.

Logs and temporary copies: `/tmp/luxir-validation-2576454/` (`focused-tests.log`, `site-tests.log`, `digest-verify.log`, `default-build.log`). The verified extracted preview is preserved at `verified-artifact-dist/`.

Initial validation-harness attempts lacked dependency/fixture paths; these were corrected in the temporary copy. Astro could not compile through the external node_modules symlink, so dependencies were privately copied. An intermediate link check against digest-filtered build output failed; a clean normal default build and the freshly extracted exact preview artifact both pass their respective link checks. These intermediate outputs were not treated as deployment artifacts.

## Limitations and boundaries

No browser/visual check performed; Mini post-deploy validation is separate and does not block this result. No live deployment, custom-domain convergence, Cloudflare credentials or deployed endpoint identity was tested. No Rust/runtime/engine or SDK execution was performed: the documentation accurately bounds its claims, but this review does not independently reproduce the historical native/demo evidence. No redundant full Rust/mirror or Pangram refresh was run; Adam explicitly waived Pangram refresh. Local checks used Node v26.8.1; workflow specifies Node 22. This validates the supplied artifact and source isolation, not bit-for-bit reproducibility of its build.

No edits to candidate source, commits, pushes, deployments, secrets, release publication or agents. Only this requested report and temporary validation files were written.
