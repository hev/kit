---
id: "failure-modes#the-config-is-rejected"
title: "Failure modes"
heading: "The config is rejected"
group: "Operate"
order: 158
url: "/docs/failure-modes#the-config-is-rejected"
anchor: "the-config-is-rejected"
terms: ["config","rejected","errors","include","unknown","keys","misspelled","unsupported","version","invalid","host","entries","name","below","neither","local","targets","starting","dash","undefined","default","override","hosts","admitting","work","only","requires","entry","valid","warns","about","others","target","warning","error","cause","field","does","know","starts"]
hash: "aedfae64a97c7e763247d31216c2d77c51dddfa75220fe187886adc43b32e4c9"
mode: "agent-primary"
facts: [{"kind":"code","literal":"unknown config key: KEY","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"hev","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"invalid host NAME","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"cap","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"local","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"ssh","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"invalid ssh target","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"unknown default_host NAME","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"default_host","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"unknown host NAME","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"--host","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"warning:","chunkId":"failure-modes#the-config-is-rejected"}]
sources: [{"chunkId":"failure-modes#the-config-is-rejected","url":"/docs/failure-modes#the-config-is-rejected","anchor":"the-config-is-rejected"}]
---

Config errors include unknown keys (misspelled or unsupported by this version), invalid host entries (bad name, cap below 1, neither local nor SSH), SSH targets starting with a dash, and undefined default or override hosts. A host admitting work only requires its own entry to be valid and warns about others.
