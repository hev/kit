---
id: "concepts#design-principles"
title: "Concepts"
heading: "Design principles"
group: "Use"
order: 47
url: "/docs/concepts#design-principles"
anchor: "design-principles"
terms: ["design","principles","binary","plain","files","host","central","scheduler","sender","talks","queue","first","optional","layer","shared","archive","these","visible","code","explain","most","behavior","daemon","dispatch","supervisor","remote","every","call","install","once","machine","config","toml","home","directory","state","markdown","json","read","target"]
hash: "00f9797008e97da750e607e8f41e9759ba69c4073ac6b6b75073464e4d3c9a7e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev","chunkId":"concepts#design-principles"},{"kind":"code","literal":"cat","chunkId":"concepts#design-principles"}]
sources: [{"chunkId":"concepts#design-principles","url":"/docs/concepts#design-principles","anchor":"design-principles"}]
---

Design principles: one Go binary, plain files per host, no central scheduler (the sender talks to each host over SSH, each with its own queue), and files first with an optional hev layer key for a shared archive.
