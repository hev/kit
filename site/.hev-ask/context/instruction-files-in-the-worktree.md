---
id: "context#instruction-files-in-the-worktree"
title: "Composable context"
heading: "Instruction files in the worktree"
group: "Use"
order: 81
url: "/docs/context#instruction-files-in-the-worktree"
anchor: "instruction-files-in-the-worktree"
terms: ["instruction","files","worktree","because","claude","code","codex","read","themselves","dispatch","places","file","form","recipe","gives","exactly","long","harness","runs","same","both","harnesses","update","index","skip","path","checkout","agents","assess","keep","drop","status","diff","commit","stash","edits","result","json","warnings","skills"]
hash: "f6e1824b997b024662f5950ad4240f984d67b7a72da6c31fa97725b973309344"
mode: "agent-primary"
facts: [{"kind":"code","literal":"git update-index --no-skip-worktree -- PATH \u0026\u0026 git checkout -- PATH","chunkId":"context#instruction-files-in-the-worktree"},{"kind":"code","literal":"AGENTS.md","chunkId":"context#instruction-files-in-the-worktree"},{"kind":"code","literal":"CLAUDE.md","chunkId":"context#instruction-files-in-the-worktree"},{"kind":"code","literal":"claude","chunkId":"context#instruction-files-in-the-worktree"},{"kind":"code","literal":"codex","chunkId":"context#instruction-files-in-the-worktree"},{"kind":"code","literal":"assess","chunkId":"context#instruction-files-in-the-worktree"},{"kind":"code","literal":"keep","chunkId":"context#instruction-files-in-the-worktree"},{"kind":"code","literal":"drop","chunkId":"context#instruction-files-in-the-worktree"},{"kind":"code","literal":"skip-worktree","chunkId":"context#instruction-files-in-the-worktree"},{"kind":"code","literal":"git status","chunkId":"context#instruction-files-in-the-worktree"},{"kind":"code","literal":"diff","chunkId":"context#instruction-files-in-the-worktree"},{"kind":"code","literal":"add -A","chunkId":"context#instruction-files-in-the-worktree"},{"kind":"code","literal":"commit -a","chunkId":"context#instruction-files-in-the-worktree"},{"kind":"code","literal":"stash","chunkId":"context#instruction-files-in-the-worktree"},{"kind":"code","literal":"git add PATH","chunkId":"context#instruction-files-in-the-worktree"},{"kind":"code","literal":"instruction-edits/","chunkId":"context#instruction-files-in-the-worktree"},{"kind":"code","literal":"result.json","chunkId":"context#instruction-files-in-the-worktree"},{"kind":"code","literal":"instruction_warnings","chunkId":"context#instruction-files-in-the-worktree"}]
sources: [{"chunkId":"context#instruction-files-in-the-worktree","url":"/docs/context#instruction-files-in-the-worktree","anchor":"instruction-files-in-the-worktree"}]
---

Because Claude Code and Codex read instruction files from the worktree themselves, dispatch places each file in the form the recipe gives, for exactly as long as the harness runs, the same for both harnesses.
