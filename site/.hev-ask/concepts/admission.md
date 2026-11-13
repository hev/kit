---
id: "concepts#admission"
title: "Concepts"
heading: "Admission"
group: "Use"
order: 43
url: "/docs/concepts#admission"
anchor: "admission"
terms: ["admission","target","decides","repositories","section","dispatch","config","missing","repos","admits","everything","empty","list","nothing","https","example","team","project","toml","origin","repo","show","clean","policy","lives","host","limit","replaced","exact","clone","want","admitted","read","remote","keep","rest","file","final","remind","docs"]
hash: "7d8949c45e94c18f89ac6f4b61137786ec805e1db8a8c78194fbc7f4a7532e74"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[admission]\nrepos = [\"https://example.com/team/project.git\"]","chunkId":"concepts#admission"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"concepts#admission"},{"kind":"code","literal":"origin","chunkId":"concepts#admission"},{"kind":"code","literal":"repo","chunkId":"concepts#admission"},{"kind":"code","literal":"repos","chunkId":"concepts#admission"},{"kind":"code","literal":"repos = []","chunkId":"concepts#admission"},{"kind":"code","literal":"show","chunkId":"concepts#admission"},{"kind":"code","literal":"clean","chunkId":"concepts#admission"}]
sources: [{"chunkId":"concepts#admission","url":"/docs/concepts#admission","anchor":"admission"}]
---

Each target decides which repositories it will run, via an admission section in its own dispatch config. A missing repos key admits everything and an empty list admits nothing.
