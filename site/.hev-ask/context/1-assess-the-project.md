---
id: "context#1-assess-the-project"
title: "Composable context"
heading: "1. Assess the project"
group: "Use"
order: 70
url: "/docs/context#1-assess-the-project"
anchor: "1-assess-the-project"
terms: ["assess","project","step","repository","agent","instructions","output","directory","writes","assessment","baseline","recipe","editable","improved","summarizes","duplicates","other","findings","workspace","context","root","agents","claude","skills","json","scope","always","tests","never","extract","sources","path","differs","must","exist","confirm","wrote","summarize","repeated","lines"]
hash: "ca8985a7099cbb8686e6ef7a8c4f8ae0606cbdbe952f07774b9be70c6b68f219"
mode: "agent-primary"
facts: [{"kind":"code","literal":"cd ~/workspace/my-project\nhev context assess --root . --out /tmp/my-project-assessment","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"assess","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"AGENTS.md","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"CLAUDE.md","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":".claude/agents/","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":".claude/skills/","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":".agents/skills/","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"assessment.json","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"Scope:","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"Always run tests.","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"Never run tests.","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"recipe.json","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"improved.recipe.json","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"hev context extract","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"NEW/sources","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"--root NEW/sources","chunkId":"context#1-assess-the-project"}]
sources: [{"chunkId":"context#1-assess-the-project","url":"/docs/context#1-assess-the-project","anchor":"1-assess-the-project"}]
---

Step one: assess the repository's agent instructions into a new output directory, which writes an assessment, a baseline recipe and an editable improved recipe, and summarizes duplicates and other findings.
