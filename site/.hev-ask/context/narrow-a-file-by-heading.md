---
id: "context#narrow-a-file-by-heading"
title: "Composable context"
heading: "Narrow a file by heading"
group: "Use"
order: 83
url: "/docs/context#narrow-a-file-by-heading"
anchor: "narrow-a-file-by-heading"
terms: ["narrow","file","heading","source","keep","drop","sections","instead","being","taken","whole","worker","fixing","thing","does","unrelated","process","billing","notes","path","agents","target","release","claude","testing","fixtures","preamble","build","context","narrowed","bytes","level","instruction","files","grow","sent","rarely","needs","their","headings"]
hash: "ced49976d09cdac28c19936a064b3decdfab94b4a4a00d0177852995b41db9ae"
mode: "agent-primary"
facts: [{"kind":"code","literal":"{\"path\": \"AGENTS.md\", \"target\": \"AGENTS.md\", \"drop\": [\"## Release process\", \"## Billing\"]}\n{\"path\": \"CLAUDE.md\", \"target\": \"CLAUDE.md\", \"keep\": [\"## Testing\", \"### Fixtures\"]}","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"\"## Testing\"","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"Testing","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"\"(preamble)\"","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"keep","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"### Fixtures","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"##","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"drop","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"build","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"context.md","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"(narrowed: N of M bytes)","chunkId":"context#narrow-a-file-by-heading"},{"kind":"value","literal":"level-2","chunkId":"context#narrow-a-file-by-heading"}]
sources: [{"chunkId":"context#narrow-a-file-by-heading","url":"/docs/context#narrow-a-file-by-heading","anchor":"narrow-a-file-by-heading"}]
---

A source can keep or drop sections by heading instead of being taken whole, so a worker fixing one thing does not get unrelated process or billing notes.
