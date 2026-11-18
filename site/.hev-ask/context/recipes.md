---
id: "context#recipes"
title: "Composable context"
heading: "Recipes"
group: "Use"
order: 141
url: "/docs/context#recipes"
anchor: "recipes"
terms: ["recipes","recipe","plain","json","version","task","ordered","list","sources","project","relative","path","target","location","inside","bundle","source","order","reading","part","digest","body","replaces","dispatch","skill","files","should","keep","their","original","targets","unknown","fields","absolute","parent","paths","backslashes","symlinks","duplicate","very"]
hash: "f84a8fbcf639bf005c82bdcfcb2aa479c021763d1e0cc8b1f38dcdecf25b5717"
mode: "agent-primary"
facts: [{"kind":"code","literal":"{\n  \"version\": 1,\n  \"task\": \"Update the docs for a CLI change\",\n  \"sources\": [\n    {\"path\": \"AGENTS.md\", \"target\": \"AGENTS.md\"},\n    {\"path\": \"docs/AGENTS.md\", \"target\": \"docs/AGENTS.md\"},\n    {\"path\": \".claude/skills/docs-style/SKILL.md\", \"target\": \".claude/skills/docs-style/SKILL.md\"}\n  ]\n}","chunkId":"context#recipes"},{"kind":"code","literal":"path","chunkId":"context#recipes"},{"kind":"code","literal":"target","chunkId":"context#recipes"},{"kind":"code","literal":"sources","chunkId":"context#recipes"},{"kind":"code","literal":"task","chunkId":"context#recipes"},{"kind":"code","literal":"keep","chunkId":"context#recipes"},{"kind":"code","literal":"drop","chunkId":"context#recipes"},{"kind":"code","literal":"..","chunkId":"context#recipes"}]
sources: [{"chunkId":"context#recipes","url":"/docs/context#recipes","anchor":"recipes"}]
---

A recipe is plain JSON with a version, a task and an ordered list of sources, each with a project-relative path and a target location inside the bundle. Source order is the reading order and part of the digest, the order's body replaces the task at dispatch, and a skill's files should keep their original relative targets. Unknown fields, absolute or parent paths, backslashes, symlinks, duplicate targets and very large files are refused.
