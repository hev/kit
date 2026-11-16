---
id: "context#1-assess-the-project"
title: "Composable context"
heading: "1. Assess the project"
group: "Use"
order: 126
url: "/docs/context#1-assess-the-project"
anchor: "1-assess-the-project"
terms: ["assess","project","step","worked","example","find","every","agent","instruction","skill","file","write","assessment","baseline","recipe","including","everything","improved","without","exact","duplicates","findings","include","repeated","lines","oversized","files","missing","scope","broken","local","links","opposite","directives","checks","simple","word","matches","never","change"]
hash: "ca8985a7099cbb8686e6ef7a8c4f8ae0606cbdbe952f07774b9be70c6b68f219"
mode: "agent-primary"
facts: [{"kind":"code","literal":"cd ~/workspace/my-project\nhev context assess --root . --out /tmp/my-project-assessment","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"assess","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"AGENTS.md","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"CLAUDE.md","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":".claude/agents/","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":".claude/skills/","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":".agents/skills/","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"assessment.json","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"Scope:","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"Always run tests.","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"Never run tests.","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"recipe.json","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"improved.recipe.json","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"hev context extract","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"NEW/sources","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"--root NEW/sources","chunkId":"context#1-assess-the-project"}]
sources: [{"chunkId":"context#1-assess-the-project","url":"/docs/context#1-assess-the-project","anchor":"1-assess-the-project"}]
---

Step one of the worked example: assess the project to find every agent instruction and skill file and write an assessment, a baseline recipe including everything, and an improved recipe without exact duplicates. Findings include duplicates, repeated lines, oversized files, missing scope lines, broken local links and opposite directives; the checks are simple word matches that never change a source file and cannot say which of two conflicting instructions should win. An extract variant also copies the sources so bundles can be built apart from the project.
