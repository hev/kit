# PASS — independent narrowly scoped size-fix revalidation

Run: `run-4b0072abc2e81b8def19ff19572d0dd1`  
Date: 2026-10-23 UTC  
Exact assigned source and inspected HEAD: `e7f93576d3574b8a78a6668308fee5fa375cc07b`  
Prior independently passed source: `4559d0dd623ce87f0a8ead29a3ff2e3f3ae5a8a8`

**PASS for the scoped local source/artifact gates. No blocking local findings. Cloudflare upload acceptance remains unproven and requires the parent’s live retry.**

Read the prior independent report `/home/hev/workspace/orders/luxir-docs-deploy-validation-report.md`, the sizefix report `/home/hev/workspace/orders/luxir-docs-sizefix-report.md`, and its fixture/reproduction/measurement scripts. This supplements the prior broader source PASS; it does not repeat or supersede that review.

## Source gate — PASS

The entire prior-PASS-to-candidate diff is ten added lines in `site/astro.config.mjs`. An inline Astro integration sets `build.minify: "esbuild"` through `astro:build:setup` only when target is server. This changes server graph serialization without changing route definitions, Ask implementation, docs content, edition flags, release catalog, dependencies, workflow or deployment configuration. Behavior preservation is supported by the extracted-artifact checks below. `git diff --check 4559d0dd HEAD` passes; source worktree is clean. No full rebuild was performed, as requested.

Preview-only Luxir and the existing no-Pangram configuration are unchanged from the prior PASS. The artifact positively exercises Pro preview visibility and CE exclusion. Default-build isolation evidence is inherited from the prior review and sizefix report, not independently rebuilt here.

## Identity gate — PASS

Independently SHA256-hashed `/home/hev/workspace/orders/luxir-docs-sizefix-artifacts/site-dist-e7f93576.tar.gz`:

`3312c4700b236a5b0ae79b7ebfafec4d126fc73f7e2a9ae5db589a2cd2c0d2a6`

The exact tar was extracted into `/tmp/luxir-sizefix-validation-4b0072/dist`. Maintained marker verification passes; an additional strict object-equality check confirms the marker has exactly:

```json
{"schema":"hevlayer.docs-preview/v2","branch":"v0.8","sourceRepository":"hev/layer-pro","sourceSha":"e7f93576d3574b8a78a6668308fee5fa375cc07b"}
```

## Actual multipart measurement gate — PASS locally

Independently parsed `final-worker.bundle` with Python’s MIME multipart parser, read `metadata.main_module`, counted filename-bearing module parts and measured decoded module bodies separately from the complete serialized payload. Did not rely on the supplied measurement script or directory size.

| Measurement | Independent result |
| --- | ---: |
| Complete multipart bytes | 8,126,461 |
| Modules including entry | 336 |
| Sum of module bodies | 8,059,858 bytes |
| Entry body | 3,187 bytes |
| 25 MiB threshold | 26,214,400 bytes |
| Headroom | 18,087,939 bytes |

Payload SHA256: `d1effb2b958ba28348db411c1b7bc43bffbf48a1e08f4eada1fd1959d3bd5a62`.
Independent machine-readable measurement: `/tmp/luxir-sizefix-validation-4b0072/independent-multipart.json`.

This measures the supplied final Wrangler multipart, not a newly generated upload. The sizefix report also records serialization from its extracted tar. The reported local baseline of 14,252,143 bytes was itself below 25 MiB and therefore did not explain Cloudflare’s reported 25 MiB rejection. The candidate provides substantial measured headroom, but no local number proves platform acceptance. Only the parent’s live upload retry can establish that gate.

## Extracted-artifact behavior and isolation gates — PASS

Executed the supplied `check-built-ask.mjs` directly against this fresh extraction, using the prior validation’s private dependency copy through a temporary symlink:

- CE and Pro built minified Ask GET suggestions and POST keyword retrieval return 200, six retrieval results each; every result has the correct edition URL and CE responses contain no Luxir.
- Legacy GET/POST return 307 with the expected selected edition, resource and remaining query parameters preserved.
- Maintained `luxir-docs.acceptance.mjs --preview` passes: CE isolation, Pro picker/corpus visibility and preserved Postgres documentation. Its auxiliary CE corpus fixture was copied from the prior independent validation; rendered checks and Ask execution consume the newly extracted tar.
- Maintained docs-links acceptance passes: 8,440 internal links/anchors across 161 rendered docs pages.
- Independent scan finds no Luxir in all 55 CE HTML/text/JSON files, and no slop-score badge markers in all 168 HTML pages.
- All HTML route paths and normalized `_routes.json` values equal the prior independently verified artifact. Route-array ordering is ignored when comparing sets.

No browser, model upstream call, secrets, Pangram request, build, source mutation, commit, push, deployment, release publication or agents. Only the requested report and temporary extraction/check files were written. Parent retains the live Cloudflare upload/marker gate and Mini browser validation.
