---
id: "api/dashboard#get-apiasksid"
title: "Dashboard API"
heading: "GET /api/asks/\\{id\\}"
group: "API"
order: 40
url: "/docs/api/dashboard#get-apiasksid"
anchor: "get-apiasksid"
terms: ["asks","returns","rooted","trace","spans","tree","waterfall","view","orders","runs","validations","sessions","kind","specific","details","such","order","text","acceptance","items","state","host","harness","model","branch","errors","context","digest","session","cost","tokens","unknown","curl","http","8787","0af7651916cd43dd8448eb211c80319c","error","found","dispatch","records"]
hash: "98b8f01c9e6316b073057db8d649dd92c0c7defaa3b4110e95eddfa4babb6ddd"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s http://127.0.0.1:8787/api/asks/0af7651916cd43dd8448eb211c80319c","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"{\"error\":\"trace not found on this host's dispatch records\",\"timing\":{\"layer_ms\":0,\"queries\":0,\"rows\":0}}","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"id","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"/api/asks","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"run:NAME","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"trace","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"spans","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"parent","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"timing","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"kind","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"ask","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"order","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"run","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"validation","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"session","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"name","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"start","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"text","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"action","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"dispatch","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"continue","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"rework","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"amend","chunkId":"api/dashboard#get-apiasksid"},{"kind":"code","literal":"retry","chunkId":"api/dashboard#get-apiasksid"}]
sources: [{"chunkId":"api/dashboard#get-apiasksid","url":"/docs/api/dashboard#get-apiasksid","anchor":"get-apiasksid"}]
---

Returns one ask-rooted trace and its spans as a tree (the waterfall view), where spans are asks, orders, runs, validations and sessions with kind-specific details such as order text and acceptance items, run state, host, harness, model, branch, PR, errors, context digest, and session cost and tokens. Unknown IDs are 404.
