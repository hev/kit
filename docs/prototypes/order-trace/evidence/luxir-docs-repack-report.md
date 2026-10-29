# PASS — Luxir preview artifact repack

Run: `run-3dd6cda765cd25737522d7736bf31523`.
Source: `bf6e69893cbdc1deff669c7c4fa94ae3f2e7eaef`. Git checkout remains clean; no tracked source changes.

Artifact: `/home/hev/workspace/orders/luxir-docs-repacked-artifacts/preview.tar.gz`
SHA-256: `0a0447daa6f3b08dd0dac8c4cfe0e148dc4fb20e768352525cbcd606aa82870b`
Fresh extraction: `/home/hev/workspace/orders/luxir-docs-repacked-artifacts/extracted-fcDbAJ/dist`

## Cause

The failed archive was default-gated output, despite a correct source marker. `site/astro.config.mjs` invokes `generateEditions()` and `generateEditionDigests()` during configuration. Edition filtering reads `process.env.PUBLIC_DOCS_LUXIR`; absent `1`, it removes Luxir before HTML compilation. `site/scripts/doc-redirects.mjs` separately requires that process flag to emit the legacy Luxir redirects. Thus a correct marker alone proves no feature content.

A concrete overwrite hazard exists in `site/scripts/store-picker.acceptance.mjs`: it invokes Astro builds twice using its inherited environment. Running that acceptance script without the exported Luxir flag overwrites both generated inputs and dist with default output. The previous failing archive is consistent with an unflagged last build or such an overwrite; retained evidence does not establish which historical command caused it. No source fix is needed: the explicit-environment final build produced all required content.

## Final build

Exactly one build was run for this rework: `pnpm run build` in `site/`, with `PUBLIC_PREVIEW_NAV=1 PUBLIC_DOCS_LUXIR=1 PUBLIC_DOCS_SLOP_SCORE=0` exported in its parent shell throughout the process. Exit status 0. Before it, removed dist, generated CE/Pro source views, generated edition manifests/digests, and Astro/Vite caches. Astro regenerated edition inputs under those same flags. The committed canonical digest remained unchanged. Log: `/home/hev/workspace/orders/luxir-docs-repacked-artifacts/final-build.log`.

After that build, only direct file inspection, marker stamping/verification, packaging, extraction, hashing and reporting ran. No acceptance/build/regeneration script ran afterward. Marker uses the maintained writer and was verified: schema `hevlayer.docs-preview/v2`, branch `v0.8`, sourceRepository `hev/layer-pro`, exact assigned source SHA.

## Direct HTML and archive proof

The same read-only HTML parser inspected actual `site/dist` and the fresh archive extraction, without invoking Astro or importing content generators. Both independently passed:

- Pro Quickstart: exactly 1 Luxir picker button; AWS installer gap in a Luxir-scoped warning with `aside.callout`; shared headings “1. Sign up for a 30-day trial”, “2. Install in your AWS account”, “3. Switch your client”; `luxir-first-write-and-query` anchor.
- Pro Write: exactly 1 Luxir-scoped `python-write-response` warning, containing `aside.callout`, “Python write response gap”, billing, committed-write warning, and Go/TypeScript/HTTP guidance.
- All 53 Pro HTML pages: exactly 53 Luxir buttons, one per page.
- All 55 CE HTML/text files: zero case-insensitive Luxir mentions.
- `_redirects`: all 4 old Luxir URL variants (`/docs/luxir`, `/docs/luxir/`, `/docs/pro/luxir`, `/docs/pro/luxir/`) redirect with 301 to `/docs/pro/quickstart/#luxir-first-write-and-query`.
- Exact marker values pass inside the extracted archive.
- All 571 extracted files have identical relative paths and SHA-256 content hashes to the inspected final dist.

Machine-readable evidence: `/home/hev/workspace/orders/luxir-docs-repacked-artifacts/dist-proof.json`, `/home/hev/workspace/orders/luxir-docs-repacked-artifacts/archive-proof.json` (includes all 53 picker page paths), `/home/hev/workspace/orders/luxir-docs-repacked-artifacts/file-hashes.json`, `/home/hev/workspace/orders/luxir-docs-repacked-artifacts/preview.sha256`. Read-only verifier: `/home/hev/workspace/orders/luxir-docs-repacked-artifacts/inspect-output.py`.

No push, deployment, secrets, browser, agents, Rust builds or unrelated tests. Independent validator and Mini may consume this new archive directly.
