---
id: "context#narrow-a-file-by-heading"
title: "Composable context"
heading: "Narrow a file by heading"
group: "Use"
order: 139
url: "/docs/context#narrow-a-file-by-heading"
anchor: "narrow-a-file-by-heading"
terms: ["narrow","file","heading","recipe","source","keep","drop","sections","markdown","instead","taking","whole","selectors","headings","written","exactly","appear","section","includes","subsections","keeping","retains","preamble","parent","dropping","applies","after","selector","matches","nothing","left","empty","error","narrowing","only","cuts","never","rewrites","summarizes","deterministic"]
hash: "ced49976d09cdac28c19936a064b3decdfab94b4a4a00d0177852995b41db9ae"
mode: "agent-primary"
facts: [{"kind":"code","literal":"{\"path\": \"AGENTS.md\", \"target\": \"AGENTS.md\", \"drop\": [\"## Release process\", \"## Billing\"]}\n{\"path\": \"CLAUDE.md\", \"target\": \"CLAUDE.md\", \"keep\": [\"## Testing\", \"### Fixtures\"]}","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"\"## Testing\"","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"Testing","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"\"(preamble)\"","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"keep","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"### Fixtures","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"##","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"drop","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"build","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"context.md","chunkId":"context#narrow-a-file-by-heading"},{"kind":"code","literal":"(narrowed: N of M bytes)","chunkId":"context#narrow-a-file-by-heading"},{"kind":"value","literal":"level-2","chunkId":"context#narrow-a-file-by-heading"}]
sources: [{"chunkId":"context#narrow-a-file-by-heading","url":"/docs/context#narrow-a-file-by-heading","anchor":"narrow-a-file-by-heading"}]
---

A recipe source can keep or drop sections of a Markdown file by heading instead of taking the file whole. Selectors are headings written exactly as they appear, a section includes its subsections, keeping retains the preamble and parent headings, and dropping applies after keeping; a selector that matches nothing, or a file left empty, is an error. Narrowing only cuts, never rewrites or summarizes, is deterministic, and build output and the review page show sizes and the sections kept and dropped.
