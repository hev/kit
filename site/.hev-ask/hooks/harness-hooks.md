---
id: "hooks#harness-hooks"
title: "Hooks"
heading: "Harness hooks"
group: "Use"
order: 237
url: "/docs/hooks#harness-hooks"
anchor: "harness-hooks"
terms: ["harness","hooks","both","claude","code","codex","call","command","normalizes","event","logs","runs","handlers","answers","shape","table","maps","events","session","start","prompt","submit","tool","post","stop","compact","context","block","halt","observe","hook","sessionstart","userpromptsubmit","pretooluse","posttooluse","precompact","turn","bash","apply","patch"]
hash: "18288e9619bdc2c050d59790e3449003d97cc3ceee978295f44198740e7a83c3"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev hook EVENT --harness claude|codex","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"hev.hook.event/v1","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"session-start","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"SessionStart","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"prompt-submit","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"UserPromptSubmit","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"pre-tool","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"PreToolUse","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"post-tool","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"PostToolUse","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"stop","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"Stop","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"pre-compact","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"PreCompact","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"turn_id","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"Bash","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"apply_patch","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"--harness pi","chunkId":"hooks#harness-hooks"},{"kind":"code","literal":"peers send","chunkId":"hooks#harness-hooks"}]
sources: [{"chunkId":"hooks#harness-hooks","url":"/docs/hooks#harness-hooks","anchor":"harness-hooks"}]
---

Harness hooks: both Claude Code and Codex call one command, which normalizes the event, logs it, runs handlers and answers in the harness's shape. A table maps events (session-start, prompt-submit, pre-tool, post-tool, stop, pre-compact) to what handlers can do (add context, block, halt, observe).
