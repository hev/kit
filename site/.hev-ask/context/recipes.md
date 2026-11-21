---
id: "context#recipes"
title: "Composable context"
heading: "Recipes"
group: "Use"
order: 182
url: "/docs/context#recipes"
anchor: "recipes"
terms: ["recipes","recipe","plain","json","version","task","sources","list","mapping","repo","paths","target","templating","update","docs","change","path","agents","claude","skills","style","skill","keep","drop","write","context","repository","listing","equal","only","files","exist","here","replace","absolute","backslashes","check","build","root","needs"]
hash: "f84a8fbcf639bf005c82bdcfcb2aa479c021763d1e0cc8b1f38dcdecf25b5717"
mode: "agent-primary"
facts: [{"kind":"code","literal":"{\n  \"version\": 1,\n  \"task\": \"Update the docs for a CLI change\",\n  \"sources\": [\n    {\"path\": \"AGENTS.md\", \"target\": \"AGENTS.md\"},\n    {\"path\": \"docs/AGENTS.md\", \"target\": \"docs/AGENTS.md\"},\n    {\"path\": \".claude/skills/docs-style/SKILL.md\", \"target\": \".claude/skills/docs-style/SKILL.md\"}\n  ]\n}","chunkId":"context#recipes"},{"kind":"code","literal":"path","chunkId":"context#recipes"},{"kind":"code","literal":"target","chunkId":"context#recipes"},{"kind":"code","literal":"sources","chunkId":"context#recipes"},{"kind":"code","literal":"task","chunkId":"context#recipes"},{"kind":"code","literal":"keep","chunkId":"context#recipes"},{"kind":"code","literal":"drop","chunkId":"context#recipes"},{"kind":"code","literal":"..","chunkId":"context#recipes"}]
sources: [{"chunkId":"context#recipes","url":"/docs/context#recipes","anchor":"recipes"}]
---

A recipe is plain JSON with version, task and a sources list mapping repo paths to target paths, with no templating.
