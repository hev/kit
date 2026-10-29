# Luxir docs correction

Run `run-56479d3afdaecf40f525099b0edd84ca`; source branch `luxir-docs-deploy-20261023` (PR1033 lineage). Assigned base `e7f93576d3574b8a78a6668308fee5fa375cc07b`. Final committed source: e60407a6e8b9e79a4e24a1f5960db48463d2be8e. Ready for parent independent validation, Mini interaction checks and then parent preview deployment. No deployment performed.

## Delivered shape

The full Layer API is the contract; Turbopuffer compatibility is a subset. CE and Pro share Layer client installation/setup. Customer snippets use `hevlayer.AsyncHevlayer`, TypeScript `Hevlayer`, `github.com/hev/layer-go`, or HTTP. Historical generated Go type/method names remain intact where they are the correct contract; no Turbopuffer SDK import/install/constructor remains in customer examples.

Luxir is a backend choice, labeled **Luxir**, on every rendered Pro preview page, including backend-independent pages. `pageStores()` retains the edition registry instead of narrowing navigation state from manifest references. Existing prepaint/localStorage and keyboard behavior remain; backend-scoped TOC entries now follow visible content; the selector no longer disappears and resets the active backend on neutral pages. CE retains its existing stores. No-JS output hides inactive controls and shows all backend blocks with explicit backend labels. Browser execution is deliberately left to Mini; `site/scripts/luxir-picker.acceptance.mjs` provides exhaustive persisted-navigation, keyboard, CE round-trip and no-JS checks.

Setup lives in Install/VectorStores/Quickstart; supported writes, ranks, filters, schema/IDs and feature-local gaps live in their standard references. The standalone `luxir.mdx` is removed from navigation, search and text corpus. Preview-only Pages 301 rules redirect both `/docs/luxir[/]` and `/docs/pro/luxir[/]` to `/docs/pro/quickstart/#luxir-first-write-and-query`. No CE alias is introduced. All links point to standard pages. The generated store matrix remains unchanged and explicitly identifies its missing preview column. The Astro server-graph minification fix from the assigned base is preserved.

## Runtime and SDK findings

Audited runtime: clean `/home/hev/workspace/orders/luxir-staging-20261023` at `4f1da68d4d2c9a25987f01eb97216c43513aeadd`. The owned native demo used its existing Pro gateway binary with pinned Luxir 0.1.0, synthetic hand-set vectors, loopback confinement, telemetry off, an allowlisted credential-free environment and no shared services. `native/manifest.json` records binary hashes and exact source; `native/result.json` records cooperative stop and **zero children remaining**.

Two real SDK gaps are disclosed beside affected code tabs:

- Python write/delete sends the correct request and the mutation commits, but response parsing fails because Luxir returns `rows_affected` without required `status`, `message`, `billing`. The docs explicitly mark these Python tabs as request-contract examples rather than successful runnable writes, direct writes to TypeScript/Go/HTTP, and warn against blind retry after validation failure. Python filtered ANN and BM25 examples passed after seeding.
- Go typed `QueryNamespace` serializes its zero-value `collapse` struct, producing HTTP 400: `` `collapse.by` is required: an attribute or a list ``. Luxir Go examples use the verified **Layer** raw-query method `QueryTurbopufferNamespace` on the compatible v1 query route. Those exact write/query snippets compiled and ran successfully. This remains a Layer SDK, not the Turbopuffer SDK.

No Rust, gateway, OpenAPI or generated SDK file was changed. These runtime/SDK fixes are separate follow-up work; docs do not conceal them or promise a working unsupported path.

## Coverage inventory

Evidence abbreviations: **A** = runtime `crates/vectorstore-core/src/luxir/mod.rs` (`coverage` 56–86, schema 89+, filters 242+, `top_docs` 358+, `fusion` 413+, `wire` 646+, `write` 725+, adapter methods 805–970); **G** = runtime `apps/layer-gateway/src/`; **SDK** = this candidate's `clients/python/src/hevlayer/{client,models}.py`, `clients/typescript/src/{client,models}.ts`, `clients/go/{client,models}.go`. **Live** = retained correction-artifact native SDK logs. Source-reviewed composition is distinguished from runtime acceptance below.

