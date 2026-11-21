---
id: "clustering#configure-the-receiving-side"
title: "Clustering"
heading: "Configure the receiving side"
group: "Operate"
order: 116
url: "/docs/clustering#configure-the-receiving-side"
anchor: "configure-the-receiving-side"
terms: ["configure","receiving","side","target","dispatch","config","wins","anything","sender","says","without","admits","every","repo","workspace","covers","notify","settings","command","reply","name","state","summary","admission","repos","https","example","team","project","hosts","buildbox","local","true","extra","token","toml","origin","harness","wrapper","optional"]
hash: "b9ab9266cc0566a214ac893329525c55015421979bbd06d756f03ae55ae5a3a1"
mode: "agent-primary"
facts: [{"kind":"code","literal":"workspace = \"~/workspace\"\ncap = 8\nnotify = [\"notify-command\", \"{reply}\", \"dispatch {name}: {state} {summary}\"]\n\n[admission]\nrepos = [\"https://example.com/team/project.git\"]\n\n[hosts.buildbox]\nlocal = true\n\n[hosts.buildbox.env]\nEXTRA_API_TOKEN = \"\"","chunkId":"clustering#configure-the-receiving-side"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"clustering#configure-the-receiving-side"},{"kind":"code","literal":"[admission] repos","chunkId":"clustering#configure-the-receiving-side"},{"kind":"code","literal":"origin","chunkId":"clustering#configure-the-receiving-side"},{"kind":"code","literal":"repos = []","chunkId":"clustering#configure-the-receiving-side"},{"kind":"code","literal":"[hosts.buildbox]","chunkId":"clustering#configure-the-receiving-side"},{"kind":"code","literal":"harness_wrapper","chunkId":"clustering#configure-the-receiving-side"},{"kind":"code","literal":"env","chunkId":"clustering#configure-the-receiving-side"},{"kind":"code","literal":"[hosts.buildbox.env]","chunkId":"clustering#configure-the-receiving-side"},{"kind":"code","literal":"notify","chunkId":"clustering#configure-the-receiving-side"},{"kind":"code","literal":"{name}","chunkId":"clustering#configure-the-receiving-side"},{"kind":"code","literal":"{state}","chunkId":"clustering#configure-the-receiving-side"},{"kind":"code","literal":"{summary}","chunkId":"clustering#configure-the-receiving-side"},{"kind":"code","literal":"{reply}","chunkId":"clustering#configure-the-receiving-side"}]
sources: [{"chunkId":"clustering#configure-the-receiving-side","url":"/docs/clustering#configure-the-receiving-side","anchor":"configure-the-receiving-side"}]
---

The target may have its own dispatch config, which wins over anything the sender says; without one it admits every repo in its workspace. Covers workspace, cap and notify settings.
