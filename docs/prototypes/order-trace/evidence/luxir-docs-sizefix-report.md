# Luxir docs Pages size fix

Candidate ready for parent independent validation and deployment to v0.8 / v0.8.x.hevlayer.com. No deployment performed; Cloudflare server acceptance remains for the parent to confirm.

Run: run-cd0bc1a859849cdb5b027410525073d6.
Owned checkout: /home/hev/workspace/orders/luxir-docs-deploy-20261023.
Assigned base: 4559d0dd623ce87f0a8ead29a3ff2e3f3ae5a8a8.
Final source: e7f93576d3574b8a78a6668308fee5fa375cc07b (`Minify Astro server graph for Pages uploads`). Clean worktree. One maintained file changed: site/astro.config.mjs, ten added lines. No dependency/lockfile, prose, API, runtime or workflow changes.

## Artifact

/home/hev/workspace/orders/luxir-docs-sizefix-artifacts/site-dist-e7f93576.tar.gz

SHA256: 3312c4700b236a5b0ae79b7ebfafec4d126fc73f7e2a9ae5db589a2cd2c0d2a6

Contains dist/ including maintained docs-preview-marker.json, generated and verified with schema hevlayer.docs-preview/v2, branch v0.8, sourceRepository hev/layer-pro and final sourceSha above. Built with PUBLIC_PREVIEW_NAV=1 PUBLIC_DOCS_LUXIR=1 and slop scoring unset. Old artifact/report untouched. The extracted packaged-check/dist is preserved separately; final site/dist restored from this exact tar after digest verification (the digest command itself rebuilds default output).

## Change and investigation

Astro explicitly sets SSR minify:false in node_modules/astro/dist/core/build/static-build.js after normal Vite configuration. An inline integration uses the maintained astro:build:setup hook, server target only, to set build.minify:"esbuild". This minifies the complete server graph before Cloudflare Pages collects its split modules. No route or required import is removed. Ordinary vite.build.minify would be overridden; @astrojs/cloudflare 12's adapter options expose no minification setting. A no-bundle deploy switch is supported by Wrangler but is unnecessary here; normal Pages deploy can consume the new dist without extra flags/config. Keeping the fix at build time also supports deployment from an extracted tar outside the source checkout.

Wrangler 4.95's `produceWorkerBundleForWorkerJSDirectory` discovers every .js/.mjs additional module, marks them external when bundling index.js, then attaches them. `pages functions build --build-output-directory dist --outfile PATH` uses that exact same function and `createUploadWorkerBundleContents` as `pages deploy`. Its output is the real multipart _worker.bundle upload payload, including entry, all attached chunks and metadata; this is stronger evidence than directory size or the additional-module table. No network authentication or deployment is needed. `--outfile` is accepted but labeled deprecated; --outdir writes module files instead of the serialized upload, so --outfile was deliberately used for measurement.

The suspected entry/chunk double-bundling was NOT reproduced: the baseline entry is only 3,719 bytes, and the local baseline serialized upload is below 25 MiB. Thus the parent's API rejection is not fully explained by the available local evidence. No claim of reproduced server enforcement or guaranteed live acceptance is made. The actual measured reduction provides substantial additional headroom.

## Actual upload sizes

25 MiB = 26,214,400 bytes.

| Measurement | Assigned baseline | Final candidate |
| --- | ---: | ---: |
| Attached modules (excluding entry) | 335 | 335 |
| Total code modules (including entry) | 336 | 336 |
| Sum of module bodies | 14,185,540 bytes | 8,059,858 bytes |
| Main entry | 3,719 bytes | 3,187 bytes |
| Entire serialized multipart upload | 14,252,143 bytes | 8,126,461 bytes |
| Headroom including multipart overhead | 11,962,257 bytes | 18,087,939 bytes |

Final upload is about 7.75 MiB, a 43% reduction. Both baseline/candidate payloads retained. Baseline table matches the reported 335 / 13849.43 KiB. Wrangler randomizes temporary entry names and multipart boundaries; repeated serialization can differ slightly in byte count/checksum without source changes. An additional serialization from the extracted final tar is recorded in packaged-bundle-size.json.

Evidence directory: /home/hev/workspace/orders/luxir-docs-sizefix-artifacts/.
- baseline-worker.bundle, baseline-bundle.log, bundle-sizes.json.
- final-worker.bundle, final-bundle.log, final-bundle-size.json (final payload SHA256 d1effb2b958ba28348db411c1b7bc43bffbf48a1e08f4eada1fd1959d3bd5a62).
- packaged-worker.bundle, packaged-bundle.log, packaged-bundle-size.json: rebuilt upload from the extracted exact tar, no source config required.
- measure-bundle.py parses the real multipart payload, reads metadata.main_module, counts all module bodies and checks whole payload against 25 MiB.
- reproduce.sh records build, bundling, behavior checks, marker generation, tar and checksum. Copy its helper scripts to a fresh SIZEFIX_EVIDENCE_DIR and run from the final source root; no credentials required.

Core reproduction from site/:

```sh
env -u PUBLIC_DOCS_SLOP_SCORE PUBLIC_PREVIEW_NAV=1 PUBLIC_DOCS_LUXIR=1 pnpm run build
pnpm exec wrangler pages functions build --build-output-directory dist --outfile /absolute/new-worker.bundle
python3 /home/hev/workspace/orders/luxir-docs-sizefix-artifacts/measure-bundle.py /absolute/new-worker.bundle
```

For parent: extract the tar into a new directory and use the same project's Wrangler 4.95 Pages deploy path as before, branch v0.8, commit hash e7f93576d3574b8a78a6668308fee5fa375cc07b. No --no-bundle requirement. This report does not authorize publication of a release or production-branch deployment.

## Validation

All final relevant checks pass:
- Preview build from committed source; default build. Node v26.8.1, pnpm 9.15.9, Wrangler 4.95.0.
- Existing site Node tests: 33/33. Maintained marker tests: 3/3; final marker stamped/verified.
- Preview acceptance: Pro Luxir picker/corpus and CE isolation preserved. Default acceptance: Luxir stays hidden. No released store catalog change.
- Preview links: 8,440 internal links/anchors across 161 pages. Default links: 8,477 across 159 pages.
- Strict digest verification: 448 anchors.
- Direct execution of the actual minified CE and Pro Ask modules: GET suggestions and POST keyword retrieval return 200 with six results each, correctly edition-scoped; CE results contain no Luxir. Legacy GET/POST preserve 307 redirect, selected edition, resource and remaining query parameters. Same checks pass from extracted tar (packaged-ask.log). No external model call or key used; AI upstream behavior is not exercised.
- Extracted tar has no Luxir slop badge; route include/exclude sets equal the old artifact. An initial order-sensitive route comparison failed because directory enumeration reordered equivalent exclusions; sorted-set comparison passes. An initial legacy handler assertion expected 200; corrected to the source's documented 307 contract. These required no source fixes.
- Initial extra check after digest verification saw default dist (digest verify rebuilds it); exact packaged tar was independently extracted/checked and site/dist restored from it. The tar/checksum did not change.
- git diff --check clean; worktree clean.

No Rust builds, browser, agents, credentials, Pangram, deployment, push or merge. No API/client generation needed for this config-only change. Released and v0.8 edge docs were retrieved via curl into released-docs.html / edge-docs.html; relevant docs context remains the preview contract and edition Ask/query/store docs from assigned source. Train resolved with maintained scripts/workspace-semver-branch.sh: v0.8.
