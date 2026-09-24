# RFC 0003: The trace model — transcript-native capture and the namespace schema

Tracking issue: TBD

**Status: draft, first cut implemented and verified against the live wire
(`internal/trace`, `internal/layer`, `internal/index`, `hev index`,
`hev find`). Answers the schema, chunking and embedding open questions RFC
0002 left open; retires the raw-API-body Claude source. Closes
[#8](https://github.com/hev/kit/issues/8) and
[#1](https://github.com/hev/kit/issues/1).**

## Summary

Capture stops reading Claude Code's raw API bodies and reads the session
transcripts instead, normalizing every harness into one turn/block model
ported from [funes](https://github.com/huggingface/funes) (Apache-2.0).
Blocks are chunked in value tiers, each chunk carries a content-addressed
id, and chunks land as rows in a Layer namespace whose `text` column is
both full-text indexed and declared with an embedding model. Retrieval is
hybrid — a semantic leg and a BM25 leg, fused by reciprocal rank — and all
three of those happen in the store.

kit computes no vectors, runs no model, and carries no reranker.

What changes for the user: `hev index` puts the sessions they actually ran
into their own namespace, and `hev find "why did the turbopuffer preflight
fail"` returns the turn that says so — including when the turn never used
those words.

## Motivation

- **The best source on the machine is the one kit does not read.**
  `ClaudeRawBodiesDir` (`~/.hev/claude-code/raw-api-bodies`) captures wire
  bodies, which is why #8 reports Claude sessions as "mostly redacted":
  the policy is doing its job on a payload that is largely system prompt
  and headers. The harness meanwhile writes a complete, structured
  transcript per session to `~/.claude/projects/*.jsonl` and nothing reads
  it. On the factory host that is 382 sessions and 110 MB of unread
  history — the archive kit exists to keep, sitting next to a daemon that
  is not keeping it.
- **Every read-side feature is currently per-harness.** Codex arrives as
  `codex.rollout_jsonl.v0.130.0`, Claude as raw bodies, and there is no
  shape they share. `hev ls`, `hev trace`, the TUI, `hev find`, Map and
  evals each pay that tax again. One normalized model pays it once, and
  a new harness becomes one interface implementation.
- **Redaction is buying nothing and costing the archive.** #8's own
  resolution is to delete it. The trust boundary that matters is where
  the data lands, and RFC 0002 already moved that to a Layer the operator
  runs. A regex pass over your own transcripts on your own machine is not
  a security control; it is a lossy filter on the asset.
- **Content-addressing falls out of chunking.** #1 asks for a
  content-addressed envelope for checkout and replay. A chunk id that is
  a hash of its text gives idempotent re-index and dedup for free — the
  same property from the same primitive.
- **RFC 0002 stopped at the schema.** It settled the destination and left
  chunking granularity, filterable-vs-FTS-vs-vector, and embedding
  placement open. Those are this RFC.

## Decision

**Port the funes ingestion model; do not port its storage or inference.**

funes solved the half of this problem that is tedious and unglamorous —
turning four harnesses' transcript formats into one queryable shape — and
solved it under a license we can use. Its other half, a local Lance
dataset with pinned on-device embedding and client-side hybrid search, is
precisely what Layer is for. Taking the first and declining the second is
the whole design.

Ported (as Go, with attribution — see Vocabulary):

- **The turn/block model.** A transcript becomes `[]Turn`; each `Turn`
  carries `session_id`, `workdir`, `turn_uuid`, `parent_uuid`, `seq`,
  `ts`, `role`, `harness`, `source_path`, and typed `Block`s of
  `text` | `thinking` | `tool_use` | `tool_result`. Every parser produces
  this and everything downstream consumes only this.
- **`TraceSource` + `Unit`.** A source enumerates units cheaply (stat, no
  parse) and parses one on demand. A unit's `signature` is a change-stamp:
  unchanged units are skipped. This replaces the current per-file ledger
  scan with incremental indexing that survives a growing transcript.
- **Value tiers.** L1 `text` (and `thinking`), L2 `tool_use`, L3
  `tool_result`. Tier decides *when* a block is indexed, never whether —
  unknown block types fold into L1 so nothing is dropped. `tool_result`
  is the bulk of the bytes and the least of the value; tiering is what
  keeps a 110 MB corpus from being 110 MB of `cat` output.
- **Chunking.** ~1200 characters with 150 of overlap, split by code point,
  chunk id = hash of text. Overlap bounds what reassembly has to match
  when stitching cited turns back together.

  The content hash cuts both ways, and the second edge is easy to miss:
  **changing how a block renders orphans every row it already wrote.** A new
  rendering hashes to a new id, so the old rows are not overwritten — they
  persist beside the new ones and one turn comes back twice. Any change to
  `normalizeBlocks` is therefore a delete-then-reindex, or a namespace
  re-create, and never a plain re-run. Idempotence holds only while the
  rendering is fixed.

Not ported: Lance, the HF Hub path, ONNX/Accelerate inference, the local
reranker, RRF fusion, recency weighting. Layer's gateway already does
BM25/FTS and hybrid scans server-side; duplicating a retrieval stack
inside a capture daemon is how kit ends up maintaining a search engine
it does not own.

### Verified before written

Every claim below was exercised against the live wire before this RFC was
finalized, because both the embed-wire and native-query-embedding RFCs are
drafts and the gateway's `prefer: autoscaler` leg still returns 503. The
`prefer: native` leg is the one in play and it works today:

- `qwen/qwen3-embedding-0p6b`, `-4b` and `-8b` all accept a write and embed
  server-side (`embedding_tokens` / `embedding_ms` echoed on the response).
- A query sharing **no keywords** with the target chunk retrieved it first,
  which is the whole point of having vectors at all.
- Hybrid is a `multi_query` of an ANN leg and a BM25 leg with
  `rerank_by: ["RRF"]`, fused server-side into one result set. The shape in
  turbopuffer's embedding docs — both legs inside one `rank_by` — is not
  accepted; the error points at `/docs/hybrid`, which is the multi-query form.
- All three qwen3 sizes normalize to the same derived `embed_text:[1024]f16`
  column. Model size changes the token price and nothing about storage.

### Two one-way doors

Neither is a decision that can be revisited cheaply, and they are the reason
this RFC exists rather than being an implementation detail:

1. **A vector column is immutable after namespace creation.** An FTS-only
   namespace can never accept an embedding UDF (layer-pro RFC 0011, confirmed
   empirically). Declaring the embedding at first write is not an optimization.
2. **Turbopuffer cannot re-embed.** A model change is a full re-ingest of the
   archive.

Which settles the model: **`qwen/qwen3-embedding-8b`**. Since all three sizes
cost the same per byte stored, the only axis that differs is quality against
token price — $0.05/M versus $0.01/M — and a measured 878k tokens per 30
transcripts puts a complete backfill of a 1,700-transcript corpus around
$2.50. Paying three cents a thousand transcripts to avoid a re-ingest is not
a close call.

### Namespace schema

One row per chunk. `text` is the one column that is both full-text indexed
and embedded; the rest are filterable attributes, so `hev find` can scope a
query the way an operator thinks about it — this session, this repo, this
plan, everything since Tuesday.

| Column | Kind | Notes |
|---|---|---|
| `id` | key | content hash of text + coordinates — idempotent re-index |
| `text` | FTS + `embed` | the chunk body; the store derives `embed_text:[1024]f16` |
| `session_id`, `turn_uuid`, `parent_uuid`, `seq`, `part` | filter | reassembly + threading |
| `harness`, `role`, `block_type`, `tier`, `tool_name` | filter | `claude_code` \| `codex` \| … |
| `ts` | filter | recency, ranges |
| `workdir`, `branch`, `source_path`, `is_sidechain` | filter | repo context, subagent turns |
| `instance`, `plan`, `rfc`, `issue`, `pr` | filter | factory join, when present |

`distance_metric` is `cosine_distance`. There is no hand-written `vector`
column: declaring `embed` on `text` is what creates the vector, and the
derived column is the store's to name.

The last row is the one that makes a fleet archive worth more than a pile
of transcripts, and it is the one no upstream tool can supply. It depends
on the factory writing the harness session id into its child ledger at
dispatch — tracked separately in `hev/factory` (`plans/trace-join.md`).
Absent that, these columns are simply empty and everything else works.

## Goals

- One model, four harnesses, and a fifth costs one interface.
- Re-running an index is a no-op; interrupting one loses nothing.
- `hev find` scopes by repo, plan and time without a full-text hack.
- Nothing embeds, reranks or fuses inside kit.

## Non-goals

- **Retrieval quality work.** Ranking is the store's; if hybrid recall is
  bad, that is a layer-pro issue and a better bug report for having kit as a
  real client.
- **Keeping the raw-body source.** It goes. RFC 0001 frames are unaffected.
- **A migration for existing warehouse envelopes.** `hev migrate` stays
  scoped to RFC 0002.
- **Retrieval tuning knobs.** No client-side reranker, no recency decay, no
  weighting between the two legs. RRF is the default that needs no tuning,
  and adding a dial before there is a complaint is how a capture tool grows a
  retrieval stack.

## Vocabulary

- **Turn / Block** — the normalized unit of a transcript, above.
- **Unit** — one artifact a source indexes (a transcript file; a session
  inside a bulk file), and the granule of both skip-if-unchanged and
  single-commit append.
- **Tier** — L1/L2/L3, deciding when a block is indexed.
- **Attribution** — funes is Apache-2.0. Ported files carry a header
  naming upstream and the license, and the repo gains a `NOTICE`. This is
  a license obligation, not a courtesy.

## Open questions

- ~~Does a whole-session summary row earn its place next to chunk rows, or
  does `hev ls` reconstruct from `seq`-ordered chunks?~~ Resolved: `hev ls`
  uses a grouped aggregation over chunk coordinates. Existing namespaces work
  without a forced backfill, and the embedded archive remains one row per
  searchable chunk rather than mixing content and bookkeeping rows.
- ~~Is L3 `tool_result` indexed by default?~~ Resolved: opt-in via
  `--tier`. It is 44% of chunks in a real corpus (105,793 of 240,853) and
  the least of what anyone searches for.
- ~~Should `tool_use` rendering summarize a call, or should retrieval legs be
  weighted?~~ Resolved: the renderer keeps the tool name plus its salient
  command, path, or query. That preserves command/path lookup and makes tool
  results legible without adding retrieval tuning to the client.
- Sidechains: funes tracks subagent transcripts as their own units with an
  `is_subagent` flag. Does a factory worker's subagent get its own
  `session_id`, or a `parent_session_id` edge?
- Does the daemon write chunks directly, or keep landing envelopes in the
  spool and chunk on upload? The second keeps the spool replayable when
  the schema changes, at the cost of a second pass.
- Does deleting redaction need a config escape hatch for a shared machine,
  or is "do not point kit at a machine you do not own" the answer?
