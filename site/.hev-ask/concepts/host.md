---
id: "concepts#host"
title: "Concepts"
heading: "Host"
group: "Use"
order: 107
url: "/docs/concepts#host"
anchor: "host"
terms: ["host","machine","work","named","dispatch","config","sends","orders","either","local","reachable","simultaneous","runs","same","names","every","because","validators","targets","look","other","name","hosts","laptop","true","buildbox","toml","real","alias","place","differs","keep","already","file","confirm","both","listed","free","slots","docs"]
hash: "3fcf10cb4bdfb6bc646275c16e9d569f35fdde4f18fb966afa91127b287568f5"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[hosts.laptop]\nlocal = true\ncap = 2\n\n[hosts.buildbox]\nssh = \"buildbox\"\ncap = 8","chunkId":"concepts#host"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"concepts#host"},{"kind":"code","literal":"local = true","chunkId":"concepts#host"},{"kind":"code","literal":"ssh","chunkId":"concepts#host"},{"kind":"code","literal":"cap","chunkId":"concepts#host"}]
sources: [{"chunkId":"concepts#host","url":"/docs/concepts#host","anchor":"host"}]
---

A host is a machine that can run work, named in the dispatch config on the machine that sends orders; it is either the local machine or reachable by SSH, with a cap on simultaneous runs. Use the same host names in every machine's config, because validators and targets look each other up by name.
