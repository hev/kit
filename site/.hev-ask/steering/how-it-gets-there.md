---
id: "steering#how-it-gets-there"
title: "Steering"
heading: "How it gets there"
group: "Use"
order: 403
url: "/docs/steering#how-it-gets-there"
anchor: "how-it-gets-there"
terms: ["gets","there","steering","travels","through","harness","hooks","hook","command","claude","code","codex","session","start","prompt","submit","around","tool","calls","stop","compaction","dispatch","runs","orchestrators","install","merges","settings","json","check","uninstall","removes","only","entries","event","steer","peers","send","warnings","enabled","false"]
hash: "6c53168906c2d36a0911cfd5ef929450c8f35275c457f5b7e5524256b4120dd8"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev hooks install      # merges into ~/.claude/settings.json and ~/.codex/hooks.json\nhev hooks check\nhev hooks uninstall    # removes only kit's entries","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"hev hook EVENT --harness H","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"/hooks","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"hev steer","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"peers send","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"~/.hev/hooks/warnings.log","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"HEV_HOOKS=off","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"[hooks] enabled = false","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"hev-hook-NAME","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"needs-you","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"steer-sent","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"steer-acked","chunkId":"steering#how-it-gets-there"},{"kind":"code","literal":"steer-escalated","chunkId":"steering#how-it-gets-there"},{"kind":"value","literal":"0.7","chunkId":"steering#how-it-gets-there"}]
sources: [{"chunkId":"steering#how-it-gets-there","url":"/docs/steering#how-it-gets-there","anchor":"how-it-gets-there"}]
---

Steering travels through kit's harness hooks (one hook command that Claude Code and Codex run at session start, prompt submit, around tool calls, stop and compaction); dispatch runs and orchestrators install them.
