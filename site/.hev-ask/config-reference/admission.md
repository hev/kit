---
id: "config-reference#admission"
title: "Config reference"
heading: "[admission]"
group: "Reference"
order: 63
url: "/docs/config-reference#admission"
anchor: "admission"
terms: ["admission","repos","field","lists","origin","urls","paths","host","accepts","absent","admits","empty","none","read","only","target","default","config","never","sender","chosen","file","dispatch","toml","path","type","meaning","array","strings","admit","hevdispatchconfig","configpath","matched"]
hash: "9878b7bb35b104033eb204dba2726385cf93b76b5912ca3f1f7000d7011e3ab1"
mode: "source-primary"
facts: [{"kind":"code","literal":"[admission]","chunkId":"config-reference#admission"},{"kind":"code","literal":"repos","chunkId":"config-reference#admission"},{"kind":"code","literal":"[]","chunkId":"config-reference#admission"},{"kind":"code","literal":"HEV_DISPATCH_CONFIG","chunkId":"config-reference#admission"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"config-reference#admission"},{"kind":"code","literal":"--config","chunkId":"config-reference#admission"},{"kind":"code","literal":"config_path","chunkId":"config-reference#admission"}]
sources: [{"chunkId":"config-reference#admission","url":"/docs/config-reference#admission","anchor":"admission"}]
---

The admission repos field lists origin URLs or paths a host accepts; absent admits all and empty admits none. It is read only from the target's default config, never from a sender-chosen file.
