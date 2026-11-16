---
id: "failure-modes#the-config-is-rejected"
title: "Failure modes"
heading: "The config is rejected"
group: "Operate"
order: 165
url: "/docs/failure-modes#the-config-is-rejected"
anchor: "the-config-is-rejected"
terms: ["config","rejected","errors","their","causes","unknown","misspelled","unsupported","version","invalid","host","entry","target","starting","dash","default","override","naming","undefined","reads","admit","only","must","valid","problems","other","entries","warned","about","skipped","name","local","warning","error","cause","field","does","know","below","neither"]
hash: "aedfae64a97c7e763247d31216c2d77c51dddfa75220fe187886adc43b32e4c9"
mode: "agent-primary"
facts: [{"kind":"code","literal":"unknown config key: KEY","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"hev","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"invalid host NAME","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"cap","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"local","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"ssh","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"invalid ssh target","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"unknown default_host NAME","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"default_host","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"unknown host NAME","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"--host","chunkId":"failure-modes#the-config-is-rejected"},{"kind":"code","literal":"warning:","chunkId":"failure-modes#the-config-is-rejected"}]
sources: [{"chunkId":"failure-modes#the-config-is-rejected","url":"/docs/failure-modes#the-config-is-rejected","anchor":"the-config-is-rejected"}]
---

Config errors and their causes: an unknown key (misspelled or unsupported by this version), an invalid host entry, an SSH target starting with a dash, or a default host or host override naming an undefined host. When a host reads its own config to admit a run, only its own entry must be valid; problems in other entries are warned about and skipped.
