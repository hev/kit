---
id: "dispatch#repeating-orders"
title: "Dispatch"
heading: "Repeating orders"
group: "Use"
order: 146
url: "/docs/dispatch#repeating-orders"
anchor: "repeating-orders"
terms: ["repeating","orders","repeat","schedule","cadence","optional","stop","condition","worker","checks","maximum","lifetime","hours","default","rounds","start","fresh","harness","sessions","same","worktree","without","overlapping","hold","host","slot","while","waiting","carry","notes","inbox","messages","between","expires","stopped","every","until","tracked","issues","closed"]
hash: "2708d7880c2a77f81a45233613c9fac4e37ce863d24c4250aee188e4331c90e5"
mode: "agent-primary"
facts: [{"kind":"code","literal":"every: 15m\nuntil: All tracked issues are closed\nmax_lifetime: 24h","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"every","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"max_lifetime","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"24h","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"until","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"inbox.md","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"round-notes.md","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"ls","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"show","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"schedule.json","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"round-N.log","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"until-met.json","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"{\"met\":true,\"reason\":\"evidence\"}","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"stop","chunkId":"dispatch#repeating-orders"}]
sources: [{"chunkId":"dispatch#repeating-orders","url":"/docs/dispatch#repeating-orders","anchor":"repeating-orders"}]
---

Orders can repeat on a schedule with a cadence, an optional stop condition the worker checks, and a maximum lifetime (24 hours by default). Rounds start fresh harness sessions in the same worktree without overlapping, hold a host slot while waiting, carry notes and inbox messages between rounds, and end when the condition is met, the lifetime expires, or the run is stopped.
