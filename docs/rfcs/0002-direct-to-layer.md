# RFC 0002: Direct-to-Layer capture — retire the warehouse-first architecture

Tracking issue: TBD

**Status: draft, no implementation started. Supersedes the
warehouse-first stance in `README.md` ("Warehouse Stance") and the
`iceberg`-Warehouse tie-in described in the lyr umbrella docs; those
update when this RFC is accepted.**

## Summary

Capture ships trace envelopes **directly to a hev layer endpoint** — a
self-hosted Layer CE instance or the hosted gateway — instead of to a
user-provisioned S3/R2 warehouse. The warehouse tier is removed from
kit's architecture, and with it the plan where Layer would later read
kit's archive through the reserved `iceberg` `Warehouse` kind
(layer-pro RFC 0046, with RFC 0063 as precedent) and a `Pipeline`
would turn it into an `Index`.

The property the warehouse actually provided — a durable, append-only,
`zcat`-able raw log that outlives any index — does not belong in kit.
It becomes a **WAL feature in Layer** (a future layer-pro RFC): every
write lands in an append-only log on object storage before indexing,
giving replay and point-in-time recovery at the platform layer, for
every Layer customer, not just kit. Until that ships, kit's existing
local spool + upload ledger covers durability-in-transit.

Net effect: `hev find` works on day one instead of after an unshipped
read path, `hev init` asks for an endpoint instead of running a
bucket-credentials wizard, and **every kit install becomes a Layer
install** — kit turns into Layer's OSS wedge.

## Motivation

- **The current wedge waits on the longest unbuilt path in the
  portfolio.** Today Capture lands envelope JSONL in a bucket, and
  everything interesting — search, replay, Map, evals — waits on:
  Iceberg table layout in kit, the reserved `iceberg` `Warehouse` kind
  shipping in Layer (RFC 0046 is a sketch, not a plan of record), a
  `Pipeline` to index it. Three projects have to move before a user
  can search their own traces. Direct-to-Layer collapses that to one
  hop that works with Layer as it exists today: the gateway's
  turbopuffer-shaped write API.
- **Onboarding is a storage-credentials interview.** `hev init` today
  configures endpoint, bucket, key, secret, region, profile
  (`internal/daemon/config.go` — the `S3*` block) before the tool has
  done anything for you. The direct path needs a Layer URL and an API
  key; the CE quickstart (Compose) stands up both in minutes on the
  operator's own hardware.
- **The archive is inert.** The current ceiling is
  `aws s3 cp | zcat | jq`. The pitch — personalization, evals,
  replay, "your traces before they disappear" — is search-shaped.
  Landing traces in a Layer namespace makes `hev find`, the TUI, and
  Map real features instead of roadmap items, and frames (RFC 0001)
  get their vector index for free when they arrive.
- **kit currently drives zero Layer adoption.** The demos exercise
  Layer, but none of them put it in a stranger's hands. kit is a
  daily-driver CLI for exactly Layer's audience — engineers building
  and tuning coding agents. As a direct client, every kit user stands
  up Layer CE (or points at the hosted gateway). That is a better OSS
  wedge than "portable Iceberg you could theoretically attach later."
- **Durability is a platform concern, not an app concern.** kit
  building its own warehouse contract duplicates what a Layer WAL
  gives every customer: raw-log durability, replay, PITR — the same
  write-path guarantees the Mesh sketch reaches for. One
  implementation, at the layer that owns the write path.

## What is preserved: the sovereignty pitch

"Get your agency back" survives the retool; the trust boundary moves
from *your bucket* to *your Layer*.

- The default target is **Layer CE, self-hosted** — source-available,
  running on the operator's machine or homelab, backed by object
  storage the operator holds the keys to. Same ownership as the
  bucket, plus an index.
- Nothing requires the hosted gateway. It is an option for people who
  want zero infrastructure, not the default.
- The future Layer WAL keeps the "valuable with basic tools" promise:
  an append-only log on the operator's object storage, readable with
  `zcat` and `jq`, independent of any index built over it.

## Vocabulary

- **Target** — the Layer endpoint + API key + namespace Capture writes
  to. Replaces the *warehouse* (bucket + layout) as the destination
  concept.
- **Spool / ledger** — unchanged: the local buffer and upload record
  (`internal/daemon/ledger.go`) that make shipping resumable and
  offline-safe. They are transport durability, not the archive.
- **WAL (Layer)** — the proposed platform feature: an append-only raw
  write log on object storage, ahead of indexing, owned and specified
  by layer-pro. Out of scope for kit beyond being its first motivating
  customer.

## Proposed surface

- **`hev init`** — asks for a target: an existing Layer endpoint + key,
  or offers to bootstrap Layer CE locally (delegating to the CE
  quickstart). The bucket wizard goes away.
- **Config** — the `S3*` / `Buckets` block is replaced by a `[layer]`
  section: `endpoint`, `api_key` (or a 1Password/env reference),
  `namespace` (default `hev-traces`). Old configs with only S3 keys
  fail with a pointer to `hev migrate`.
- **Daemon** — `internal/daemon/uploader.go` targets the gateway write
  API instead of minio: envelopes become rows (embeddable text plus
  session/turn/repo/branch/profile/timing metadata columns), schema
  asserted on write the way the demos do. Scanner, envelope, policy,
  redaction are untouched — redaction still happens **before**
  anything leaves the machine.
- **Read side** — `hev ls`, `hev trace`, and the TUI read from the
  target namespace; `hev find <query>` becomes a real search command
  over the gateway.
- **`hev migrate`** — one-shot backfill: walk an existing warehouse
  bucket, upsert the envelopes into the target namespace. The bridge
  out of the old architecture; S3 read code survives only here.

## Goals

- A fresh user goes from `go install` to searching their own traces in
  one sitting, with nothing but kit and Layer CE.
- kit has no user-facing storage contract of its own; Layer owns the
  data path.
- Existing warehouse users have a migration path and lose no data.

## Non-goals

- **Building the WAL in kit.** kit states the requirement; the design
  lands as a layer-pro RFC.
- **Keeping a parallel S3 write path** ("mirror to bucket") as a
  permanent option — transition shim at most, decided in review.
- **Frame capture changes.** RFC 0001 is unaffected; frames follow the
  same direct path when they land.
- **Fleet/team capture, analytical warehouses** (ClickHouse et al.) —
  same posture as before: not the wedge.

## Open questions

- ~~Namespace schema for trace envelopes: chunking granularity, which
  fields are filterable vs FTS vs vector.~~ Answered by
  [RFC 0003](0003-trace-model.md).
- ~~Embedding: gateway-side (the embed wire) vs on-device.~~ Answered by
  RFC 0003: FTS-first retrieval, but the vector column is declared at
  namespace creation because Layer vector columns are immutable
  afterwards (layer-pro RFC 0011).
- Offline depth: how much history the spool retains when the target is
  unreachable for days, and whether that answer changes before the
  Layer WAL exists.
- CE bootstrap: does `hev init` shell out to Compose, or just print
  the quickstart? (Owning a running Layer would make kit an operator,
  which it shouldn't be.)
- Whether `hev migrate` ships in the first cut or follows once the
  write path is proven.
