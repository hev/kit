---
id: "hosts#configure-the-receiving-side"
title: "Hosts"
heading: "Configure the receiving side"
group: "Operate"
order: 178
url: "/docs/hosts#configure-the-receiving-side"
anchor: "configure-the-receiving-side"
terms: ["configure","receiving","side","target","optional","dispatch","config","whose","settings","anything","sender","says","none","admits","every","repository","workspace","sets","capacity","notify","command","admission","list","host","entry","same","name","uses","reads","harness","wrapper","names","environment","variables","forward","forwarded","take","their","values","reply"]
hash: "813c3141f770e3f63773ca0163a06f32d8c5c99a9bf3213f08467d9dac3791fe"
mode: "agent-primary"
facts: [{"kind":"code","literal":"workspace = \"~/workspace\"\ncap = 8\nnotify = [\"notify-command\", \"{reply}\", \"dispatch {name}: {state} {summary}\"]\n\n[admission]\nrepos = [\"https://example.com/team/project.git\"]\n\n[hosts.buildbox]\nlocal = true\n\n[hosts.buildbox.env]\nEXTRA_API_TOKEN = \"\"","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"[admission] repos","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"origin","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"repos = []","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"[hosts.buildbox]","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"harness_wrapper","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"env","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"[hosts.buildbox.env]","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"notify","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"{name}","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"{state}","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"{summary}","chunkId":"hosts#configure-the-receiving-side"},{"kind":"code","literal":"{reply}","chunkId":"hosts#configure-the-receiving-side"}]
sources: [{"chunkId":"hosts#configure-the-receiving-side","url":"/docs/hosts#configure-the-receiving-side","anchor":"configure-the-receiving-side"}]
---

The target can have its own optional dispatch config, whose settings win over anything the sender says; with none it admits every repository in its workspace. It sets the workspace, capacity, notify command, admission list, and a host entry using the same name the sender uses, from which it reads its harness wrapper and the names of environment variables to forward. Forwarded variables take their values from the target's own environment at admission.
