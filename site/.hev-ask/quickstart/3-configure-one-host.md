---
id: "quickstart#3-configure-one-host"
title: "Quickstart"
heading: "3. Configure one host"
group: "Start"
order: 348
url: "/docs/quickstart#3-configure-one-host"
anchor: "3-configure-one-host"
terms: ["configure","host","quickstart","step","dispatch","toml","naming","laptop","local","workspace","path","already","written","default","named","here","check","hosts","command","true","create","config","file","exists","show","before","changing","confirm","listed","reachable","free","slots","docs","https","hevkit","pages","llms","tell","whether","many"]
hash: "ad2d10656b6dbabf1952c52438898d72fc12715dc578263742514be40f4b2328"
mode: "agent-primary"
facts: [{"kind":"code","literal":"workspace = \"~/workspace\"\n\n[hosts.laptop]\nlocal = true\ncap = 2","chunkId":"quickstart#3-configure-one-host"},{"kind":"code","literal":"Create ~/.config/hev/dispatch.toml with workspace = \"~/workspace\" and one host, [hosts.laptop], with local = true and cap = 2. If the file already exists, show it to me before changing it. Then run `hev dispatch hosts` and confirm laptop is listed as reachable with free slots.\nDocs: https://hevkit.com/docs/quickstart#3-configure-one-host (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#3-configure-one-host"},{"kind":"code","literal":"hev dispatch hosts","chunkId":"quickstart#3-configure-one-host"},{"kind":"code","literal":"Run `hev dispatch hosts` and tell me, for each host in ~/.config/hev/dispatch.toml, whether it is reachable and how many free slots it has. If a host is unreachable, show the error.\nDocs: https://hevkit.com/docs/quickstart#3-configure-one-host (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#3-configure-one-host"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"quickstart#3-configure-one-host"},{"kind":"code","literal":"hev up","chunkId":"quickstart#3-configure-one-host"},{"kind":"code","literal":"here","chunkId":"quickstart#3-configure-one-host"},{"kind":"code","literal":"default_host = \"here\"","chunkId":"quickstart#3-configure-one-host"},{"kind":"code","literal":"laptop","chunkId":"quickstart#3-configure-one-host"},{"kind":"code","literal":"local = true","chunkId":"quickstart#3-configure-one-host"},{"kind":"code","literal":"cap","chunkId":"quickstart#3-configure-one-host"}]
sources: [{"chunkId":"quickstart#3-configure-one-host","url":"/docs/quickstart#3-configure-one-host","anchor":"3-configure-one-host"}]
---

Quickstart step 3: configure one host in dispatch.toml, naming the laptop as a local host with a workspace path and cap; hev up may already have written a default named here. Check with the hosts command.
