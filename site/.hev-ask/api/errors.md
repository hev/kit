---
id: "api#errors"
title: "API overview"
heading: "Errors"
group: "API"
order: 12
url: "/docs/api#errors"
anchor: "errors"
terms: ["errors","responses","standard","error","envelope","human","readable","message","stable","code","branch","optional","details","request","post","locked","8f2c1a9d","errorresponse","unauthorized","forbidden","found","conflict","payload","large","unsupported","media","type","invalid","order","acceptance","rate","limited","retry","after","upstream","unavailable","every","response","requestid","req8f2c1a9d"]
hash: "7281a9568f85d53f3bd9d605261d315c16092b6be7aafa7010a0a6a67c1d8719"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\n  \"error\": \"post 534 is locked\",\n  \"code\": \"locked\",\n  \"details\": { \"id\": 534 },\n  \"request_id\": \"req_8f2c1a9d\"\n}","chunkId":"api#errors"},{"kind":"code","literal":"ErrorResponse","chunkId":"api#errors"},{"kind":"code","literal":"error","chunkId":"api#errors"},{"kind":"code","literal":"{\"error\": \"...\"}","chunkId":"api#errors"},{"kind":"code","literal":"code","chunkId":"api#errors"},{"kind":"code","literal":"request_id","chunkId":"api#errors"},{"kind":"code","literal":"X-Request-Id","chunkId":"api#errors"},{"kind":"code","literal":"bad_request","chunkId":"api#errors"},{"kind":"code","literal":"unauthorized","chunkId":"api#errors"},{"kind":"code","literal":"forbidden","chunkId":"api#errors"},{"kind":"code","literal":"not_found","chunkId":"api#errors"},{"kind":"code","literal":"conflict","chunkId":"api#errors"},{"kind":"code","literal":"locked","chunkId":"api#errors"},{"kind":"code","literal":"payload_too_large","chunkId":"api#errors"},{"kind":"code","literal":"unsupported_media_type","chunkId":"api#errors"},{"kind":"code","literal":"invalid_order","chunkId":"api#errors"},{"kind":"code","literal":"## Acceptance","chunkId":"api#errors"},{"kind":"code","literal":"rate_limited","chunkId":"api#errors"},{"kind":"code","literal":"Retry-After","chunkId":"api#errors"},{"kind":"code","literal":"upstream_error","chunkId":"api#errors"},{"kind":"code","literal":"unavailable","chunkId":"api#errors"},{"kind":"value","literal":"non-2xx","chunkId":"api#errors"}]
sources: [{"chunkId":"api#errors","url":"/docs/api#errors","anchor":"errors"}]
---

Non-2xx responses use a standard error envelope with a human-readable message, a stable code to branch on, optional details and a request id.
