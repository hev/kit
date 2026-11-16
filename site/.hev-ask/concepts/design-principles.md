---
id: "concepts#design-principles"
title: "Concepts"
heading: "Design principles"
group: "Use"
order: 103
url: "/docs/concepts#design-principles"
anchor: "design-principles"
terms: ["design","principles","behind","binary","serving","every","role","plain","files","host","readable","hand","central","scheduler","senders","talk","target","keeps","queue","first","shared","archive","teams","receiving","deciding","admission","wrappers","forwarded","environment","notifications","being","trace","query","instead","reading","logs","these","visible","code","explain"]
hash: "dd4067f7bbdcac01b27029574c45b1fc167b0857d4460b1048b6b40c1282a080"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev","chunkId":"concepts#design-principles"},{"kind":"code","literal":"cat","chunkId":"concepts#design-principles"}]
sources: [{"chunkId":"concepts#design-principles","url":"/docs/concepts#design-principles","anchor":"design-principles"}]
---

The design principles behind hev kit: one Go binary serving every role, plain files per host readable by hand, no central scheduler (senders talk to each target over SSH and each host keeps its own queue), files first with a shared archive for teams, the receiving host deciding admission, wrappers, forwarded environment and notifications, and every run being a trace you can query instead of reading logs.
