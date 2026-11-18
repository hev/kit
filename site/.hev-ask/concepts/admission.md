---
id: "concepts#admission"
title: "Concepts"
heading: "Admission"
group: "Use"
order: 99
url: "/docs/concepts#admission"
anchor: "admission"
terms: ["admission","target","decides","repositories","policy","dispatch","config","compares","list","clone","origin","remote","never","order","repo","label","missing","admits","everything","empty","nothing","sender","cannot","override","refused","orders","leave","failed","record","inspected","cleaned","repos","https","example","team","project","toml","show","clean","lives"]
hash: "7d8949c45e94c18f89ac6f4b61137786ec805e1db8a8c78194fbc7f4a7532e74"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[admission]\nrepos = [\"https://example.com/team/project.git\"]","chunkId":"concepts#admission"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"concepts#admission"},{"kind":"code","literal":"origin","chunkId":"concepts#admission"},{"kind":"code","literal":"repo","chunkId":"concepts#admission"},{"kind":"code","literal":"repos","chunkId":"concepts#admission"},{"kind":"code","literal":"repos = []","chunkId":"concepts#admission"},{"kind":"code","literal":"show","chunkId":"concepts#admission"},{"kind":"code","literal":"clean","chunkId":"concepts#admission"}]
sources: [{"chunkId":"concepts#admission","url":"/docs/concepts#admission","anchor":"admission"}]
---

Each target decides which repositories it will run, with the policy in its own dispatch config. It compares the list to its clone's origin remote, never the order's repo label; a missing list admits everything, an empty one admits nothing, and the sender cannot override it. Refused orders leave a failed record that can be inspected and cleaned.
