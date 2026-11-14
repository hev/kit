---
id: "api#errors"
title: "API overview"
heading: "Errors"
group: "API"
order: 11
url: "/docs/api#errors"
anchor: "errors"
terms: ["errors","apis","return","http","status","json","body","input","missing","disallowed","requests","unknown","objects","server","faults","field","returns","short","error","code","local","document","their","bodies","page","unauthenticated","invalid","grant","every","answers","codes","mean","usually","parameter","request","mode","does","allow","object","fault"]
hash: "6c6d0495f830f2bb4e38fea8b7c736588a36db00dcd9e0224e7d72a1d1feb4a9"
mode: "source-primary"
facts: [{"kind":"code","literal":"400","chunkId":"api#errors"},{"kind":"code","literal":"401","chunkId":"api#errors"},{"kind":"code","literal":"403","chunkId":"api#errors"},{"kind":"code","literal":"404","chunkId":"api#errors"},{"kind":"code","literal":"5xx","chunkId":"api#errors"},{"kind":"code","literal":"{\"error\": \"code\"}","chunkId":"api#errors"},{"kind":"code","literal":"unauthenticated","chunkId":"api#errors"},{"kind":"code","literal":"invalid_grant","chunkId":"api#errors"}]
sources: [{"chunkId":"api#errors","url":"/docs/api#errors","anchor":"errors"}]
---

All APIs return an HTTP status and a JSON body: 400 for bad input, 401 for a missing or bad key, 403 for disallowed requests, 404 for unknown objects, 5xx for server faults. The Field Kit API returns a short error code; local APIs document their error bodies per page.
