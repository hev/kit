---
id: "secrets#status"
title: "Secrets"
heading: "Status"
group: "Operate"
order: 389
url: "/docs/secrets#status"
anchor: "status"
terms: ["status","secrets","command","json","output","reports","host","secret","manager","plus","store","credential","type","reachability","quota","budget","information","refresh","programs","shows","there","none","never","value","identifier","last","real","request","hits","misses","requests","cooldown","unconfigured","says","whether","daemon","since","cache","start","many"]
hash: "49c04500d4557b3c6310d77fc90624a46a476c7580f04f1981db1bb528dd960a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets status","chunkId":"secrets#status"},{"kind":"code","literal":"--json","chunkId":"secrets#status"},{"kind":"code","literal":"secret://","chunkId":"secrets#status"},{"kind":"code","literal":"manager","chunkId":"secrets#status"},{"kind":"code","literal":"--refresh-quota","chunkId":"secrets#status"}]
sources: [{"chunkId":"secrets#status","url":"/docs/secrets#status","anchor":"status"}]
---

The secrets status command (with JSON output) reports the host's secret manager and where it is set, plus a row per store with credential type, reachability, and quota/budget information.
