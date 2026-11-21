---
id: "hooks#one-handler-model"
title: "Hooks"
heading: "One handler model"
group: "Use"
order: 241
url: "/docs/hooks#one-handler-model"
anchor: "one-handler-model"
terms: ["handler","model","every","event","takes","path","whether","harness","raised","normalize","hook","envelope","naming","source","claude","codex","payload","redacted","copy","events","file","built","handlers","name","concurrently","under","time","budget","fail","open","rules","argument","json","stdin","hooks","jsonl","attention","needs","ready","done"]
hash: "d1d5fed48ee4ca8f5dcb4b45948d8b9eef2b3becd9f5f812aecb19aeb3243e23"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev.hook.event/v1","chunkId":"hooks#one-handler-model"},{"kind":"code","literal":"claude","chunkId":"hooks#one-handler-model"},{"kind":"code","literal":"codex","chunkId":"hooks#one-handler-model"},{"kind":"code","literal":"kit","chunkId":"hooks#one-handler-model"},{"kind":"code","literal":"~/.hev/hooks/events.jsonl","chunkId":"hooks#one-handler-model"},{"kind":"code","literal":"hev-hook-NAME","chunkId":"hooks#one-handler-model"},{"kind":"code","literal":"HEV_HOOK_EVENT","chunkId":"hooks#one-handler-model"},{"kind":"code","literal":"HEV_HOOK_HARNESS","chunkId":"hooks#one-handler-model"},{"kind":"code","literal":"hev.attention.event/v1","chunkId":"hooks#one-handler-model"},{"kind":"code","literal":"needs-you","chunkId":"hooks#one-handler-model"},{"kind":"code","literal":"ready","chunkId":"hooks#one-handler-model"},{"kind":"code","literal":"done","chunkId":"hooks#one-handler-model"},{"kind":"code","literal":"peer-message","chunkId":"hooks#one-handler-model"},{"kind":"code","literal":"board-post","chunkId":"hooks#one-handler-model"},{"kind":"code","literal":"evidence-*","chunkId":"hooks#one-handler-model"},{"kind":"code","literal":"events.jsonl","chunkId":"hooks#one-handler-model"},{"kind":"value","literal":"0.7","chunkId":"hooks#one-handler-model"}]
sources: [{"chunkId":"hooks#one-handler-model","url":"/docs/hooks#one-handler-model","anchor":"one-handler-model"}]
---

Every event takes one path whether a harness or kit raised it: normalize into a hev.hook.event/v1 envelope naming event, source (claude, codex or kit) and payload; log a redacted copy to the events file; then run built-in handlers and every hev-hook-NAME concurrently under one time budget with fail-open rules. Handlers get the event name as argument and env var, source in an env var, and JSON on stdin.
