# Luxir product context correction

Run identity: `run-9e5e076f83db03f3d387aa5131578573`.
Assigned source: `e60407a6e8b9e79a4e24a1f5960db48463d2be8e`.
Final source: `2f903686f9ad6a663d8221575e7e4fb823d33427`.
Branch: `luxir-docs-deploy-20261023`, PR1033 lineage. Local commit only; clean working tree. No push, merge or deployment.

Root AGENTS.md now carries Adam's two-goal Product frame: compatible migration to win adoption, and Layer gateway enhancements across both Turbopuffer and Luxir, including agentic search. Full Layer API through Layer SDKs; wire compatibility is the entry point, not ceiling. Feature support follows actual gateway composition, with missing integration documented as a current gap rather than product scope.

Diff is limited to AGENTS.md, nine prose pages (API introduction, agents, embed, federated query, pipelines, query, scans, search; concepts), and the committed digest: 11 files, 132 insertions / 102 deletions. API introduction and concepts preserve the compatibility migration story and explain gateway enhancements. Named Luxir framing stays in preview-gated Pro notes. Feature-local search/embedding/Auto/HybridText notes describe current integration checks; agent guidance distinguishes orchestration from retrieval dependencies. Removed customer-facing source-review/exercise status in collapse/federation/pipeline guidance. No new page, runtime, generated client, spec, selector or SDK example changes.

The prior correction report and diff were reviewed. LYR-324 (Python successful Luxir mutation response missing required fields) and LYR-325 (Go typed QueryNamespace serializes empty collapse) remain implementation gaps. Existing local Python request-contract notes and Go Layer raw-query workaround remain unchanged. No Turbopuffer SDK introduced. Prior native acceptance evidence is retained, not rerun.

Evidence for the feature wording: candidate `apps/layer-gateway/src/routes/search.rs:1236` requires WireFeature::Search before execution; query.rs dispatches native-wire queries separately from HybridText/Auto and checks their prerequisites. Embedding preparation/profile persistence is in routes/embed_wire.rs; audited runtime Luxir adapter at `4f1da68d4d2c9a25987f01eb97216c43513aeadd`, `crates/vectorstore-core/src/luxir/mod.rs:56-86`, declares supported/approximate explicit ranks but defaults other wire features to unsupported and declares schema embedding unsupported. Agent orchestration remains a gateway feature, not an inferred native-engine capability. This pass used source checks plus prior retained reports; no new engine/runtime acceptance is claimed. Released/edge docs evidence remains in the prior correction artifacts (released-docs.html, edge-docs.html), train v0.8 resolved previously with the maintained helper.

Validation evidence is in `/home/hev/workspace/orders/luxir-docs-context-artifacts/`:

- Scoped site tests: 36/36 pass. First run caught ungated Luxir framing; corrected using existing Pro StoreNote gates and rerun green.
- Digest required corpus/build, status, assemble and strict verify pass: four retained shard inputs/distillations reused and framing summaries/context refreshed locally; zero pending/stale shards, 445 verified anchors. No model/credential call. Retained shard files remain in site/.hev-ask/shards.
- Explicit default build and isolation acceptance pass; 8,489 internal links/anchors across 159 pages pass.
- Final preview build uses exactly PUBLIC_PREVIEW_NAV=1 PUBLIC_DOCS_LUXIR=1 with PUBLIC_DOCS_SLOP_SCORE unset. 53/53 Pro selectors, no-JS fallback, SDK-only imports, redirects and CE isolation pass; 8,353 links/anchors across 159 pages pass.
- An initial default link check against output left by digest verification failed on a legacy alias anchor. An explicit default Astro rebuild resolved it; final default and preview checks both pass. No source workaround or hidden failure.
- Built CE/Pro Ask suggestions and keyword retrieval plus legacy 307 redirects pass using the prior retained helper. No external model call.
- git diff --check passes. Final marker verified: schema hevlayer.docs-preview/v2, branch v0.8, sourceRepository hev/layer-pro, exact final sourceSha above.
- Packaged tar contents independently hashed against every file in site/dist; exact match (package-check.json).

Tar: `/home/hev/workspace/orders/luxir-docs-context-artifacts/site-dist-2f903686.tar.gz`

SHA256: `f9bbc2bd7a4d4b9e51a28371927bfe7e85475da4beee35e457a53c6ee80900cf`

Actual Wrangler Pages functions bundle serialized with `pnpm exec wrangler pages functions build --build-output-directory dist --outfile .../final-worker.bundle` and measured by prior `luxir-docs-correction-artifacts/measure-bundle.py`: **8,350,878 bytes**, 332 module parts, under 26,214,400 bytes. Whole multipart payload, not disk/module subtotal. Measurement JSON and bundle retained. No network deployment or credentials required.

No credentials, browser, agents, Rust build or native-engine retesting used. Pangram not called, per explicit no-credentials/slop-unset scope. Browser interaction, independent validation and deployment remain with parent. Completed within the 900-second bound.
