---
id: "hosts#the-config-file"
title: "Hosts"
heading: "The config file"
group: "Operate"
order: 176
url: "/docs/hosts#the-config-file"
anchor: "the-config-file"
terms: ["config","file","dispatch","read","explicit","path","flag","environment","variable","default","home","prefix","expanded","reading","machine","same","format","serves","senders","host","entries","receivers","admission","notifications","launch","settings","example","shows","windows","local","hosts","unknown","keys","errors","toml","reads","first","these","command","hevdispatchconfig"]
hash: "8b7f408c96452178f214a9248ac3373ab468c6862a45a34718eddb68588639fa"
mode: "agent-primary"
facts: [{"kind":"code","literal":"--config PATH","chunkId":"hosts#the-config-file"},{"kind":"code","literal":"hev dispatch","chunkId":"hosts#the-config-file"},{"kind":"code","literal":"HEV_DISPATCH_CONFIG","chunkId":"hosts#the-config-file"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"hosts#the-config-file"},{"kind":"code","literal":"~/","chunkId":"hosts#the-config-file"}]
sources: [{"chunkId":"hosts#the-config-file","url":"/docs/hosts#the-config-file","anchor":"the-config-file"}]
---

The dispatch config is read from an explicit path flag, then an environment variable, then the default file, with the home prefix expanded on the reading machine. The same format serves senders (host entries) and receivers (admission, notifications, launch settings); an example shows SSH, Mac, Windows and local hosts. Unknown keys are errors.
