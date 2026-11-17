---
id: "api/dashboard#errors"
title: "Dashboard API"
heading: "Errors"
group: "API"
order: 53
url: "/docs/api/dashboard#errors"
anchor: "errors"
terms: ["errors","dashboard","json","message","status","code","table","covers","parameters","failed","archive","reads","missing","sessions","asks","images","wrong","methods","later","read","failures","factory","accounting","being","unknown","paths","return","page","rather","error","window","must","timing","layer","queries","rows","stats","search","values","session"]
hash: "8a97937a7d0e76555d807ba6c5c7d4980c73377bb58c7848d4fa0976ab47a176"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\"error\":\"window must be 7d, 30d, 90d, or all\",\"timing\":{\"layer_ms\":0,\"queries\":0,\"rows\":0}}","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"400","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/sessions","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/stats","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/search","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/values","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"404","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/session/{id}","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/asks/{id}","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/image/{sha256}","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"405","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"GET","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"HEAD","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"500","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"502","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/asks","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"503","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/factory","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"200","chunkId":"api/dashboard#errors"}]
sources: [{"chunkId":"api/dashboard#errors","url":"/docs/api/dashboard#errors","anchor":"errors"}]
---

Dashboard API errors are a JSON message with a status code. The table covers bad parameters and failed archive reads, missing sessions, asks or images, wrong methods, later read failures, and factory accounting not being set up. Unknown paths return the dashboard page with 200 rather than a 404.
