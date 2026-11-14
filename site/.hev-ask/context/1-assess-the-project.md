---
id: "context#1-assess-the-project"
title: "Composable context"
heading: "1. Assess the project"
group: "Use"
order: 125
url: "/docs/context#1-assess-the-project"
anchor: "1-assess-the-project"
terms: ["assess","project","assessing","finds","every","agent","instruction","file","depth","plus","skills","agents","writes","assessment","findings","duplicates","repeated","lines","oversized","files","broken","links","contradictory","directives","baseline","recipe","improved","without","duplicate","checks","simple","word","matches","never","modify","sources","workspace","context","root","claude"]
hash: "250956636e1fdf8eb0114fbd7bf97038f508e06be9a94eb092fefd5d2eec1108"
mode: "agent-primary"
facts: [{"kind":"code","literal":"cd ~/workspace/my-project\nhev context assess --root . --out /tmp/my-project-assessment","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"assess","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"AGENTS.md","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"CLAUDE.md","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":".claude/","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":".agents/","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"assessment.json","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"Always run tests.","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"Never run tests.","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"recipe.json","chunkId":"context#1-assess-the-project"},{"kind":"code","literal":"improved.recipe.json","chunkId":"context#1-assess-the-project"}]
sources: [{"chunkId":"context#1-assess-the-project","url":"/docs/context#1-assess-the-project","anchor":"1-assess-the-project"}]
---

Assessing a project finds every agent instruction file at any depth plus skills and agents, and writes an assessment with findings (duplicates, repeated lines, oversized files, broken links, contradictory directives), a baseline recipe and an improved recipe without duplicate files. Checks are simple word matches and never modify sources.
