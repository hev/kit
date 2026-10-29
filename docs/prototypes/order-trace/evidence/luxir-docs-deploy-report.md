# Luxir docs deploy candidate

Ready for parent foh-hev-3 independent validation and deployment. No deployment performed.
Run identity: run-39f4f50280bac22471ca0b0f68d668ae.

- Final source SHA: `4559d0dd623ce87f0a8ead29a3ff2e3f3ae5a8a8` (clean worktree).
- Owned worktree: `/home/hev/workspace/orders/luxir-docs-deploy-20261023`, branch `luxir-docs-deploy-20261023`.
- Fetched upstream main: `2f81bd0fad101abb9f3ed285d5f3599346d4d6f5`. Cherry-picked ONLY assigned docs commit `edc74674821ccdf71b382e3b1a8314d735b3867d` as `b8199735`; added preview workflow/test changes in final commit. No runtime/demo ancestor commits carried over. Current upstream docs/site preserved. Final delta: 24 files, only site and continuous-mirror workflow.
- Read root AGENTS, prior report, workflow deployment shape, maintained marker script. Derived train with maintained helper: v0.8. Relevant released CE and edge Pro query docs fetched to artifacts; source query/write, quickstart, stores and dedicated preview contract inspected.

## Exact artifact

`/home/hev/workspace/orders/luxir-docs-deploy-artifacts/site-dist-4559d0dd.tar.gz`

SHA256: `d4b878e3bea1294d80f692332629d49bebcd84d61a280aa3908b5e41b25cbbcf`

Tar contains `dist/` from the final commit build, including `dist/docs-preview-marker.json`. Built with PUBLIC_PREVIEW_NAV=1 PUBLIC_DOCS_LUXIR=1 and PUBLIC_DOCS_SLOP_SCORE unset. Marker generated AND verified with maintained `scripts/write-docs-preview-marker.mjs`: schema hevlayer.docs-preview/v2, branch v0.8, sourceRepository hev/layer-pro, exact sourceSha above.

Workflow shape: independent preview job builds site alone, stamps marker, deploys dist to Cloudflare Pages project hevlayer, branch v0.8, commit hash exact source SHA; then converges custom domain and proves marker at alias/custom domain. Parent target is https://v0.8.x.hevlayer.com; never production branch. Parent can extract tar outside source and deploy its dist using existing authorization. No secrets accessed here.

## Scope and visibility

continuous-mirror preview now enables PUBLIC_DOCS_LUXIR=1 and removes PUBLIC_DOCS_SLOP_SCORE. Released workflows remain Luxir-off. CE contains no Luxir even in preview. Released store catalog unchanged. Pro quickstart/stores link directly to preview; query/write link to its relevant anchors. Expected data-store="luxir" and data-store-only="luxir" selectors checked across relevant Pro docs. Dedicated Pro preview remains explicitly private/default-off, bounded, not shipped or hosted support; unsupported operations and compatibility limits preserved.

Pangram refresh explicitly skipped per Adam's authorization. No broad Pangram cleanup. Minimal existing workflow assertion updated for requested flag behavior.

## Validation (exit codes)

Logs under `/home/hev/workspace/orders/luxir-docs-deploy-artifacts/`.

- frozen dependency install: 0; no lock changes.
- focused/all site Node tests final: 0 (tests-final.log). Initial run exited 1 because old workflow test required removed slop flag; corrected assertion, all tests pass.
- maintained preview marker tests: 0; stamp and verify: 0.
- Astro check: 0, zero errors/warnings, one unused Props hint.
- default build: 0; default Luxir isolation: 0; default links: 0.
- preview build: 0; preview Luxir isolation/selectors: 0; preview links: 0.
- store picker --render-only: 0; edition --render-only: 0. No browser launched.
- digest build/status/assemble: each 0; strict verify: 0, 448 anchors, 4 distilled shards, no pending/stale. Reused prior maintained shard inputs; digest content hash unchanged, refreshed timestamp only.
- CE capability matrix source check: 0.
- final committed-source preview rebuild: 0; final preview isolation: 0; final links: 0 (8440 links/anchors, 161 pages).
- final discovery/slop-off/catalog check: 0 (final-artifact-check.log). Initial extra discovery assertion was too strict for query/write section anchors; allowed actual anchor URLs and passed; no source change needed.
- git diff --check: 0; worktree clean.

No Rust or compiled CE checks needed for this docs-only candidate. No push, merge, deployment, runtime changes, agents, browser, credential reads or release publication. No blocking findings. Independent validation and actual deployment remain with parent; this report does not claim a live rollout. Prior preview's missing generated Luxir capability column remains explicitly documented rather than fabricated.
