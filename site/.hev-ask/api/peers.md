---
id: "api/peers"
title: "Peers API"
heading: null
group: "API"
order: 53
url: "/docs/api/peers"
anchor: null
terms: ["peers","lists","live","agent","sessions","across","cluster","registers","heartbeats","presence","reads","status","sends","acknowledges","messages","rolling","train","club","accounts","local","state","everywhere","json","hosts","host","read","inbox","sent","jsonl","post","send","enqueue","receipts","consume","callout","astro","linkgrid","list","register","heartbeat"]
hash: "1177bac7e92401185dab46ae4d9f930a82040a340fee1812f27128da918ba24f"
mode: "source-primary"
facts: [{"kind":"code","literal":"/v1/peers","chunkId":"api/peers"},{"kind":"code","literal":"~/.hev/peers/","chunkId":"api/peers"},{"kind":"code","literal":"~/.local/state/peers-v2/","chunkId":"api/peers"},{"kind":"code","literal":"hev peers","chunkId":"api/peers"},{"kind":"code","literal":"hev-peers","chunkId":"api/peers"},{"kind":"code","literal":"GET /v1/peers","chunkId":"api/peers"},{"kind":"code","literal":"peers ls --everywhere --json","chunkId":"api/peers"},{"kind":"code","literal":"PUT /v1/peers/{id}","chunkId":"api/peers"},{"kind":"code","literal":"hosts/HOST.json","chunkId":"api/peers"},{"kind":"code","literal":"GET /v1/peers/{id}","chunkId":"api/peers"},{"kind":"code","literal":"peers read","chunkId":"api/peers"},{"kind":"code","literal":"GET /v1/peers/{id}/messages","chunkId":"api/peers"},{"kind":"code","literal":"peers inbox --json","chunkId":"api/peers"},{"kind":"code","literal":"sent.jsonl","chunkId":"api/peers"},{"kind":"code","literal":"POST /v1/peers/{id}/messages","chunkId":"api/peers"},{"kind":"code","literal":"peers send","chunkId":"api/peers"},{"kind":"code","literal":"POST /v1/peers/enqueue","chunkId":"api/peers"},{"kind":"code","literal":"POST /v1/peers/{id}/receipts","chunkId":"api/peers"},{"kind":"code","literal":"peers inbox consume","chunkId":"api/peers"},{"kind":"code","literal":"ack","chunkId":"api/peers"},{"kind":"value","literal":"Callout.astro","chunkId":"api/peers"},{"kind":"value","literal":"LinkGrid.astro","chunkId":"api/peers"},{"kind":"value","literal":"0.7","chunkId":"api/peers"}]
sources: [{"chunkId":"api/peers","url":"/docs/api/peers","anchor":null}]
---

The peers API lists live agent sessions across the cluster, registers and heartbeats presence, reads status, and sends, reads and acknowledges agent-to-agent messages. Rolling out on the 0.7 train for Club and Pro accounts.
