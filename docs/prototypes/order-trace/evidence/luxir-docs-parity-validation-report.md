# FAIL — Luxir docs acceptance

Run: `run-90d8d40bd086279f485c4aa239c2e31c`.
Source: `bf6e69893cbdc1deff669c7c4fa94ae3f2e7eaef`; baseline: `4566e906`. Checkout clean before and after inspection.

**Blocking finding: the supplied artifact does not contain the Luxir preview.** Inspected the actual archive in memory, without extraction or rebuild. Its Pro Quickstart, Write, Query and Install HTML each contain zero case-insensitive Luxir mentions and no Luxir picker button. Quickstart lacks `luxir-first-write-and-query` and the AWS installer gap callout; Write lacks the styled Python write-response warning. `_redirects` has no Luxir redirect. Therefore rendered Turbopuffer/Luxir parity and required callout acceptance fail for this artifact, despite its correct source marker. This is consistent with default-gated output; the build environment was not independently established.

Artifact: `/home/hev/workspace/orders/luxir-docs-final-artifacts/luxir-docs-preview-bf6e6989.tar.gz`.
SHA-256 verified: `305dae891b41bcb74986143197e67d84671a0e22ced52be03d70e913006cbd4c`.
Public `dist/docs-preview-marker.json`: schema `hevlayer.docs-preview/v2`, branch `v0.8`, repository `hev/layer-pro`, exact assigned source SHA.

Passing evidence:

- Source diff preserves shared Pro signup/install/client headings and Layer SDK setup. Write and Query examples are outside Luxir StoreNote wrappers, following the existing store-note pattern; backend limitations remain local. Concepts retains compatible-wire migration and gateway enhancements, with the full Layer API exposed through Layer clients.
- Python warning occurs once in Write source, inside `StoreWarning` → `Callout` (`aside.callout`), scoped to Luxir/Pro. Quickstart and Query do not repeat it. StoreWarning participates in source filtering and enabled-store rendering.
- Quickstart and Install honestly identify the AWS installer gap. `apps/layer-cli/cmd/install.go:172` unconditionally checks the Turbopuffer key before dry-run; its flags offer no Luxir option. No installer change is part of this diff; LYR327 remains separate.
- `node --test site/scripts/luxir-docs.test.mjs`: 5/5 pass, including all-corpus default/CE source filtering and shared-flow checks.
- Archive: 55 CE HTML/text files have no Luxir mentions; root `llms.txt` and `llms-full.txt` also have none. There are 53 rendered Pro content pages, but the inspected required pages lack Luxir preview content.
- Git blob identities confirm Header.astro, Footer.astro, Base.astro and global.css are byte-identical to `4566e906`. No tracked Oktoberfest asset or fixed-theme test; no Oktoberfest/fixed-theme archive filename.

Scope: read-only source/artifact inspection and narrow Node tests; only this requested report was written. No rebuild, runtime, browser, agents, secrets, commit, push or deployment. Mini browser validation remains separate. Acceptance requires a supplied artifact that actually includes the gated Luxir preview, followed by rendered checks against that artifact.
