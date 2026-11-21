---
id: "clustering#admit-repos"
title: "Clustering"
heading: "Admit repos"
group: "Operate"
order: 113
url: "/docs/clustering#admit-repos"
anchor: "admit-repos"
terms: ["admit","repos","host","admission","repo","list","optional","addfrom","prefixes","dispatch","config","only","runs","work","listed","workspace","https","github","example","project","against","toml","level","table","listing","origin","urls","members","permission","real","show","file","before","saving","keep","existing","hosts","settings","verify","still"]
hash: "6123db4dfac517c254f9c3c416ec320544adf08b0308ba2adfae01b5314db8e0"
mode: "agent-primary"
facts: [{"kind":"code","literal":"workspace = \"~/workspace\"\ncap = 4\n\n[admission]\nrepos = [\"https://github.com/example-org/my-project.git\"]\nadd_from = [\"https://github.com/example-org/\"]","chunkId":"clustering#admit-repos"},{"kind":"code","literal":"repos","chunkId":"clustering#admit-repos"},{"kind":"code","literal":"add_from","chunkId":"clustering#admit-repos"}]
sources: [{"chunkId":"clustering#admit-repos","url":"/docs/clustering#admit-repos","anchor":"admit-repos"}]
---

On the host, set the admission repo list (and optional addfrom prefixes) in its dispatch config so it only runs work for listed repos.
