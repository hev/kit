---
title: Layer CE has no local backend suitable for a free-tier/offline home lab today
date: 2026-09-06
area: deploy
kind: environment
tags: [layer-ce, turbopuffer, pgvector, minio, homelab, docker-compose]
source: LYR-26 step 5, worker-lyr-kit-homelab-step5, corrected per Adam 2026-09-06
superseded_by: docs/learnings/deploy/layer-ce-pgvector-serves-search-only.md
---

## What happened
Read-side plan step 5 asked for a `deploy/homelab/docker-compose.yml` with
Layer CE, MinIO and `hev serve` — a fresh clone reaching the page with no
external dependency. The plan's own constraints section already flagged an
open question ("decide hosted gateway vs FTS-only") but framed it as an
embedding choice.

## What didn't work
Assuming FTS-only (no embedding model) would make the stack self-contained.
It doesn't, but not for the reason first suspected: CE's mirrored gateway
*does* carry a second `ResolvedVectorStoreKind::Search` arm (`HttpSearchClient`
in `mirror-gateway.sh`'s generated `server.rs`) pointing at the first-party
`hev search` engine — this is not Turbopuffer-only. The real blocker is
product positioning, not code: Adam's call (2026-09-06) is that the free/CE
tier is fronted by **pgvector** (RFC 0114), not `kind: search` — so a
home-lab compose built against `kind: search` would be building on a backend
CE isn't meant to expose for this purpose, even though the code path exists
today.

## What works
Nothing landed yet — this is sequencing, not an open architecture question.
Step 5 is parked behind LYR-20 (RFC 0114, `kind: pgvector`). Once phase 1
lands, the home-lab stack is Layer CE + Postgres/pgvector (the `paradedb/paradedb`
bundle per 0114's licensing section) + MinIO + `hev serve`.

## How to avoid it
Before scoping a "local/offline" deploy against Layer CE, don't infer the
available backend from a grep of the mirrored gateway code alone — a code
path existing (`kind: search`) doesn't mean it's the intended free-tier
backend. Check current product direction (LYR-20 / RFC 0114) for what CE is
meant to front with. Don't scope a home-lab/offline deploy against CE until
RFC 0114 phase 1 lands.
