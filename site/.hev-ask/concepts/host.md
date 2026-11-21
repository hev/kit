---
id: "concepts#host"
title: "Concepts"
heading: "Host"
group: "Use"
order: 144
url: "/docs/concepts#host"
anchor: "host"
terms: ["host","machine","work","node","cluster","named","sender","dispatch","config","local","reached","capacity","hosts","laptop","true","buildbox","toml","name","sends","orders","real","alias","place","differs","keep","already","file","confirm","both","listed","reachable","free","slots","docs","https","hevkit","concepts","pages","llms","either"]
hash: "1258de4da8139db2d65e552f17cdeb6c672516ee8bd329524f8ce89afd2a4554"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[hosts.laptop]\nlocal = true\ncap = 2\n\n[hosts.buildbox]\nssh = \"buildbox\"\ncap = 8","chunkId":"concepts#host"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"concepts#host"},{"kind":"code","literal":"local = true","chunkId":"concepts#host"},{"kind":"code","literal":"ssh","chunkId":"concepts#host"},{"kind":"code","literal":"cap","chunkId":"concepts#host"}]
sources: [{"chunkId":"concepts#host","url":"/docs/concepts#host","anchor":"host"}]
---

A host is a machine that can run work, a node in your cluster, named in the sender's dispatch config as local or SSH-reached with a capacity.
