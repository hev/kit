---
id: "share-a-host#set-up-skills"
title: "Shared hosts"
heading: "Set up skills"
group: "Operate"
order: 324
url: "/docs/share-a-host#set-up-skills"
anchor: "set-up-skills"
terms: ["skills","install","host","side","role","guess","first","time","team","repo","added","config","every","machine","installed","pinned","commit","members","laptops","member","skill","instead","owner","only","dispatch","expedite","configuration","secrets","setup","verify","command","lists","reports","errors","docs","https","hevkit","share","pages","llms"]
hash: "850bff048cdbd554a9bb2458f882bea814f4246d752b28650bc938049f4f5c15"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev skills setup --role host","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"Install hev's host-side skills on this machine: run `hev skills setup --role host`. Verify the command lists the skills it installed and reports no errors.\nDocs: https://hevkit.com/docs/share-a-host#set-up-skills (all pages: https://hevkit.com/llms.txt)","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"[[skills.sources]]\nurl = \"https://github.com/example-org/skills.git\"\nref = \"refs/heads/main\"\nsubdir = \"skills\"","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev skills setup","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev-member","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev-dispatch","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev-expedite","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"--role host","chunkId":"share-a-host#set-up-skills"}]
sources: [{"chunkId":"share-a-host#set-up-skills","url":"/docs/share-a-host#set-up-skills","anchor":"set-up-skills"}]
---

Install hev's host-side skills on the host with the host role (it won't guess the role the first time). A team skills repo can be added to the config on every machine and is installed pinned to a commit; members' laptops get the member skill instead of owner-only dispatch and expedite skills. MCP configuration and secrets on the host are set up by the owner.
