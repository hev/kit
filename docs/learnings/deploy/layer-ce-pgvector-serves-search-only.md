---
title: Historical Postgres read-side gaps and their runtime capability resolution
date: 2026-09-20
area: deploy
kind: environment
tags: [layer-ce, pgvector, paradedb, hev-up, capabilities, read-side]
source: https://linear.app/hevmind/issue/LYR-89
---

*Since then: Layer 0.7.0 serves ordered scans and conditional upserts on
Postgres (LYR-112), and 0.7.1 serves `[]uint`/`[]string` attributes with
`ContainsAny` and `exclude_attributes` (LYR-137, LYR-138; kit v0.3.1, RFC 0006
amendment 2026-09-28). 0.7.2 serves `patch_rows` and `patch_condition`
(LYR-140; kit v0.3.2), so session summaries are written there too.*

## What happened
`hev up` (RFC 0006) runs Layer CE locally: `public/ce/docker-compose.yml` from
`hev/layer-pro`, ParadeDB plus `hevlayer/layer-gateway:edge`, with a blank
`TURBOPUFFER_API_KEY` selecting the Postgres backend. That supersedes the older
learning that CE had no local backend. RFC 0006 listed three things the backend
refuses (`embed`, a second `full_text_search` field, the multi-query body). The
first live index found the list is much longer.

## What didn't work
Assuming the three named gaps were all of them. On `edge` (0.6.0-dev) the
pgvector store answers `422 UnsupportedByStore` for every one of these, all of
which kit sends somewhere:

- `upsert_condition` (session and eval writes)
- schema types `[]uint` and `[]string` (the sessions namespace)
- any ordered scan: `rank_by` on an attribute or on `id`, and a filter-only
  query (every listing in `hev serve`, `hev ls`, `hev trace`, the TUI)
- `aggregate_by` (`hev ls`), `patch_rows` (summaries), `exclude_attributes`
- `HybridText` with the default `fuzziness: "auto"`; it needs `fuzziness: 0`

The session write failing meant no unit was ever recorded as indexed, so every
cycle re-uploaded everything and `hev s` showed an error forever.

## What worked on 0.6.0-dev
Writes of scalar and one full-text column, and
`rank_by: ["text", "HybridText", q, {"fuzziness": 0}]`. So on the local lane
`hev up`, the daemon, `hev index` and `hev find` work, and the read side has
nothing it can list. Every per-store decision is one answer in
`internal/layer/capabilities.go`; the read-side rows are skipped where the
store has no ordered scan, rather than written and never read.

## How to avoid it
Before building on a Layer store, read its row in
`site/src/generated/store-capabilities.json` on `layer-pro` `main` — it lists
every wire feature per store — and then prove the calls you need with `curl`
against a throwaway Compose project, since the artifact is coarser than the
gateway (it says `hybrid: supported` while `fuzziness: "auto"` 422s). The gateway now serves `GET /v2/namespaces/{ns}/capabilities`. Kit reads
pgvector's `declared: true` report to enable ordered lists and conditional
upserts, matching entries by feature id. Older, unavailable or undeclared
servers enable neither operation. Refusal notes come from the server.

The authoritative contract and live wire receipts are in merged
[layer-pro #801](https://github.com/hev/layer-pro/pull/801), following the
implementation in [#627](https://github.com/hev/layer-pro/pull/627). Ordered
scans do not imply `search_after`: ranked cursors remain unsupported. Other
compatibility restrictions in kit's static table remain until their own
contracts are resolved.
