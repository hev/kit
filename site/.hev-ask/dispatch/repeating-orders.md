---
id: "dispatch#repeating-orders"
title: "Dispatch"
heading: "Repeating orders"
group: "Use"
order: 153
url: "/docs/dispatch#repeating-orders"
anchor: "repeating-orders"
terms: ["repeating","orders","frontmatter","fields","cadence","optional","stop","condition","maximum","lifetime","make","order","repeat","schedule","rounds","start","fresh","harness","same","worktree","never","overlap","skip","missed","ticks","hold","host","slot","even","while","waiting","pass","notes","inbox","messages","between","scheduling","ends","worker","records"]
hash: "2708d7880c2a77f81a45233613c9fac4e37ce863d24c4250aee188e4331c90e5"
mode: "agent-primary"
facts: [{"kind":"code","literal":"every: 15m\nuntil: All tracked issues are closed\nmax_lifetime: 24h","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"every","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"max_lifetime","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"24h","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"until","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"inbox.md","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"round-notes.md","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"ls","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"show","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"schedule.json","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"round-N.log","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"until-met.json","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"{\"met\":true,\"reason\":\"evidence\"}","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"stop","chunkId":"dispatch#repeating-orders"}]
sources: [{"chunkId":"dispatch#repeating-orders","url":"/docs/dispatch#repeating-orders","anchor":"repeating-orders"}]
---

Frontmatter fields for a cadence, an optional stop condition and a maximum lifetime make an order repeat on a schedule. Rounds start a fresh harness in the same worktree, never overlap, skip missed ticks, hold one host slot even while waiting, and pass notes and inbox messages between rounds. Scheduling ends when the worker records the stop condition as met or the lifetime (24 hours by default) runs out; rework and amendments keep the original deadline.
