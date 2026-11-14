---
id: "api/dashboard#get-apisessions"
title: "Dashboard API"
heading: "GET /api/sessions"
group: "API"
order: 45
url: "/docs/api/dashboard#get-apisessions"
anchor: "get-apisessions"
terms: ["sessions","lists","every","trace","matching","filters","newest","first","paging","optionally","including","stats","returns","facet","values","across","whole","archive","session","summary","fields","such","harness","model","project","branch","host","timing","counts","tokens","cost","skills","tool","grade","curl","http","8787","window","include","true"]
hash: "ab9851aad04101b350c10e8a64ce5fe818747e64320090d498b9ff1f770657a0"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s 'http://127.0.0.1:8787/api/sessions?window=7d'","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"include_stats","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"true","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"stats","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"/api/stats","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"facets","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"window","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"7d","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"30d","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"90d","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"all","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"sessions","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"project","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"model","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"harness","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"host","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"skill","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"tool","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"role","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"instance","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"marks","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"include_stats=true","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"timing","chunkId":"api/dashboard#get-apisessions"},{"kind":"code","literal":"id","chunkId":"api/dashboard#get-apisessions"}]
sources: [{"chunkId":"api/dashboard#get-apisessions","url":"/docs/api/dashboard#get-apisessions","anchor":"get-apisessions"}]
---

Lists every trace matching the filters newest-first with no paging, optionally including stats. Returns facet values across the whole archive and per-session summary fields such as harness, model, project, branch, host, timing, counts, tokens, cost, skills, tool counts and grade.
