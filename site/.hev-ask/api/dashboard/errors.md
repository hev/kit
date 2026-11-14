---
id: "api/dashboard#errors"
title: "Dashboard API"
heading: "Errors"
group: "API"
order: 38
url: "/docs/api/dashboard#errors"
anchor: "errors"
terms: ["errors","dashboard","json","message","parameters","failed","archive","reads","list","endpoints","unknown","session","image","plain","text","methods","later","factory","accounting","paths","return","page","error","window","must","timing","layer","queries","rows","sessions","stats","search","values","asks","sha256","head","body","status","code","layerms"]
hash: "8a97937a7d0e76555d807ba6c5c7d4980c73377bb58c7848d4fa0976ab47a176"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\"error\":\"window must be 7d, 30d, 90d, or all\",\"timing\":{\"layer_ms\":0,\"queries\":0,\"rows\":0}}","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"400","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/sessions","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/stats","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/search","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/values","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"404","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/session/{id}","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/asks/{id}","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/image/{sha256}","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"405","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"GET","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"HEAD","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"500","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"502","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/asks","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"503","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"/api/factory","chunkId":"api/dashboard#errors"},{"kind":"code","literal":"200","chunkId":"api/dashboard#errors"}]
sources: [{"chunkId":"api/dashboard#errors","url":"/docs/api/dashboard#errors","anchor":"errors"}]
---

Dashboard API errors are JSON with a message: 400 for bad parameters or failed archive reads on list endpoints, 404 for unknown session, ask or image IDs (image 404 is plain text), 405 for non-GET methods, 500 for failed session reads, 502 for later failed reads, and 503 when factory accounting is not set up. Unknown GET paths return the dashboard page.
