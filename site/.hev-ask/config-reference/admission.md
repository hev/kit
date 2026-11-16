---
id: "config-reference#admission"
title: "Config reference"
heading: "[admission]"
group: "Reference"
order: 119
url: "/docs/config-reference#admission"
anchor: "admission"
terms: ["admission","reference","table","list","origin","urls","paths","host","accepts","absent","admits","empty","none","read","only","target","default","config","never","sender","chosen","alternate","repos","dispatch","toml","path","field","type","meaning","array","strings","admit","hevdispatchconfig","configpath","matched"]
hash: "9878b7bb35b104033eb204dba2726385cf93b76b5912ca3f1f7000d7011e3ab1"
mode: "source-primary"
facts: [{"kind":"code","literal":"[admission]","chunkId":"config-reference#admission"},{"kind":"code","literal":"repos","chunkId":"config-reference#admission"},{"kind":"code","literal":"[]","chunkId":"config-reference#admission"},{"kind":"code","literal":"HEV_DISPATCH_CONFIG","chunkId":"config-reference#admission"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"config-reference#admission"},{"kind":"code","literal":"--config","chunkId":"config-reference#admission"},{"kind":"code","literal":"config_path","chunkId":"config-reference#admission"}]
sources: [{"chunkId":"config-reference#admission","url":"/docs/config-reference#admission","anchor":"admission"}]
---

Reference for the admission table: a list of origin URLs or paths this host accepts, where an absent key admits all and an empty list admits none. It is read only from the target's default config, never from a sender-chosen or per-host alternate config.
