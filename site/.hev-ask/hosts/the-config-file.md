---
id: "hosts#the-config-file"
title: "Hosts"
heading: "The config file"
group: "Operate"
order: 127
url: "/docs/hosts#the-config-file"
anchor: "the-config-file"
terms: ["config","file","dispatch","read","flag","environment","variable","default","home","path","same","format","senders","targets","tilde","prefix","expands","reading","machine","toml","reads","first","these","command","hevdispatchconfig","directory","laptop","only","sends","work","needs","host","entries","receives","copy","admission","notifications","launch","settings","capacity"]
hash: "8b7f408c96452178f214a9248ac3373ab468c6862a45a34718eddb68588639fa"
mode: "agent-primary"
facts: [{"kind":"code","literal":"--config PATH","chunkId":"hosts#the-config-file"},{"kind":"code","literal":"hev dispatch","chunkId":"hosts#the-config-file"},{"kind":"code","literal":"HEV_DISPATCH_CONFIG","chunkId":"hosts#the-config-file"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"hosts#the-config-file"},{"kind":"code","literal":"~/","chunkId":"hosts#the-config-file"}]
sources: [{"chunkId":"hosts#the-config-file","url":"/docs/hosts#the-config-file","anchor":"the-config-file"}]
---

The dispatch config is read from a config flag, then an environment variable, then the default home path. The same format is used by senders and targets, and a tilde prefix expands to the reading machine's home.
