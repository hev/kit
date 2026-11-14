---
id: "orders#a-small-example"
title: "Writing orders"
heading: "A small example"
group: "Use"
order: 256
url: "/docs/orders#a-small-example"
anchor: "a-small-example"
terms: ["small","example","worked","order","asks","worker","expose","rate","limit","headers","every","response","including","429s","values","limiter","already","computes","named","host","repo","timeout","reply","address","internal","ratelimit","remaining","reset","name","buildbox","acme","base","main","harness","codex","laptop","return","clients","tell","close"]
hash: "effcc44da3de6520c2dfda525f3a62458ec648b083f0e0434bd369f6be923e8e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"internal/ratelimit/","chunkId":"orders#a-small-example"},{"kind":"code","literal":"X-RateLimit-Limit","chunkId":"orders#a-small-example"},{"kind":"code","literal":"X-RateLimit-Remaining","chunkId":"orders#a-small-example"},{"kind":"code","literal":"X-RateLimit-Reset","chunkId":"orders#a-small-example"}]
sources: [{"chunkId":"orders#a-small-example","url":"/docs/orders#a-small-example","anchor":"a-small-example"}]
---

A worked example order that asks a worker to expose rate-limit headers on every API response, including 429s, using values the limiter already computes, on a named host and repo with a timeout and reply address.
