# Stats and the bounded Layer latest-relation API

Kit base: ec9ccf1331846400a466028cff5bee7d443c7a43. Contract inspected at
Layer af8bfd9bf04fadf647114db00218295e7b9e5789 (tree
acad8121ba9e5cae1393cca9dbf17239bc9e0d21), especially
`site/src/content/docs/api/latest-relation.mdx`, the gateway route and core
latest/grouped types. The independent API report accepts a bounded functional
subset; full required Layer CI and complete Stats acceptance are separate.

## Implemented consumer seam

`internal/layer.Client.AggregateLatest` sends the typed Count/Sum and named
field/UTC-day/timestamp/model grouped wire to the configured namespace's
`POST /v2/namespaces/{namespace}/aggregate`. It is an internal reusable client,
not a new public endpoint or a switch of `/api/stats`. The caller supplies the
logical identity, ordering, filters, finite row/byte budgets and observed
consistency. Static physical-row aggregate support does not authorize latest
execution. Only an actual structured 422 `UnsupportedByStore` with feature
`latest_by` is classified by `LatestUnsupported`; 400/401/403/404/500,
budget, cursor and decoding failures are errors. The client never silently
retries against private namespaces or fetches raw rows as an error recovery.

The complete request is copied through JSON with exact numeric filter decoding
before execution. Endpoint, namespace, credential, transport, timeout and
context are captured for the operation. Each continuation preserves every
named plan (even a completed one), all scalar specs, filters and budgets. Only
pending plans' cursors advance. Completed plans' replayed pages are ignored;
scalar totals are retained once, checked for equality, never accumulated.
Named binding, totals, progress, duplicate keys, source completeness and metric
shapes are checked. Any failed page returns a nil result, discarding all prior
output. Integers use `json.Number`, including integers above 2^53. Cancellation
uses the existing request-bound client and HTTP deadlines. No global cursor,
result or tenant state is stored. `Executions` preserves source counters for
every request so continuation rescans cannot disappear from measurements.

## Complete Stats capability decision

The current decision is **projected scan for every complete Stats request**.
There is no speculative aggregate call ahead of fallback and no split scan.
This preserves PR107's Stats-first/paired-list behavior and its response shape.
A scalar-only or heatmap-only success does not make the whole response eligible.

| Consumer requirement | Accepted Layer API | Complete Stats decision |
| --- | --- | --- |
| Latest before mutable predicates | Explicit key/order and post-selection filter | Ordering alone is insufficient for all legacy rows |
| Winner normalization | Raw typed null/missing ordering | Kit decodes absent/null `end` as zero; raw null ordering differs from zero, including canonical ties; no normalization expression |
| Identity failure | Nonempty typed keys and physical IDs | Kit additionally rejects whitespace-only logical/physical IDs and requires string IDs; no proof of this whole-source invariant from aggregates |
| Scalar Count/Sum | Supported, exact integer decoding | Reusable client available; do not splice into Stats with another full scan |
| Model tokens/spend, UTC start-day charts | Literal dimensions and sums supported | Layer creates null/epoch groups for absent/zero start; Kit omits zero starts from charts while retaining them in totals; no per-plan filter; preserve fallback |
| Project table and 12 bins | No window-relative arithmetic bins or project-name normalization | Preserve grouping by `projectName`, repo representative, zero-filled 12 bins and window denominators locally |
| Prompt heatmap | Timestamp expansion counts instances; Monday=0 | Kit Sunday=0; IANA/DST mapping possible but coverage is not derivable from instance totals alone |
| Prompt coverage | No per-row nonempty-array conditional count | Incomplete arrays still count as one covered trace; no invented zero or count-from-prompt_count |
| Unfiltered facets | Plans share the filtered relation; no tool-array facet expansion | Preserve unfiltered latest archive facets and tool membership |
| Grade/role/instance/mark filters | No authorized HTTP grade join | Keep existing latest-eval join; never access unauthenticated internal namespaces |
| Tool/cell/summary/search and unknown future plans | Not a complete accepted consumer contract | Preserve current validation and fallback behavior |

Even an empty aggregate result does not certify empty unfiltered facets. An
aggregate probe followed by a projected scan for nonempty data doubles source
work and is deliberately absent. A future switch requires a complete plan,
authorized capability evidence, correct null/ID normalization, full facets,
coverage, project bins and parity tests before issuing its first source read.
Unsupported plans should choose fallback before execution; runtime fallback is
allowed only for genuine capability rejection. Auth, server and data failures
must remain visible, including failures after earlier successful pages.

## Cost and validation limits

Layer's `bounded_source_scan` transfers all nonvector source revisions to its
gateway, folds there, and repeats the entire scan for each continuation. This
is not backend-native latest aggregation, a snapshot, or a demonstrated latency
or billing improvement. Source page counts exclude some metadata/sharding work;
backend physical scans and billed bytes remain unmeasured. No new persistent
cache or analytics store is introduced.

The Go fake-HTTP tests validate consumer protocol behavior, not Layer's Rust
executor. The separately accepted core/HTTP/external probes establish that API
subset. Kit fixture tests cover complete Stats fallback parity, cancellation,
malformed identities, incomplete prompt arrays, DST, facets and request races.
Synthetic measurements use only fixtures; do not infer production performance.

Run `go test -race -count=1 ./internal/layer ./internal/serve` and
`node scripts/test-stats-path.cjs`. The existing opt-in fixture and Mini browser
procedure are in [stats-mini.md](stats-mini.md). Independent exact-candidate and
Mini visual acceptance remain coordinator work; preparing instructions is not
browser acceptance.

The retained synthetic run measured the supported partial-contract fixture at
3 aggregate HTTP requests, 24 mock source rows, 20,574 mock source bytes and
3,070 output bytes. This fixture has 8 revisions and rescans them three times.
The larger existing PR107 fixture's cold Stats request retained 3 source calls,
1 projected session scan, 1,323 returned source rows (including evals), 546,890
source bytes and 9,477 response bytes. These are different workloads and output
contracts, not an aggregate-versus-Stats speed comparison. The old list-then-
Stats baseline in that script predates PR107; its savings are not from this
change. All counts are synthetic decoded/application bytes, not billed bytes.
