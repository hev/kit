---
id: "clustering#set-up-skills"
title: "Clustering"
heading: "Set up skills"
group: "Operate"
order: 125
url: "/docs/clustering#set-up-skills"
anchor: "set-up-skills"
terms: ["skills","host","install","side","setup","command","role","machine","verify","lists","installed","reports","errors","docs","https","hevkit","clustering","pages","llms","sources","github","example","refs","heads","main","subdir","config","toml","member","dispatch","context","expedite","give","members","laptops","team","repo","entry","directory","holds"]
hash: "19c7549df330f68707f9c31902a4363968c4acae5926b210f19e0c3eab16e46e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev skills setup --role host","chunkId":"clustering#set-up-skills"},{"kind":"code","literal":"Install hev's host-side skills on this machine: run `hev skills setup --role host`. Verify the command lists the skills it installed and reports no errors.\nDocs: https://hevkit.com/docs/clustering#set-up-skills (all pages: https://hevkit.com/llms.txt)","chunkId":"clustering#set-up-skills"},{"kind":"code","literal":"[[skills.sources]]\nurl = \"https://github.com/example-org/skills.git\"\nref = \"refs/heads/main\"\nsubdir = \"skills\"","chunkId":"clustering#set-up-skills"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"clustering#set-up-skills"},{"kind":"code","literal":"hev skills setup","chunkId":"clustering#set-up-skills"},{"kind":"code","literal":"hev-member","chunkId":"clustering#set-up-skills"},{"kind":"code","literal":"hev-dispatch","chunkId":"clustering#set-up-skills"},{"kind":"code","literal":"hev-context","chunkId":"clustering#set-up-skills"},{"kind":"code","literal":"hev-expedite","chunkId":"clustering#set-up-skills"},{"kind":"code","literal":"--role host","chunkId":"clustering#set-up-skills"}]
sources: [{"chunkId":"clustering#set-up-skills","url":"/docs/clustering#set-up-skills","anchor":"set-up-skills"}]
---

On the host, install hev's host-side skills with the skills setup command using the host role.
