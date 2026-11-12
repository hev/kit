---
id: "share-a-host#admit-repos"
title: "Shared hosts"
heading: "Admit repos"
group: "Operate"
order: 264
url: "/docs/share-a-host#admit-repos"
anchor: "admit-repos"
terms: ["admit","repos","host","only","runs","work","against","admission","list","dispatch","config","lists","allowed","origin","urls","prefixes","members","permission","more","covers","workspace","https","github","example","project","addfrom","toml","level","table","listing","real","show","file","before","saving","keep","existing","hosts","settings","verify"]
hash: "bf40432c81833b1fb0c7d51c7746f47027f09268bb941a63f802d9b259215866"
mode: "agent-primary"
facts: [{"kind":"code","literal":"workspace = \"~/workspace\"\ncap = 4\n\n[admission]\nrepos = [\"https://github.com/example-org/my-project.git\"]\nadd_from = [\"https://github.com/example-org/\"]","chunkId":"share-a-host#admit-repos"},{"kind":"code","literal":"repos","chunkId":"share-a-host#admit-repos"},{"kind":"code","literal":"add_from","chunkId":"share-a-host#admit-repos"}]
sources: [{"chunkId":"share-a-host#admit-repos","url":"/docs/share-a-host#admit-repos","anchor":"admit-repos"}]
---

The host only runs work against repos on its admission list in its dispatch config, which lists allowed origin URLs and the URL prefixes from which members with the repos permission may add more. Admission covers repos only.
