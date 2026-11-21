---
id: "api/peers#list-peers"
title: "Peers API"
heading: "List peers"
group: "API"
order: 54
url: "/docs/api/peers#list-peers"
anchor: "list-peers"
terms: ["list","peers","lists","registered","agent","sessions","newest","last","seen","first","filterable","role","host","more","heartbeat","seconds","stale","hidden","unless","requested","curl","https","hevmind","claude","codex","authorization","bearer","include","true","offline","state","working","idle","draft","prompt","opencode","repo","since","peer","address"]
hash: "7fa3e356532659f0579f784f28cf30a3f3312233524f399da0cfa3e510dd61ed"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s \"https://kit.hevmind.com/v1/peers?role=foh\u0026agent=claude\u0026agent=codex\" \\\n  -H \"Authorization: Bearer $HEV_API_KEY\"","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"last_seen","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"stale","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"include_stale=true","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"host","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"role","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"foh","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"boh","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"offline","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"state","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"working","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"idle","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"draft","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"prompt","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"agent","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"claude","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"codex","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"pi","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"opencode","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"repo","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"since","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"Peer","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"id","chunkId":"api/peers#list-peers"},{"kind":"code","literal":"address","chunkId":"api/peers#list-peers"}]
sources: [{"chunkId":"api/peers#list-peers","url":"/docs/api/peers#list-peers","anchor":"list-peers"}]
---

Lists registered agent sessions newest last-seen first, filterable by role, agent, host and more. Peers with no heartbeat for 900 seconds are stale and hidden unless requested.
