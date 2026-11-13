---
id: "hosts#configure-the-receiving-side"
title: "Hosts"
heading: "Configure the receiving side"
group: "Operate"
order: 122
url: "/docs/hosts#configure-the-receiving-side"
anchor: "configure-the-receiving-side"
terms: ["configure","receiving","side","target","dispatch","config","workspace","capacity","notify","admission","settings","optional","none","admits","every","repo","sender","command","reply","name","state","summary","repos","https","example","team","project","hosts","buildbox","local","true","extra","token","toml","origin","harness","wrapper","repository","here","anything"]
hash: "813c3141f770e3f63773ca0163a06f32d8c5c99a9bf3213f08467d9dac3791fe"
mode: "agent-primary"
facts: [{"kind":"code","literal":"workspace = \"~/workspace\"\ncap = 8\nnotify = [\"notify-command\", \"{reply}\", \"dispatch {name}: {state} {summary}\"]\n\n[admission]\nrepos = [\"https://example.com/team/project.git\"]\n\n[hosts.buildbox]\nlocal = true\n\n[hosts.buildbox.env]\nEXTRA_API_TOKEN = \"\"","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"[admission] repos","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"origin","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"repos = []","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"[hosts.buildbox]","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"harness_wrapper","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"env","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"[hosts.buildbox.env]","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"notify","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"{name}","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"{state}","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"{summary}","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"{reply}","chunkId":"hosts#configure-the-receiving-side"}]
sources: [{"chunkId":"hosts#configure-the-receiving-side","url":"/docs/hosts#configure-the-receiving-side","anchor":"configure-the-receiving-side"}]
---

The target can have its own dispatch config with workspace, capacity, notify and admission settings. It is optional, with none the target admits every repo in its workspace, and its settings win over the sender's.
