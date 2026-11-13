---
id: "share-a-host#set-up-skills"
title: "Shared hosts"
heading: "Set up skills"
group: "Operate"
order: 280
url: "/docs/share-a-host#set-up-skills"
anchor: "set-up-skills"
terms: ["skills","shared","host","install","side","role","optionally","team","repo","source","machine","config","setup","installs","pinned","commit","beside","members","after","being","accepted","member","skill","instead","owner","only","dispatch","must","chosen","explicitly","first","time","configuration","secrets","hand","verify","command","lists","installed","reports"]
hash: "5e7984dfd6d102b5d8a0f46d6f8836cdb8134dc82e80de2a9ab9a4b59992b805"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev skills setup --role host","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"Install hev's host-side skills on this machine: run `hev skills setup --role host`. Verify the command lists the skills it installed and reports no errors.\nDocs: https://hevkit.com/docs/share-a-host#set-up-skills (all pages: https://hevkit.com/llms.txt)","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"[[skills.sources]]\nurl = \"https://github.com/example-org/skills.git\"\nref = \"refs/heads/main\"\nsubdir = \"skills\"","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev skills setup","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev-member","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev-dispatch","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev-context","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev-expedite","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"--role host","chunkId":"share-a-host#set-up-skills"}]
sources: [{"chunkId":"share-a-host#set-up-skills","url":"/docs/share-a-host#set-up-skills","anchor":"set-up-skills"}]
---

On a shared host, install hev's host-side skills with the host role, and optionally add a team skills repo as a source in each machine's config so setup installs it pinned to a commit beside hev's own skills. Members run setup after being accepted and get the member skill instead of the owner-only dispatch skills; the host role must be chosen explicitly the first time, and MCP configuration and secrets are set up on the host by hand.
