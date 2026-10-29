# Independent validation: PASS

Run: run-06fc1c36e0d016ae0c8844b0d25010ad.
Exact source: 2f903686f9ad6a663d8221575e7e4fb823d33427 (checkout HEAD verified).
Compared actual source against previously deployed e7f93576d3574b8a78a6668308fee5fa375cc07b, including correction e60407a6 and subsequent context changes. Reviewed both worker reports and retained native evidence; conclusions below include independent source/artifact checks, not report-only acceptance.

Tar: /home/hev/workspace/orders/luxir-docs-context-artifacts/site-dist-2f903686.tar.gz
Independently calculated SHA256: f9bbc2bd7a4d4b9e51a28371927bfe7e85475da4beee35e457a53c6ee80900cf

## Customer-facing findings

- AGENTS.md explicitly preserves both goals: compatible migration to win adoption, and gateway enhancements for BOTH Turbopuffer and Luxir, including agentic search. It states the full Layer API is exposed through Layer SDKs and requires composition/capability/dependency assessment. Missing integration is a current gap, not a product boundary.
- API introduction supplies common Layer Python/Go/TypeScript installation and construction for CE and Pro, explains the compatible subset and full API, and points to standard API references. Concepts preserves migration and enhancement intent. The correction removes the enhancement-only contract framing.
- Luxir is integrated into standard Install, VectorStores, Quickstart, Write and Query pages. Exactly 53 packaged Pro content pages have a Luxir tab and prepaint registry containing turbopuffer/luxir. The shared pageStores function retains the edition registry on neutral pages, preventing navigation from resetting selection. Postgres remains in the CE registry by existing edition policy; Luxir follows the same selector mechanism in Pro.
- Feature-local notes cover query ranks/filters/cursors/nearest-ID, writes, embedding, enhanced search, scans, agents, MCP, metadata, snapshots and other affected APIs. Search/Auto/HybridText/embed wording identifies current gateway integration/capability failures. Agents distinguish orchestration from retrieval dependencies. Snapshots/checkpoints, pipelines, blobs, keys and history retain their independent Layer-service meaning; there is no blanket inference of unsupported gateway services from native engine limitations. Supported direct ANN/BM25/manual-vector operations are useful without inventing full parity.
- LYR-324 Python mutation-response decoding is prominently disclosed beside Quickstart and Write examples: mutation may commit before validation failure, Python tabs are request-contract examples rather than a successful runnable write path, use Go/TypeScript/HTTP for writes, and do not blindly retry. Python reads remain usable. LYR-325 typed Go empty-collapse issue is disclosed beside the query example; QueryTurbopufferNamespace is correctly a generated Layer Go method, with Layer import and raw compatible-route workaround.
- No Turbopuffer SDK import/install/constructor was found in served HTML code blocks. Independent scan included Go upstream imports and constructors as well as Python/TypeScript patterns; legitimate Layer raw method/type names are allowed. Source Python checker also scans the maintained corpus for upstream SDK usage.
- Old /docs/pro/luxir redirects to /docs/pro/quickstart/#luxir-first-write-and-query (301 rules, including slash variant in packaged redirects). Standalone Luxir page is absent. Internal anchor destination exists and links pass.

## Independently rerun gates

- All 36 site Node tests PASS using private temporary dependencies reused from the correction checkout. No install, source modification or build was needed.
- Python: 73 snippets parse, 66 generated Layer method signatures checked against exact candidate source.
- TypeScript: 68 snippets parse, 63 calls resolve to generated Layer methods. These checks establish syntax/method contracts, not complete runtime/type validation for every contextual example.
- Packaged preview acceptance PASS: exactly 53 Pro selectors, prepaint registries, no-JS fallback, CE isolation, redirects and SDK examples. Independent direct scan also checked every shipped CE HTML/text/JSON for Luxir and every docs HTML code block for upstream client code. No Pangram score markup found.
- 8,353 internal docs links/anchors across 159 rendered pages PASS against extracted exact tar.
- Marker exactly matches schema hevlayer.docs-preview/v2, sourceRepository hev/layer-pro, branch v0.8, sourceSha 2f903686f9ad6a663d8221575e7e4fb823d33427.
- Independently parsed actual retained Wrangler multipart: 8,350,878 bytes, 332 module parts, under 26,214,400 bytes (25 MiB), 17,863,522 bytes headroom. SHA256 f88c825829073d834a67b54dd4fceaef925541fd6634161b50521ed662bbbf08. All 331 additional module payloads match exact extracted tar worker files byte-for-byte. The generated multipart entry is separate Wrangler bootstrap, not an on-disk tar module.
- git diff --check PASS; checkout remains clean; no runtime, spec or generated SDK files changed in candidate diff.

Initial verification harness limitations were resolved transparently: direct checkout full tests lacked node_modules; temporary copy with reused private dependencies and repository fixture symlinks passes 36/36. Packaged acceptance expects an untracked .hev-ask/editions/ce.json outside the tar; a temporary copy omitted only that unavailable corpus assertion, replacing it with an independent scan of the actual shipped CE HTML/text/JSON. Remaining packaged assertions pass. No candidate files were edited.

## Evidence boundaries and deployment handoff

Default build isolation is supported by retained final context default-build/default-acceptance/default-links logs, unchanged released-stores metadata, and independently rerun source gating tests over the whole corpus. The final preview tar's CE isolation is independently checked. A fresh default build was deliberately not repeated.

Retained actual native SDK evidence confirms Python committed mutations with response validation failures, Python ANN/BM25 reads, Go raw-query reads, and TypeScript write/filtered ANN/BM25/delete plus replacement and refusals. Reviewed native manifest/result (stopped, zero children remaining) and prior fused-facet evidence. No native engine or Rust retest, runtime modification or new parity claim.

Built Ask suggestions/keyword retrieval and legacy 307 redirect evidence is retained from the final context build, preserving the earlier server-graph minification fix. This is existing built-artifact evidence; no new live Ask evidence is claimed.

Source no-JS semantics hide inactive picker controls, reveal backend blocks and label them; JavaScript source reads shared localStorage before paint, synchronizes selected state, handles Home/End/arrow keys, and updates scoped TOC visibility. The maintained Mini acceptance script covers all-page persistence, keyboard selection, CE round-trip and no-JS. No GUI/browser was run on Oven. Mini must validate live interaction after deploy; Cloudflare redirect execution and deployed server acceptance remain external handoff gates. PASS here means product-intent/source/package readiness, not completed live deployment acceptance.

No source edits, commits, push, deployment, credentials, Pangram calls, browser, subagents, Rust build or runtime changes performed. Only this requested report and private /tmp validation files were written.
