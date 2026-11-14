---
id: "api/field-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"
title: "Field Kit API"
heading: "POST /v1/traces/sessions, /v1/traces/blocks, /v1/traces/chunks"
group: "API"
order: 61
url: "/docs/api/field-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"
anchor: "post-v1tracessessions-v1tracesblocks-v1traceschunks"
terms: ["post","traces","sessions","blocks","chunks","write","batches","trace","rows","search","already","scrubbed","secrets","machine","capped","writes","idempotent","retries","safe","rate","limiting","asks","clients","back","daemon","keeps","outbox","capture","never","waits","network","accepted","retry","after","hevd","batch","session","block","turn","message"]
hash: "e21a8c75508bbab74c8452731429bd53435b17caabcfd2701a293ae8b2c357f2"
mode: "source-primary"
facts: [{"kind":"code","literal":"{ \"accepted\": 1 }","chunkId":"api/field-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"},{"kind":"code","literal":"413","chunkId":"api/field-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"},{"kind":"code","literal":"id","chunkId":"api/field-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"},{"kind":"code","literal":"429","chunkId":"api/field-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"},{"kind":"code","literal":"Retry-After","chunkId":"api/field-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"},{"kind":"code","literal":"hevd","chunkId":"api/field-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks"}]
sources: [{"chunkId":"api/field-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks","url":"/docs/api/field-kit#post-v1tracessessions-v1tracesblocks-v1traceschunks","anchor":"post-v1tracessessions-v1tracesblocks-v1traceschunks"}]
---

Write batches of trace rows (sessions, blocks, or search chunks), already scrubbed of secrets on the machine. Batches are capped at 500 rows and 4 MiB, writes are idempotent by row ID so retries are safe, and rate limiting asks clients to back off. The daemon keeps an outbox so capture never waits on the network.
