---
id: "concepts#run"
title: "Concepts"
heading: "Run"
group: "Use"
order: 58
url: "/docs/concepts#run"
anchor: "run"
terms: ["order","executing","host","state","directory","holding","worktree","status","inbox","report","metadata","result","ends","done","failed","timeout","died","dispatch","commands","read","these","files","name","meta","json","contexts","show","logs","clean","lives","under","path","holds","sent","agent","works","short","progress","notes","messages"]
hash: "bc0064921004acb947de2e5138964aaab8e7c3c76748eceb5fa20b45b090a3fa"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/dispatch/\u003cname\u003e/","chunkId":"concepts#run"},{"kind":"code","literal":"order.md","chunkId":"concepts#run"},{"kind":"code","literal":"wt/","chunkId":"concepts#run"},{"kind":"code","literal":"status.md","chunkId":"concepts#run"},{"kind":"code","literal":"inbox.md","chunkId":"concepts#run"},{"kind":"code","literal":"hev dispatch say","chunkId":"concepts#run"},{"kind":"code","literal":"report.md","chunkId":"concepts#run"},{"kind":"code","literal":"log","chunkId":"concepts#run"},{"kind":"code","literal":"meta.json","chunkId":"concepts#run"},{"kind":"code","literal":"result.json","chunkId":"concepts#run"},{"kind":"code","literal":"contexts/N/","chunkId":"concepts#run"},{"kind":"code","literal":"done","chunkId":"concepts#run"},{"kind":"code","literal":"failed","chunkId":"concepts#run"},{"kind":"code","literal":"timeout","chunkId":"concepts#run"},{"kind":"code","literal":"died","chunkId":"concepts#run"},{"kind":"code","literal":"hev dispatch ls","chunkId":"concepts#run"},{"kind":"code","literal":"show","chunkId":"concepts#run"},{"kind":"code","literal":"logs","chunkId":"concepts#run"},{"kind":"code","literal":"clean","chunkId":"concepts#run"}]
sources: [{"chunkId":"concepts#run","url":"/docs/concepts#run","anchor":"run"}]
---

A run is one order executing on one host, with its state in a per-run directory holding the order, worktree, status, inbox, report, log, metadata and result. It ends done, failed, timeout or died, and the dispatch commands read these files over SSH.
