---
id: "join-a-host#3-join-with-your-invite-code"
title: "Join a host"
heading: "3. Join with your invite code"
group: "Operate"
order: 200
url: "/docs/join-a-host#3-join-with-your-invite-code"
anchor: "3-join-with-your-invite-code"
terms: ["join","invite","code","redeem","valid","only","hours","received","through","trusted","channel","side","dedicated","host","pinned","there","trust","prompt","alias","node","entry","dispatch","config","prints","request","containing","public","send","owner","nodes","toml","file","came","already","works","once","expires","after","shared","replacing"]
hash: "6d9593cdbeb79090b366afd15292f55e82def67cb883348bd6dac8abba4d6bef"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev nodes join CODE","chunkId":"join-a-host#3-join-with-your-invite-code"},{"kind":"code","literal":"~/.ssh/","chunkId":"join-a-host#3-join-with-your-invite-code"},{"kind":"code","literal":"hev-node-…","chunkId":"join-a-host#3-join-with-your-invite-code"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"join-a-host#3-join-with-your-invite-code"},{"kind":"code","literal":"--config FILE","chunkId":"join-a-host#3-join-with-your-invite-code"}]
sources: [{"chunkId":"join-a-host#3-join-with-your-invite-code","url":"/docs/join-a-host#3-join-with-your-invite-code","anchor":"3-join-with-your-invite-code"}]
---

Redeem the one-use invite code (valid only for a few hours, and only if received through a trusted channel) to set up your side: a dedicated SSH key, the host's pinned key so there is no trust prompt, an SSH alias for the node, and a host entry in your dispatch config. It prints a join request containing only your public key, which you send to the owner.