| Page / sections | Luxir applicability | Evidence |
| --- | --- | --- |
| Install / dedicated install; VectorStores / Connection, Luxir | Default-off Pro feature, dedicated mode, private engine origin, explicit inbound keys/open; matching chart/operator readiness separately required | A `new`; G config/store resolution; runtime dedicated demo supervisor |
| Quickstart / first write and query | Inline Layer setup, schema write, filtered ANN; Python response gap and Go raw-query method stated | SDK; Live Python/TS/Go |
| Document model / schema and IDs | Restricted names, string IDs, explicit finite fixed-size vectors, persisted schema, replacement semantics | A schema/write/wire; Live replacement |
| Concepts / wire, scatter/gather, document cache | Full Layer contract; no Luxir hash-shard init/backfill or origin fetch fallback | A fetch/scan/head; G routes/init, shards |
| Guarantees / commitments; Failure modes / Read | No Luxir watermark, complete origin backup or cold-cache origin fallback; independent Layer services remain distinct | A scan/fetch/head; G consistency, snapshots |
| API introduction / Install, Auth, failures, operations | Common CE/Pro generated Layer setup; explicit inbound auth; existence of SDK methods does not imply backend parity | SDK constructors; G auth/config |
| API write / upsert/delete | Full replacement, explicit schema, string IDs; submitted counts and immediate commits; Python response gap local | A write; SDK models; Live |
| API write / patches, conditions, columns; Stage | Patch/filter/conditional/column/Arrow/numeric-ID gaps; cache chunk staging remains Layer service | A keys/write/patch/delete_by_filter/import; G routes/pipeline |
| API query / Stable reads, Query by ID | No watermark, cursor/search_after, nearest-ID/vector fetch | A top_docs/fetch_vector/head; G routes/query |
| API query / Rank expressions | Native ANN/BM25; bounded string ID asc/desc; scalar Eq/In, numeric ranges, nonempty And/Or depth 16; no negation/null/bool/array/datetime/analyzed equality/coercion; projection limits | A top_docs/scalar_filter/schema validation; Live ANN/BM25/ID/filter |
| API query / Batch query, facets | Native RRF over simple ANN/BM25 legs; facets only over fused candidates. Unfused batch/general aggregation unsupported | A fusion/multi_ranked_query/facet; prior native fused-facets test log |
| API query / HybridText, Auto | Explicit gateway capability rejection, distinct from native RRF | G routes/query:100+, A capabilities |
| API query / Collapse, Counting, Fetch | Grouping is Layer composition, not blanket store rejection; no exhaustive origin count/cold-cache fetch guarantee; composition not live-accepted here | G routes/collapse, fetch, scans; A fetch/scan |
| API scans / ID, count, values, text/hybrid/radius, Sources | No complete origin scan_page/count/facet/cursor contract; bounded ID query is distinct; existing snapshot-backed work remains separate | A scan_page/facet; G routes/scans |
| API namespace metadata / Request, layer block, field stats, listing | Metadata/head, watermark, namespace listing unsupported; schema read/deletion supported; stats refresh depends on scans/snapshots | A wire/head; G routes/metadata |
| API embed / Example, Embed query, Auto, chunking | Gateway capability checks reject schema embed and Embed because adapter does not persist profiles; external provider config does not bypass | G routes/embed_wire:205+, 624+; A capabilities/schema |
| API search / Request, Collapse, Stores | Enhanced /search rejected for this backend; explicit /query ranks remain supported | G routes/search:1236+; A capabilities |
| API federated query / fan-out, consistency, merge | Layer composition can use supported legs; no shared Luxir stable cut, origin hydration or full accepted merge guarantee | G routes/federated_query; A ranked_query/fetch/head; source-reviewed, not live-exercised |
| API upstream features / branch, shard, pin, recall | Those Turbopuffer passthrough shapes unsupported; not a restriction on the whole Layer API | A wire/write allowlists; G routes/branch, init |
| API warm cache / hint, Layer warm | No native hint or origin scan-backfill; independent existing snapshot/blob warming is separate | A hint_cache_warm/scan_page; G routes/turbopuffer |
| API snapshots / policy, manual | Policy/orchestration/history are Layer services; complete new Luxir origin snapshot/export is not established | G snapshots, routes/snapshot_policy/history; A scan/head |
| API checkpoints / Create | Labels existing durable S3 snapshots; no engine scan is performed; no assertion of a recoverable Luxir backup | G routes/checkpoints |
| API record versions / Stage/restore, History | Explicit pgvector-only route guard; all Luxir versioning/activation endpoints unsupported | G routes/record_versions:11–24; A write allowlist |
| API blobs / storage | No native engine bytes; configured Layer S3 stores blobs; separate from document fetch | A CAPABILITIES blobs NONE; G routes/blobs |
| API search history / Storage | Backend-neutral Layer cache/S3 service; does not enable enhanced search | G routes/history, history |
| API keys / Routes | Layer key service; explicit inbound config, no derivation from unauthenticated Luxir engine | G routes/keys, auth; runtime config |
| API warehouses / Connection | Backend-neutral source connections/control plane; no inference of successful target ingestion | G routes/warehouses; pipeline target write paths |
| API pipelines / extraction, staged writes, Target | Extraction/staging/queue are Layer services; target schema/ID/replacement rules still apply; no accepted full Luxir pipeline flow claimed | G routes/pipeline; A write/patch/scan; source review |
| API agents / Request | Layer orchestration; backend retrieval dependencies remain; no blanket route rejection or accepted full Luxir agent flow claimed | G routes/agents, agent; A capabilities |
| API MCP / Tools, Aggregate, Paging | Schema discovery calls unsupported head_namespace; not incorrectly described as /search; fetch, scans, cursor gaps local | G mcp:638–663, 980+, 2036+; A head/fetch/scan |
| API Lattice / end-to-end | Embedding compute is Layer-side; schema/Embed capability guard prevents this Luxir end-to-end example | G routes/embed_wire; A capabilities |
| API capabilities / Request/response | Backend-neutral declared report without engine I/O; remains separately feature-gated, not silently enabled | G routes/capabilities; A CAPABILITIES |
| API response headers | Timing/tracing backend-neutral; no Luxir stable watermark or cursor promise; cache header is not origin-fetch support | G routes/query/fetch, consistency; A head/top_docs |
| API license | Local gateway license projection, backend-neutral; no Luxir warning invented | G routes/license |
| API data supply | Navigation to store/source resources, backend-neutral; no runtime support promise added | Maintained page links; G vectorstores/warehouses |
| Indexes / field stats, embedding, snapshots; Functions / Selection, Writeback | Operator skips unsupported Luxir lifecycle/discovery; origin scan, patch and embedding gaps adjacent to their examples | runtime operator store-kind handling; G routes/udf, pipeline, embed_wire; A scan/patch |

