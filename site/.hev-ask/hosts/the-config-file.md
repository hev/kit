---
id: "hosts#the-config-file"
title: "Hosts"
heading: "The config file"
group: "Operate"
order: 121
url: "/docs/hosts#the-config-file"
anchor: "the-config-file"
terms: ["config","file","dispatch","read","explicit","path","environment","variable","default","location","same","format","senders","targets","example","shows","hosts","reached","prefixes","windows","host","workspace","wrapper","local","unknown","keys","errors","toml","reads","first","these","command","hevdispatchconfig","prefix","expands","home","directory","machine","reading","laptop"]
hash: "8b7f408c96452178f214a9248ac3373ab468c6862a45a34718eddb68588639fa"
mode: "agent-primary"
facts: [{"kind":"code","literal":"--config PATH","chunkId":"hosts#the-config-file"},{"kind":"code","literal":"hev dispatch","chunkId":"hosts#the-config-file"},{"kind":"code","literal":"HEV_DISPATCH_CONFIG","chunkId":"hosts#the-config-file"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"hosts#the-config-file"},{"kind":"code","literal":"~/","chunkId":"hosts#the-config-file"}]
sources: [{"chunkId":"hosts#the-config-file","url":"/docs/hosts#the-config-file","anchor":"the-config-file"}]
---

The dispatch config is read from an explicit path, then an environment variable, then the default location, with the same format on senders and targets. An example shows hosts reached by SSH, a Mac with PATH prefixes, a Windows host with workspace and wrapper, and a local host; unknown keys are errors.
