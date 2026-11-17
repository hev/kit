---
id: "api/dashboard#get-apivalues"
title: "Dashboard API"
heading: "GET /api/values"
group: "API"
order: 62
url: "/docs/api/dashboard#get-apivalues"
anchor: "get-apivalues"
terms: ["values","counts","filter","facet","across","traces","other","filters","keep","picker","shows","selection","ignored","choosing","value","never","hides","others","without","grade","come","memory","copy","archive","refreshed","configurable","interval","curl","http","8787","model","cached","true","timing","layer","queries","rows","total","truncated","false"]
hash: "bd8e2063c087ee21741890206b85058877833dd8546634458d0ccafe5d98e40e"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s 'http://127.0.0.1:8787/api/values?facet=model'","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"{\n  \"cached\": true,\n  \"timing\": { \"layer_ms\": 0, \"queries\": 0, \"rows\": 0 },\n  \"total\": 4,\n  \"truncated\": false,\n  \"values\": [\n    { \"n\": 4, \"v\": \"claude-opus-5-5\" },\n    { \"n\": 2, \"v\": \"gpt-5.3-codex\" },\n    { \"n\": 1, \"v\": \"claude-opus-4-5\" },\n    { \"n\": 1, \"v\": \"claude-sonnet-5-5\" }\n  ]\n}","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"{\"error\":\"facet must be project, model, tool, skill, harness, or host\",\"timing\":{\"layer_ms\":0,\"queries\":0,\"rows\":0}}","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"facet","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"project","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"model","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"tool","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"skill","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"harness","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"host","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"hev serve","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"--cache-ttl","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"1m","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"values","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"{v, n}","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"total","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"truncated","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"false","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"cached","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"true","chunkId":"api/dashboard#get-apivalues"},{"kind":"code","literal":"timing","chunkId":"api/dashboard#get-apivalues"}]
sources: [{"chunkId":"api/dashboard#get-apivalues","url":"/docs/api/dashboard#get-apivalues","anchor":"get-apivalues"}]
---

Counts the values of one filter facet across the traces the other filters keep, which is what a filter picker shows; the facet's own selection is ignored so choosing a value never hides the others. Without grade filters, counts come from an in-memory copy of the archive refreshed on a configurable interval.
