---
id: "share-a-host#set-up-skills"
title: "Shared hosts"
heading: "Set up skills"
group: "Operate"
order: 249
url: "/docs/share-a-host#set-up-skills"
anchor: "set-up-skills"
terms: ["skills","host","setup","role","install","side","give","hosts","member","laptops","team","source","repo","subdirectory","machine","config","verify","command","lists","installed","reports","errors","docs","https","hevkit","share","pages","llms","sources","github","example","refs","heads","main","subdir","toml","dispatch","context","expedite","members"]
hash: "bdcbeeca69cb4f05f10ad28bba899bdadcc9c80256b4f00cdd714ea2ce9a77ca"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev skills setup --role host","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"Install hev's host-side skills on this machine: run `hev skills setup --role host`. Verify the command lists the skills it installed and reports no errors.\nDocs: https://hevkit.com/docs/share-a-host#set-up-skills (all pages: https://hevkit.com/llms.txt)","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"[[skills.sources]]\nurl = \"https://github.com/example-org/skills.git\"\nref = \"refs/heads/main\"\nsubdir = \"skills\"","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev skills setup","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev-member","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev-dispatch","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev-context","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"hev-expedite","chunkId":"share-a-host#set-up-skills"},{"kind":"code","literal":"--role host","chunkId":"share-a-host#set-up-skills"}]
sources: [{"chunkId":"share-a-host#set-up-skills","url":"/docs/share-a-host#set-up-skills","anchor":"set-up-skills"}]
---

On the host run the skills setup for the host role to install hev's host-side skills. To give hosts and member laptops team skills, add a skills source with a repo URL, ref and subdirectory to each machine's config.
