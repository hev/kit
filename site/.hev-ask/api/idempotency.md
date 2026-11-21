---
id: "api#idempotency"
title: "API overview"
heading: "Idempotency"
group: "API"
order: 15
url: "/docs/api#idempotency"
anchor: "idempotency"
terms: ["idempotency","create","calls","accept","header","replay","within","hours","returns","first","response","trace","ingest","peer","messages","idempotent","their","creates","take","without","client"]
hash: "f07f1cb29d36a005a493079c7141ca94d628f9ae5b0b838a98bf42e2445ed2c8"
mode: "source-primary"
facts: [{"kind":"code","literal":"Idempotency-Key","chunkId":"api#idempotency"},{"kind":"code","literal":"id","chunkId":"api#idempotency"}]
sources: [{"chunkId":"api#idempotency","url":"/docs/api#idempotency","anchor":"idempotency"}]
---

Create calls accept an idempotency key header; a replay within 24 hours returns the first response. Trace ingest and peer messages are idempotent by their own ids.
