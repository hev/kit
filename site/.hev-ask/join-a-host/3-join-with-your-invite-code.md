---
id: "join-a-host#3-join-with-your-invite-code"
title: "Join a host"
heading: "3. Join with your invite code"
group: "Operate"
order: 207
url: "/docs/join-a-host#3-join-with-your-invite-code"
anchor: "3-join-with-your-invite-code"
terms: ["join","invite","code","single","short","lived","only","came","owner","trusted","channel","joining","creates","host","specific","pins","adds","alias","dispatch","config","entry","prints","request","holding","public","send","nodes","node","toml","file","through","already","trust","works","once","expires","after","hours","shared","replacing"]
hash: "6d9593cdbeb79090b366afd15292f55e82def67cb883348bd6dac8abba4d6bef"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev nodes join CODE","chunkId":"join-a-host#3-join-with-your-invite-code"},{"kind":"code","literal":"~/.ssh/","chunkId":"join-a-host#3-join-with-your-invite-code"},{"kind":"code","literal":"hev-node-…","chunkId":"join-a-host#3-join-with-your-invite-code"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"join-a-host#3-join-with-your-invite-code"},{"kind":"code","literal":"--config FILE","chunkId":"join-a-host#3-join-with-your-invite-code"}]
sources: [{"chunkId":"join-a-host#3-join-with-your-invite-code","url":"/docs/join-a-host#3-join-with-your-invite-code","anchor":"3-join-with-your-invite-code"}]
---

Use a single-use, short-lived invite code only if it came from the owner over a trusted channel. Joining creates a host-specific SSH key, pins the host key from the code, adds an SSH alias and a dispatch config entry, and prints a request holding only your public key for you to send to the owner.