All 26 API source pages were audited, including separately gated capabilities content. Independent services were not labeled wholesale unsupported merely because the native adapter lacks a similarly named feature.

## Validation

- 36/36 focused site Node tests; maintained SDK syntax checks: 73 Python snippets parsed, 66 Layer method call signatures; 68 TypeScript snippets parsed, 63 calls resolved to generated methods. These are syntax/method checks, not typechecking every pre-existing snippet's arbitrary context.
- Exact new Go write/query examples compile against the generated local client and ran on native Luxir. The TypeScript client compiles with its maintained tsconfig; exact docs TS write/filter ANN/BM25/delete ran, plus replacement, ID ordering and numeric/patch refusals. Python exact query/BM25 examples ran; write/delete response-validation failures were reproduced after successful mutation and are documented.
- Default and preview Astro builds pass. Default public and all CE HTML/text/corpora have no Luxir; released-store metadata unchanged. Preview has exactly 53 rendered Pro docs pages checked for the Luxir picker/prepaint registry and no-JS fallback. No Turbopuffer SDK code or enhancement-only framing found in rendered code/docs.
- Default links: 8,486 links/anchors across 159 rendered pages. Final preview links: 8,350 links/anchors across 159 rendered pages. All internal docs links/anchors checked.
- Digest corpus/build, status, assemble, strict verify pass: 445 anchors. Summaries retained/reviewed from the prior digest and updated locally for changed sections; no remote model or secret used. Final preview is built after digest verification because verification rebuilds default dist.
- Preview flags are exactly `PUBLIC_PREVIEW_NAV=1 PUBLIC_DOCS_LUXIR=1`; `PUBLIC_DOCS_SLOP_SCORE` is unset. No Pangram calls or score mutation. Source marker schema `hevlayer.docs-preview/v2`, repository `hev/layer-pro`, branch `v0.8`, exact final SHA verified.
- Released https://hevlayer.com/docs and edge https://v0.8.x.hevlayer.com/docs were read via curl and saved in the evidence directory. Train resolved by maintained `scripts/workspace-semver-branch.sh` from assigned Cargo workspace version: `v0.8`; relevant source sections are inventoried above.

