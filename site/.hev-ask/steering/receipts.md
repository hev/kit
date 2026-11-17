---
id: "steering#receipts"
title: "Steering"
heading: "Receipts"
group: "Use"
order: 344
url: "/docs/steering#receipts"
anchor: "receipts"
terms: ["receipts","every","steering","message","receipt","moves","queued","injected","turn","tool","call","acknowledged","reading","inbox","file","hand","never","counts","listings","show","column","orchestrator","detail","views","list","delivery","latency","peers","flags","unacknowledged","acked","dispatch","steer","orchestrate","shows","such","session"]
hash: "1178b1cd8253df9dc2691db3650a625ea69ccf99b25fc063203bb68de0f81973"
mode: "agent-primary"
facts: [{"kind":"code","literal":"queued","chunkId":"steering#receipts"},{"kind":"code","literal":"injected","chunkId":"steering#receipts"},{"kind":"code","literal":"acked","chunkId":"steering#receipts"},{"kind":"code","literal":"hev dispatch ls","chunkId":"steering#receipts"},{"kind":"code","literal":"STEER","chunkId":"steering#receipts"},{"kind":"code","literal":"q1 i0 a2","chunkId":"steering#receipts"},{"kind":"code","literal":"hev dispatch show","chunkId":"steering#receipts"},{"kind":"code","literal":"hev orchestrate show","chunkId":"steering#receipts"},{"kind":"code","literal":"hev peers","chunkId":"steering#receipts"}]
sources: [{"chunkId":"steering#receipts","url":"/docs/steering#receipts","anchor":"receipts"}]
---

Every steering message has an ID and a receipt that moves from queued to injected (with when, and at which turn and tool call) to acknowledged; reading an inbox file by hand never counts as acknowledged. Run listings show a steering column with counts, run and orchestrator detail views list each receipt with delivery latency, and the peers list flags unacknowledged steering.
