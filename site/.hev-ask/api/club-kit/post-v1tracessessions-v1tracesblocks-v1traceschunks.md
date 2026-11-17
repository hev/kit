---
id: "api/club-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"
title: "Club Kit API"
heading: "POST /v1/traces/sessions, /v1/traces/blocks, /v1/traces/chunks"
group: "API"
order: 48
url: "/docs/api/club-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"
anchor: "post-v1tracessessions-v1tracesblocks-v1traceschunks"
terms: ["post","traces","sessions","blocks","chunks","writes","batches","trace","rows","search","these","after","secrets","scrubbed","machine","covers","required","fields","batch","limits","idempotent","retries","safe","errors","invalid","oversize","another","author","session","rate","limiting","billing","read","only","daemon","keeps","local","outbox","while","unreachable"]
hash: "f3a82fbaf10e16e4aa86698d25508f5dcd6aeb819661cece2927a6a5afae6558"
mode: "source-primary"
facts: [{"kind":"code","literal":"{ \"accepted\": 1 }","chunkId":"api/club-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"},{"kind":"code","literal":"id","chunkId":"api/club-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"},{"kind":"code","literal":"session_id","chunkId":"api/club-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"},{"kind":"code","literal":"400 {\"error\":\"invalid_rows\"}","chunkId":"api/club-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"},{"kind":"code","literal":"400 {\"error\":\"invalid_body\"}","chunkId":"api/club-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"},{"kind":"code","literal":"413 {\"error\":\"batch_too_large\"}","chunkId":"api/club-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"},{"kind":"code","literal":"403 {\"error\":\"not_your_session\"}","chunkId":"api/club-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"},{"kind":"code","literal":"429 {\"error\":\"rate_limited\"}","chunkId":"api/club-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"},{"kind":"code","literal":"Retry-After","chunkId":"api/club-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"},{"kind":"code","literal":"402 {\"error\":\"billing_read_only\"}","chunkId":"api/club-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"},{"kind":"code","literal":"hevd","chunkId":"api/club-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"}]
sources: [{"chunkId":"api/club-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks","url":"/docs/api/club-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks","anchor":"post-v1tracessessions-v1tracesblocks-v1traceschunks"}]
---

Writes batches of trace rows for sessions, blocks or search chunks; these are kit's own trace rows after secrets are scrubbed on the machine. Covers required row fields, batch limits of 500 rows and 4 MiB, idempotent writes by row ID so retries are safe, and errors for invalid rows, oversize batches, another author's session, rate limiting and billing read-only. The daemon keeps rows in a local outbox while the API is unreachable, busy or read-only and sends them later.
