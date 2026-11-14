---
id: "concepts#admission"
title: "Concepts"
heading: "Admission"
group: "Use"
order: 98
url: "/docs/concepts#admission"
anchor: "admission"
terms: ["admission","target","decides","repositories","runs","list","dispatch","config","compared","clone","origin","rather","order","repo","label","missing","admits","everything","empty","nothing","sender","cannot","override","refused","orders","leave","failed","record","repos","https","example","team","project","toml","show","clean","policy","lives","host","limit"]
hash: "7d8949c45e94c18f89ac6f4b61137786ec805e1db8a8c78194fbc7f4a7532e74"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[admission]\nrepos = [\"https://example.com/team/project.git\"]","chunkId":"concepts#admission"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"concepts#admission"},{"kind":"code","literal":"origin","chunkId":"concepts#admission"},{"kind":"code","literal":"repo","chunkId":"concepts#admission"},{"kind":"code","literal":"repos","chunkId":"concepts#admission"},{"kind":"code","literal":"repos = []","chunkId":"concepts#admission"},{"kind":"code","literal":"show","chunkId":"concepts#admission"},{"kind":"code","literal":"clean","chunkId":"concepts#admission"}]
sources: [{"chunkId":"concepts#admission","url":"/docs/concepts#admission","anchor":"admission"}]
---

Each target decides which repositories it runs via an admission list in its own dispatch config, compared to the clone's origin rather than the order's repo label. A missing list admits everything, an empty list admits nothing, the sender cannot override it, and refused orders leave a failed record.
