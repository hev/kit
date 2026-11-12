---
id: "context#recipes"
title: "Composable context"
heading: "Recipes"
group: "Use"
order: 81
url: "/docs/context#recipes"
anchor: "recipes"
terms: ["recipes","recipe","plain","json","version","task","ordered","list","sources","project","path","bundle","target","source","order","part","digest","body","replaces","dispatch","unknown","fields","absolute","paths","parent","references","backslashes","symlinks","duplicate","targets","very","large","files","refused","update","docs","change","agents","claude","skills"]
hash: "9dedc9b689f5219d4d6799684c16c3820b069258c7f0f3c3a88cf98206dfc751"
mode: "agent-primary"
facts: [{"kind":"code","literal":"{\n  \"version\": 1,\n  \"task\": \"Update the docs for a CLI change\",\n  \"sources\": [\n    {\"path\": \"AGENTS.md\", \"target\": \"AGENTS.md\"},\n    {\"path\": \"docs/AGENTS.md\", \"target\": \"docs/AGENTS.md\"},\n    {\"path\": \".claude/skills/docs-style/SKILL.md\", \"target\": \".claude/skills/docs-style/SKILL.md\"}\n  ]\n}","chunkId":"context#recipes"},{"kind":"code","literal":"path","chunkId":"context#recipes"},{"kind":"code","literal":"target","chunkId":"context#recipes"},{"kind":"code","literal":"sources","chunkId":"context#recipes"},{"kind":"code","literal":"task","chunkId":"context#recipes"},{"kind":"code","literal":"..","chunkId":"context#recipes"}]
sources: [{"chunkId":"context#recipes","url":"/docs/context#recipes","anchor":"recipes"}]
---

A recipe is plain JSON with a version, a task and an ordered list of sources each with a project path and a bundle target. Source order is part of the digest, the order body replaces the task at dispatch, and unknown fields, absolute paths, parent references, backslashes, symlinks, duplicate targets and very large files are refused.
