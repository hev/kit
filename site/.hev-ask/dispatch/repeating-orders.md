---
id: "dispatch#repeating-orders"
title: "Dispatch"
heading: "Repeating orders"
group: "Use"
order: 91
url: "/docs/dispatch#repeating-orders"
anchor: "repeating-orders"
terms: ["repeating","orders","repeat","cadence","optional","stop","condition","worker","checks","maximum","lifetime","defaulting","rounds","start","fresh","harness","sessions","same","worktree","without","overlapping","keep","host","slot","read","inbox","round","notes","ends","stopped","every","until","tracked","issues","closed","show","schedule","json","true","reason"]
hash: "2708d7880c2a77f81a45233613c9fac4e37ce863d24c4250aee188e4331c90e5"
mode: "agent-primary"
facts: [{"kind":"code","literal":"every: 15m\nuntil: All tracked issues are closed\nmax_lifetime: 24h","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"every","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"max_lifetime","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"24h","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"until","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"inbox.md","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"round-notes.md","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"ls","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"show","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"schedule.json","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"round-N.log","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"until-met.json","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"{\"met\":true,\"reason\":\"evidence\"}","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"stop","chunkId":"dispatch#repeating-orders"}]
sources: [{"chunkId":"dispatch#repeating-orders","url":"/docs/dispatch#repeating-orders","anchor":"repeating-orders"}]
---

Orders can repeat on a cadence with an optional stop condition the worker checks and a maximum lifetime defaulting to a day. Rounds start fresh harness sessions in the same worktree without overlapping, keep the host slot, read inbox and round notes, and stop when the condition is met, the lifetime ends, or the run is stopped.
