---
id: "share-a-host#set-up-skills"
title: "Shared hosts"
heading: "Set up skills"
group: "Operate"
order: 338
url: "/docs/share-a-host#set-up-skills"
anchor: "set-up-skills"
terms: ["skills","host","install","role","must","stated","explicitly","first","time","distribute","team","repo","source","config","machine","setup","installs","pinned","commit","members","laptops","member","skill","instead","owner","only","ones","configuration","secrets","side","verify","command","lists","installed","reports","errors","docs","https","hevkit","share"]
hash: "5e7984dfd6d102b5d8a0f46d6f8836cdb8134dc82e80de2a9ab9a4b59992b805"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev skills setup --role host","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"Install hev's host-side skills on this machine: run `hev skills setup --role host`. Verify the command lists the skills it installed and reports no errors.\nDocs: https://hevkit.com/docs/share-a-host#set-up-skills (all pages: https://hevkit.com/llms.txt)","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"[[skills.sources]]\nurl = \"https://github.com/example-org/skills.git\"\nref = \"refs/heads/main\"\nsubdir = \"skills\"","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev skills setup","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev-member","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev-dispatch","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev-context","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev-expedite","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"--role host","chunkId":"share-a-host#set-up-skills"}]
sources: [{"chunkId":"share-a-host#set-up-skills","url":"/docs/share-a-host#set-up-skills","anchor":"set-up-skills"}]
---

On the host, install hev's skills for the host role, which must be stated explicitly the first time. To distribute your team's own skills, add a skills repo source to the config on each machine and setup installs it pinned to a commit; members' laptops get the member skill instead of owner-only ones, and MCP configuration and secrets on the host are set up by you.
