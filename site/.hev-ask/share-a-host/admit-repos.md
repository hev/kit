---
id: "share-a-host#admit-repos"
title: "Shared hosts"
heading: "Admit repos"
group: "Operate"
order: 276
url: "/docs/share-a-host#admit-repos"
anchor: "admit-repos"
terms: ["admit","repos","host","only","runs","work","against","admission","list","dispatch","config","table","lists","allowed","origin","urls","prefixes","members","permission","show","file","before","saving","keep","existing","settings","verify","loads","hosts","workspace","https","github","example","project","addfrom","toml","level","listing","real","still"]
hash: "bf40432c81833b1fb0c7d51c7746f47027f09268bb941a63f802d9b259215866"
mode: "agent-primary"
facts: [{"kind":"code","literal":"workspace = \"~/workspace\"\ncap = 4\n\n[admission]\nrepos = [\"https://github.com/example-org/my-project.git\"]\nadd_from = [\"https://github.com/example-org/\"]","chunkId":"share-a-host#admit-repos"},{"kind":"code","literal":"repos","chunkId":"share-a-host#admit-repos"},{"kind":"code","literal":"add_from","chunkId":"share-a-host#admit-repos"}]
sources: [{"chunkId":"share-a-host#admit-repos","url":"/docs/share-a-host#admit-repos","anchor":"admit-repos"}]
---

The host only runs work against repos on its admission list in its own dispatch config: an admission table lists allowed origin URLs and URL prefixes members with the repos permission may add from. Show the file before saving, keep existing settings and verify it loads with hev dispatch hosts.
