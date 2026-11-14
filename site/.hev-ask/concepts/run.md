---
id: "concepts#run"
title: "Concepts"
heading: "Run"
group: "Use"
order: 113
url: "/docs/concepts#run"
anchor: "run"
terms: ["order","executing","host","state","directory","holding","worktree","status","notes","inbox","report","metadata","result","context","bundles","runs","done","failed","timeout","died","cleaning","removes","keeps","record","dispatch","name","meta","json","contexts","show","logs","clean","lives","under","path","holds","sent","agent","works","short"]
hash: "2dafcfd633feef43ca7d92291bd3056bd21627456b48aec61837071072efedef"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/dispatch/\u003cname\u003e/","chunkId":"concepts#run"},{"kind":"code","literal":"order.md","chunkId":"concepts#run"},{"kind":"code","literal":"wt/","chunkId":"concepts#run"},{"kind":"code","literal":"status.md","chunkId":"concepts#run"},{"kind":"code","literal":"inbox.md","chunkId":"concepts#run"},{"kind":"code","literal":"hev dispatch say","chunkId":"concepts#run"},{"kind":"code","literal":"report.md","chunkId":"concepts#run"},{"kind":"code","literal":"log","chunkId":"concepts#run"},{"kind":"code","literal":"meta.json","chunkId":"concepts#run"},{"kind":"code","literal":"result.json","chunkId":"concepts#run"},{"kind":"code","literal":"contexts/N/","chunkId":"concepts#run"},{"kind":"code","literal":"done","chunkId":"concepts#run"},{"kind":"code","literal":"failed","chunkId":"concepts#run"},{"kind":"code","literal":"timeout","chunkId":"concepts#run"},{"kind":"code","literal":"died","chunkId":"concepts#run"},{"kind":"code","literal":"hev dispatch ls","chunkId":"concepts#run"},{"kind":"code","literal":"show","chunkId":"concepts#run"},{"kind":"code","literal":"logs","chunkId":"concepts#run"},{"kind":"code","literal":"clean","chunkId":"concepts#run"}]
sources: [{"chunkId":"concepts#run","url":"/docs/concepts#run","anchor":"run"}]
---

A run is one order executing on one host, with its state in a per-run directory holding the order, worktree, status notes, inbox, report, log, metadata, result and any context bundles. Runs end as done, failed, timeout or died; cleaning removes the worktree but keeps the record.
