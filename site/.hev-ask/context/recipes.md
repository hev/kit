---
id: "context#recipes"
title: "Composable context"
heading: "Recipes"
group: "Use"
order: 85
url: "/docs/context#recipes"
anchor: "recipes"
terms: ["recipes","recipe","plain","json","templating","version","task","list","sources","path","target","page","includes","prompt","write","update","docs","change","agents","claude","skills","style","skill","keep","drop","context","repository","listing","equal","only","files","exist","here","replace","absolute","paths","backslashes","check","build","root"]
hash: "f84a8fbcf639bf005c82bdcfcb2aa479c021763d1e0cc8b1f38dcdecf25b5717"
mode: "agent-primary"
facts: [{"kind":"code","literal":"{\n  \"version\": 1,\n  \"task\": \"Update the docs for a CLI change\",\n  \"sources\": [\n    {\"path\": \"AGENTS.md\", \"target\": \"AGENTS.md\"},\n    {\"path\": \"docs/AGENTS.md\", \"target\": \"docs/AGENTS.md\"},\n    {\"path\": \".claude/skills/docs-style/SKILL.md\", \"target\": \".claude/skills/docs-style/SKILL.md\"}\n  ]\n}","chunkId":"context#recipes"},{"kind":"code","literal":"path","chunkId":"context#recipes"},{"kind":"code","literal":"target","chunkId":"context#recipes"},{"kind":"code","literal":"sources","chunkId":"context#recipes"},{"kind":"code","literal":"task","chunkId":"context#recipes"},{"kind":"code","literal":"keep","chunkId":"context#recipes"},{"kind":"code","literal":"drop","chunkId":"context#recipes"},{"kind":"code","literal":"..","chunkId":"context#recipes"}]
sources: [{"chunkId":"context#recipes","url":"/docs/context#recipes","anchor":"recipes"}]
---

A recipe is plain JSON with no templating: a version, a task and a list of sources, each with a path and a target. The page includes a prompt to write one.
