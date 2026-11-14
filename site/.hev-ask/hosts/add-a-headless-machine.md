---
id: "hosts#add-a-headless-machine"
title: "Hosts"
heading: "Add a headless machine"
group: "Operate"
order: 167
url: "/docs/hosts#add-a-headless-machine"
anchor: "add-a-headless-machine"
terms: ["headless","machine","target","needs","same","version","logged","harness","tmux","unix","cloned","repos","origin","under","workspace","based","known","host","push","credentials","runs","open","daemon","there","capture","traces","since","doesn","sandbox","workers","consider","giving","identity","scoped","revocable","keys","codex","claude","token","sending"]
hash: "cbc0a245778504740c0fcc91f4115d12516264fa7380c2e9abfba68a9856235a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"codex","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"claude","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"~/workspace","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"origin","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"gh","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"GH_TOKEN","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"hev up","chunkId":"hosts#add-a-headless-machine"}]
sources: [{"chunkId":"hosts#add-a-headless-machine","url":"/docs/hosts#add-a-headless-machine","anchor":"add-a-headless-machine"}]
---

A headless target needs the same hev version, a logged-in harness, Git, tmux on Unix, pre-cloned repos with origin under the workspace, key-based SSH with a known host key, and push credentials if runs open PRs. Run the daemon there to capture traces. Since kit doesn't sandbox workers, consider giving the machine its own bot identity and scoped, revocable keys.
