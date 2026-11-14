---
id: "quickstart#3-configure-one-host"
title: "Quickstart"
heading: "3. Configure one host"
group: "Start"
order: 281
url: "/docs/quickstart#3-configure-one-host"
anchor: "3-configure-one-host"
terms: ["configure","host","create","dispatch","config","sending","machine","workspace","local","laptop","itself","capacity","confirm","reachable","free","slots","hosts","true","toml","file","already","exists","show","before","changing","listed","docs","https","hevkit","quickstart","pages","llms","tell","whether","many","unreachable","error","reads","sends","work"]
hash: "47009778052e780f943aa8760f0fc3d1d32dacb8cc054fe6486f8742a6a0387c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"workspace = \"~/workspace\"\n\n[hosts.laptop]\nlocal = true\ncap = 2","chunkId":"quickstart#3-configure-one-host"},{"kind":"code","literal":"Create ~/.config/hev/dispatch.toml with workspace = \"~/workspace\" and one host, [hosts.laptop], with local = true and cap = 2. If the file already exists, show it to me before changing it. Then run `hev dispatch hosts` and confirm laptop is listed as reachable with free slots.\nDocs: https://hevkit.com/docs/quickstart#3-configure-one-host (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#3-configure-one-host"},{"kind":"code","literal":"hev dispatch hosts","chunkId":"quickstart#3-configure-one-host"},{"kind":"code","literal":"Run `hev dispatch hosts` and tell me, for each host in ~/.config/hev/dispatch.toml, whether it is reachable and how many free slots it has. If a host is unreachable, show the error.\nDocs: https://hevkit.com/docs/quickstart#3-configure-one-host (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#3-configure-one-host"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"quickstart#3-configure-one-host"},{"kind":"code","literal":"local = true","chunkId":"quickstart#3-configure-one-host"},{"kind":"code","literal":"cap","chunkId":"quickstart#3-configure-one-host"}]
sources: [{"chunkId":"quickstart#3-configure-one-host","url":"/docs/quickstart#3-configure-one-host","anchor":"3-configure-one-host"}]
---

Create the dispatch config on the sending machine with a workspace and one local host (the laptop itself) with a capacity of two, then confirm the host is reachable with free slots.
