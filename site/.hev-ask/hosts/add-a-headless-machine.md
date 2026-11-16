---
id: "hosts#add-a-headless-machine"
title: "Hosts"
heading: "Add a headless machine"
group: "Operate"
order: 174
url: "/docs/hosts#add-a-headless-machine"
anchor: "add-a-headless-machine"
terms: ["headless","machine","target","needs","same","version","sender","harness","installed","logged","user","tmux","unix","workspace","repository","already","cloned","origin","since","dispatch","never","clones","based","known","host","push","credentials","orders","open","daemon","there","want","runs","traces","because","does","sandbox","workers","consider","giving"]
hash: "cbc0a245778504740c0fcc91f4115d12516264fa7380c2e9abfba68a9856235a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"codex","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"claude","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"~/workspace","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"origin","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"gh","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"GH_TOKEN","chunkId":"hosts#add-a-headless-machine"},{"kind":"code","literal":"hev up","chunkId":"hosts#add-a-headless-machine"}]
sources: [{"chunkId":"hosts#add-a-headless-machine","url":"/docs/hosts#add-a-headless-machine","anchor":"add-a-headless-machine"}]
---

What a headless target needs: the same hev version as the sender, the harness installed and logged in as the SSH user, Git (and tmux on Unix), a workspace with each repository already cloned with an origin since dispatch never clones, key-based SSH with a known host key, and push credentials if orders open PRs. Run the daemon there too if you want its runs in your traces. Because kit does not sandbox workers, consider giving the machine its own bot identity with scoped, revocable credentials.
