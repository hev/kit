---
title: Layer CE on Postgres serves writes and HybridText search, and nothing the read side reads
date: 2026-09-20
area: deploy
kind: environment
tags: [layer-ce, pgvector, paradedb, hev-up, capabilities, read-side]
source: https://linear.app/hevmind/issue/LYR-89
---

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

## What works
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
gateway (it says `hybrid: supported` while `fuzziness: "auto"` 422s). The
gateway serves no capability endpoint at runtime yet (LYR-86); match refusals
on status 422 and the typed `error`/`store` fields, never on message text.
Re-check this learning when LYR-85..88 land: each one deletes a row of the
static table.
