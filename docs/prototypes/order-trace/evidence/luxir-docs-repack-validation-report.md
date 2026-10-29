# PASS — independent Luxir repack artifact validation

Run: `run-e74ca34c97bf1bea599bf573aa19a082`.
Exact assigned candidate: `bf6e69893cbdc1deff669c7c4fa94ae3f2e7eaef` (also checkout HEAD).
Artifact: `/home/hev/workspace/orders/luxir-docs-repacked-artifacts/preview.tar.gz`.
Independently calculated SHA-256: `0a0447daa6f3b08dd0dac8c4cfe0e148dc4fb20e768352525cbcd606aa82870b`.

Read `/home/hev/workspace/orders/luxir-docs-repack-report.md` and the prior parity validation. Independently inspected the actual new archive's 571 files in memory using a separate HTML parser; did not rely on the producer's proof JSON or extracted directory.

Passing checks:

- Pro Quickstart has the three shared headings exactly: “1. Sign up for a 30-day trial”, “2. Install in your AWS account”, “3. Switch your client”. None is inside a store-only wrapper. Rendered Python (`AsyncHevlayer`), Go (`github.com/hev/layer-go`), and TypeScript (`Hevlayer` from `hevlayer`) client setup is shared, with gateway URL/key configuration.
- AWS installer gap is inside the Luxir-only `aws-installer` wrapper with an actual nested `aside.callout`. It explains that the installer requires a Turbopuffer key and has no Luxir option, and links to dedicated Install setup. The Quickstart anchor `luxir-first-write-and-query` exists.
- Python response warning appears only on Pro Write (`dist/docs/pro/api/write/index.html`), exactly once, inside the Luxir-only `python-write-response` wrapper and nested `aside.callout`. It names missing status/message/billing, validation after the write has committed, Go/TypeScript/HTTP alternatives, and the instruction against blind retries. No repetition on Quickstart, Query, or other Pro pages.
- All 53 Pro HTML pages have exactly one Luxir picker button each: 53 total.
- All 55 shipped CE HTML/text/JSON files contain zero case-insensitive Luxir mentions.
- All four legacy Luxir URL variants redirect with 301 to `/docs/pro/quickstart/#luxir-first-write-and-query`.
- Packaged marker exactly matches schema `hevlayer.docs-preview/v2`, branch `v0.8`, repository `hev/layer-pro`, and the assigned full source SHA.
- Original theme remains restored: Header.astro, Footer.astro, Base.astro and global.css git blobs at the assigned candidate equal baseline `4566e906`. All five packaged CSS assets (paths and bytes) equal the previously theme-validated `305dae891b41bcb74986143197e67d84671a0e22ced52be03d70e913006cbd4c` archive. No Oktoberfest/fixed-theme archive filenames.

The new archive resolves the previous artifact's missing-Luxir failure. Prior source product-intent/theme review stands; no broader source review or builds were repeated. PASS applies only to this exact artifact and candidate. Mini browser/visual validation is separate and is not claimed here.

Only this report was written. No source/dist edits, builds, generators, credentials, browser, agents, push or deployment. Checkout was clean during inspection.