## Final artifact

Evidence directory: `/home/hev/workspace/orders/luxir-docs-correction-artifacts/`.

Tar: /home/hev/workspace/orders/luxir-docs-correction-artifacts/site-dist-e60407a6.tar.gz

SHA256: fb19e44acd3e2b287848212f024cd7fe077d6c710dc3d926376f8c0987fe2687

Actual Wrangler serialized multipart upload: 8,345,205 bytes; limit 26,214,400 bytes (25 MiB). Details and payload hash in `final-bundle-size.json`; built with the prior helper and the same Wrangler functions-bundle path used by Pages deploy. This measures the entry, all additional modules and multipart metadata, not merely disk size or the module-table subtotal. Final tar extracted and rechecked before handoff. Its complete relative-file/hash set matches `site/dist`; all 53 Pro selectors and CE isolation pass again. A second actual Wrangler serialization from the extracted tar also passes (`packaged-bundle-size.json`). Built CE/Pro Ask suggestions/keyword retrieval and legacy 307 redirects pass both before and after packaging, with no external model call.

## Remaining validation boundaries

Mini browser persistence/keyboard/no-JS interaction, actual Cloudflare redirect execution and deployment/server acceptance remain for the parent. No push, merge, deployment, secret access, browser, sub-agent, Rust build, generated SDK change, shared runtime mutation or external paid service occurred. Native RRF/fused-facet evidence is the prior validated test, not a new Rust run. Other backend-neutral services/compositions were source-audited, not provisioned or end-to-end accepted in this isolated demo. Python write responses and the typed Go query issue remain real runtime/SDK follow-ups; the documented TypeScript/Go-raw/HTTP path avoids presenting those failures as successful examples.


## Reproduction and handoff

From the committed checkout, run `cd site`, `pnpm test`, `python3 scripts/layer-sdk-examples.py`, `node scripts/layer-sdk-examples.mjs`. Build default with the preview/Luxir/slop flags unset, then run `node scripts/luxir-docs.acceptance.mjs` and `pnpm run test:links`. Build preview with `PUBLIC_PREVIEW_NAV=1 PUBLIC_DOCS_LUXIR=1` and slop unset, then run `node scripts/luxir-docs.acceptance.mjs --preview` and `pnpm run test:links`. Do not run digest verification after packaging without rebuilding/restoring preview: verification rebuilds default output.

On Mini, serve the extracted final artifact at an owned origin and run `node site/scripts/luxir-picker.acceptance.mjs <origin>` from the committed checkout (the script inventories `site/dist/docs/pro`). This browser test was maintained but deliberately not executed here. Parent independently checks the committed source, report, marker and tar checksum before any authorized deployment to the v0.8 preview lane.
