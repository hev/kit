---
id: "hosts#add-a-headless-machine"
title: "Hosts"
heading: "Add a headless machine"
group: "Operate"
order: 112
url: "/docs/hosts#add-a-headless-machine"
anchor: "add-a-headless-machine"
terms: ["headless","machine","target","needs","same","version","logged","harness","under","user","tmux","windows","equivalents","cloned","repos","origin","workspace","based","known","host","push","credentials","orders","open","does","sandbox","workers","many","teams","give","identity","scoped","revocable","keys","codex","claude","token","sending","side","reach"]
hash: "cbc0a245778504740c0fcc91f4115d12516264fa7380c2e9abfba68a9856235a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"codex","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"claude","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"~/workspace","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"origin","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"gh","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"GH_TOKEN","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"hev up","chunkId":"hosts#add-a-headless-machine"}]
sources: [{"chunkId":"hosts#add-a-headless-machine","url":"/docs/hosts#add-a-headless-machine","anchor":"add-a-headless-machine"}]
---

A headless target needs the same hev version, a logged-in harness under the SSH user, Git and tmux (or Windows equivalents), pre-cloned repos with origin under the workspace, key-based SSH with a known host key, and push credentials if orders open PRs. Kit does not sandbox workers, so many teams give the machine its own bot identity and scoped revocable keys.
