---
id: "api/hooks#deliveries"
title: "Hooks API"
heading: "Deliveries"
group: "API"
order: 34
url: "/docs/api/hooks#deliveries"
anchor: "deliveries"
terms: ["deliveries","lists","event","sent","hook","newest","first","kept","days","status","pending","succeeded","retrying","failed","code","latency","attempts","next","retry","blocking","delivery","details","hooks","hookdelivery","test","decision","open","since","until","post","tool","eventid","last","statuscode","latencyms","every","attempt","nextretryat","failedopen","filter"]
hash: "99e6ddb392c3b309b634ec91c7f0b25a4471b010466a5ec66d99ed8894c27129"
mode: "source-primary"
facts: [{"kind":"code","literal":"GET /v1/hooks/{id}/deliveries","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"HookDelivery","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"id","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"dlv_…","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"event_id","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"event","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"blocking","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"test","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"status","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"pending","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"succeeded","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"retrying","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"failed","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"status_code","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"latency_ms","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"attempts","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"next_retry_at","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"decision","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"failed_open","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"since","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"until","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"POST /v1/hooks/{id}/test","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"X-Hev-Test: 1","chunkId":"api/hooks#deliveries"},{"kind":"code","literal":"pre-tool","chunkId":"api/hooks#deliveries"}]
sources: [{"chunkId":"api/hooks#deliveries","url":"/docs/api/hooks#deliveries","anchor":"deliveries"}]
---

Lists one row per event sent to a hook, newest first, kept 30 days, with status (pending, succeeded, retrying, failed), status code, latency, attempts, next retry and blocking-delivery details.
