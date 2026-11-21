---
id: "api/hooks#observe-only-deliveries"
title: "Hooks API"
heading: "Observe-only deliveries"
group: "API"
order: 38
url: "/docs/api/hooks#observe-only-deliveries"
anchor: "observe-only-deliveries"
terms: ["observe","only","deliveries","endpoints","must","answer","within","seconds","failures","retry","backoff","about","seven","attempts","under","same","delivery","least","once","unordered","dedupe","disables","hook","time","gone","disabled","reason","failing","patch","enabled","true","body","ignored","anything","else","retried","ordered","order","event","disabledreason"]
hash: "bf89474f4193599c7fd29d158a348ee31aa954269f279a6144d08a9567261d14"
mode: "source-primary"
facts: [{"kind":"code","literal":"X-Hev-Delivery-Id","chunkId":"api/hooks#observe-only-deliveries"},{"kind":"code","literal":"time","chunkId":"api/hooks#observe-only-deliveries"},{"kind":"code","literal":"410 Gone","chunkId":"api/hooks#observe-only-deliveries"},{"kind":"code","literal":"disabled_reason: gone","chunkId":"api/hooks#observe-only-deliveries"},{"kind":"code","literal":"disabled_reason: failing","chunkId":"api/hooks#observe-only-deliveries"},{"kind":"code","literal":"PATCH","chunkId":"api/hooks#observe-only-deliveries"},{"kind":"code","literal":"enabled: true","chunkId":"api/hooks#observe-only-deliveries"}]
sources: [{"chunkId":"api/hooks#observe-only-deliveries","url":"/docs/api/hooks#observe-only-deliveries","anchor":"observe-only-deliveries"}]
---

Observe-only endpoints must answer any 2xx within 10 seconds. Failures retry with backoff over about a day (seven attempts) under the same delivery id; delivery is at least once and unordered, so dedupe. A 410 disables the hook.
