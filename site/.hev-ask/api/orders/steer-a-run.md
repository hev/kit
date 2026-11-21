---
id: "api/orders#steer-a-run"
title: "Orders API"
heading: "Steer a run"
group: "API"
order: 51
url: "/docs/api/orders#steer-a-run"
anchor: "steer-a-run"
terms: ["steer","steering","sends","message","live","four","levels","differ","land","next","turn","boundary","urgent","interruption","curl","https","hevmind","orders","docs","typo","20261120","authorization","bearer","content","type","application","json","level","text","stop","editing","pricing","astro","scope","01jd2x8c7k4m5n6p7q8r9s0t1v","target","dispatch","state","queued","delivery"]
hash: "2d725f4f0cf2f56966552b4a7890b69f6ef8078fcbc90edd8c98c473cc7cdb9d"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s https://kit.hevmind.com/v1/orders/kit-docs-typo-20261120/steer \\\n  -H \"Authorization: Bearer $HEV_API_KEY\" \\\n  -H \"Content-Type: application/json\" \\\n  -d '{\"level\": \"urgent\", \"text\": \"Stop editing pricing.astro; it is out of scope.\"}'","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"{\n  \"id\": \"st-01jd2x8c7k4m5n6p7q8r9s0t1v\",\n  \"target\": \"dispatch:kit-docs-typo-20261120\",\n  \"level\": \"urgent\",\n  \"text\": \"Stop editing pricing.astro; it is out of scope.\",\n  \"state\": \"queued\",\n  \"delivery\": \"tool-boundary\",\n  \"queued_at\": \"2026-11-20T09:14:02Z\"\n}","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"level","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"note","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"inbox.md","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"urgent","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"halt","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"halt_timeout","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"2m","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"redirect","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"202","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"SteerMessage","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"state","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"queued","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"injected","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"acked","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"escalated","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"blocked","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"redirected","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"GET /v1/orders/{id}","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"steer","chunkId":"api/orders#steer-a-run"},{"kind":"code","literal":"steer-*","chunkId":"api/orders#steer-a-run"},{"kind":"value","literal":"kit-v1.yaml","chunkId":"api/orders#steer-a-run"}]
sources: [{"chunkId":"api/orders#steer-a-run","url":"/docs/api/orders#steer-a-run","anchor":"steer-a-run"}]
---

Steering sends a message to a live run at one of four levels that differ in when they land (from next turn boundary to urgent interruption).
