---
id: "concepts#design-principles"
title: "Concepts"
heading: "Design principles"
group: "Use"
order: 47
url: "/docs/concepts#design-principles"
anchor: "design-principles"
terms: ["design","principles","binary","everything","plain","toml","markdown","json","files","host","central","scheduler","senders","talk","queues","work","first","optional","shared","archive","receiving","deciding","policy","every","traced","these","visible","code","explain","most","behavior","daemon","dispatch","supervisor","remote","call","install","once","machine","config"]
hash: "b1eee20e200f923bc22aebe03aefb88e16ccab19f969abca1aa1ff2a3c450789"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev","chunkId":"concepts#design-principles"},{"kind":"code","literal":"cat","chunkId":"concepts#design-principles"}]
sources: [{"chunkId":"concepts#design-principles","url":"/docs/concepts#design-principles","anchor":"design-principles"}]
---

hev kit's design: one Go binary for everything, plain TOML and Markdown or JSON files per host, no central scheduler (senders talk to each host over SSH and each host queues its own work), files first with an optional shared archive, the receiving host deciding policy, and every run traced.
