---
id: "api/dashboard#get-apiasks"
title: "Dashboard API"
heading: "GET /api/asks"
group: "API"
order: 39
url: "/docs/api/dashboard#get-apiasks"
anchor: "get-apiasks"
terms: ["asks","lists","rooted","traces","newest","first","view","built","local","dispatch","records","joined","archived","sessions","including","other","machines","runs","layer","filter","window","asked","host","project","text","whether","include","tracing","trace","summarizes","caller","timing","users","hosts","projects","orders","counts","worst","state","cost"]
hash: "7146d46b0a5d4fc75126bd81a23bc666c69b4d2e147f7d53dadb025a7ccfaf5f"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s 'http://127.0.0.1:8787/api/asks'","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"hev dispatch","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"~/dispatch","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"window","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"7d","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"30d","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"90d","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"all","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"user","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"local:hev","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"alex","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"host","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"project","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"pretrace","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"true","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"traces","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"users","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"hosts","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"projects","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"{value, count}","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"count: 0","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"pretrace_hidden","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"pretrace=true","chunkId":"api/dashboard#get-apiasks"},{"kind":"code","literal":"timing","chunkId":"api/dashboard#get-apiasks"}]
sources: [{"chunkId":"api/dashboard#get-apiasks","url":"/docs/api/dashboard#get-apiasks","anchor":"get-apiasks"}]
---

Lists ask-rooted traces newest-first (the Asks view), built from local dispatch run records joined to archived sessions, including other machines' runs when traces are in hev layer. Filter by window, who asked, host, project, text, and whether to include pre-tracing runs; each trace summarizes the ask, caller, timing, users, hosts, projects, orders, counts, worst state and cost.
