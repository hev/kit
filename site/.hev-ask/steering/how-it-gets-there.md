---
id: "steering#how-it-gets-there"
title: "Steering"
heading: "How it gets there"
group: "Use"
order: 342
url: "/docs/steering#how-it-gets-there"
anchor: "how-it-gets-there"
terms: ["gets","there","steering","rides","hooks","single","command","claude","code","codex","session","start","prompt","submit","around","tool","call","stop","before","compaction","dispatch","runs","orchestrator","rounds","automatically","install","yourself","interactive","sessions","needs","hook","trusted","once","says","falls","back","next","turn","only","receive"]
hash: "e2ccf1c546257a0057561df97396a6d9e4b8754ca9f3f0d1f54e8f56e363ec1c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev hooks install      # merges into ~/.claude/settings.json and ~/.codex/hooks.json\nhev hooks check\nhev hooks uninstall    # removes only kit's entries","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"hev hook EVENT --harness H","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"/hooks","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"hev steer","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"peers send","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"~/.hev/hooks/warnings.log","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"HEV_HOOKS=off","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"[hooks] enabled = false","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"hev-hook-NAME","chunkId":"steering#how-it-gets-there"}]
sources: [{"chunkId":"steering#how-it-gets-there","url":"/docs/steering#how-it-gets-there","anchor":"how-it-gets-there"}]
---

Steering rides on kit's hooks, a single command that Claude Code and Codex run at session start, prompt submit, around each tool call, at stop and before compaction; dispatch runs and orchestrator rounds get them automatically, and you install them yourself for interactive sessions (Codex needs the hook trusted once). Where no hook runs, steering says so and falls back to the next turn, pi sessions only receive plain peer notes, hooks fail open after a short timeout and can be turned off, and you can add your own hook handlers as executables on PATH.
