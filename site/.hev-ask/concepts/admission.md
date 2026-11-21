---
id: "concepts#admission"
title: "Concepts"
heading: "Admission"
group: "Use"
order: 135
url: "/docs/concepts#admission"
anchor: "admission"
terms: ["admission","target","decides","repositories","policy","dispatch","config","table","repos","https","example","team","project","toml","origin","repo","show","clean","lives","host","limit","section","replaced","exact","clone","want","admitted","read","remote","keep","rest","file","final","remind","missing","admits","everything","nothing","docs","hevkit"]
hash: "7d8949c45e94c18f89ac6f4b61137786ec805e1db8a8c78194fbc7f4a7532e74"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[admission]\nrepos = [\"https://example.com/team/project.git\"]","chunkId":"concepts#admission"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"concepts#admission"},{"kind":"code","literal":"origin","chunkId":"concepts#admission"},{"kind":"code","literal":"repo","chunkId":"concepts#admission"},{"kind":"code","literal":"repos","chunkId":"concepts#admission"},{"kind":"code","literal":"repos = []","chunkId":"concepts#admission"},{"kind":"code","literal":"show","chunkId":"concepts#admission"},{"kind":"code","literal":"clean","chunkId":"concepts#admission"}]
sources: [{"chunkId":"concepts#admission","url":"/docs/concepts#admission","anchor":"admission"}]
---

Each target decides which repositories it will run, with a policy in its own dispatch config's admission table.
