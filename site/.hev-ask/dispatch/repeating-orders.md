---
id: "dispatch#repeating-orders"
title: "Dispatch"
heading: "Repeating orders"
group: "Use"
order: 97
url: "/docs/dispatch#repeating-orders"
anchor: "repeating-orders"
terms: ["repeating","orders","frontmatter","fields","make","order","repeat","schedule","interval","stop","condition","maximum","lifetime","durations","without","units","show","confirms","after","dispatch","every","until","tracked","issues","closed","inbox","round","notes","json","true","reason","evidence","these","maxlifetime","lines","cadence","gave","dispatching","confirm","count"]
hash: "2708d7880c2a77f81a45233613c9fac4e37ce863d24c4250aee188e4331c90e5"
mode: "agent-primary"
facts: [{"kind":"code","literal":"every: 15m\nuntil: All tracked issues are closed\nmax_lifetime: 24h","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"every","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"max_lifetime","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"24h","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"until","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"inbox.md","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"round-notes.md","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"ls","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"show","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"schedule.json","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"round-N.log","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"until-met.json","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"{\"met\":true,\"reason\":\"evidence\"}","chunkId":"dispatch#repeating-orders"},{"kind":"code","literal":"stop","chunkId":"dispatch#repeating-orders"}]
sources: [{"chunkId":"dispatch#repeating-orders","url":"/docs/dispatch#repeating-orders","anchor":"repeating-orders"}]
---

Frontmatter fields make an order repeat on a schedule: an interval, a stop condition and a maximum lifetime. Durations are Go durations without day units, and show confirms the schedule after dispatch.
