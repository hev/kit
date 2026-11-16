---
id: "concepts#host"
title: "Concepts"
heading: "Host"
group: "Use"
order: 51
url: "/docs/concepts#host"
anchor: "host"
terms: ["host","machine","work","named","dispatch","config","local","reached","capacity","page","gives","prompt","adding","hosts","checking","reachable","laptop","true","buildbox","toml","name","sends","orders","real","alias","place","differs","keep","already","file","confirm","both","listed","free","slots","docs","https","hevkit","concepts","pages"]
hash: "3fcf10cb4bdfb6bc646275c16e9d569f35fdde4f18fb966afa91127b287568f5"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[hosts.laptop]\nlocal = true\ncap = 2\n\n[hosts.buildbox]\nssh = \"buildbox\"\ncap = 8","chunkId":"concepts#host"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"concepts#host"},{"kind":"code","literal":"local = true","chunkId":"concepts#host"},{"kind":"code","literal":"ssh","chunkId":"concepts#host"},{"kind":"code","literal":"cap","chunkId":"concepts#host"}]
sources: [{"chunkId":"concepts#host","url":"/docs/concepts#host","anchor":"host"}]
---

A host is a machine that can run work, named in the dispatch config as local or reached by SSH, with a capacity. The page gives a prompt for adding two hosts and checking they are reachable.
