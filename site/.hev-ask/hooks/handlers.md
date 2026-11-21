---
id: "hooks#handlers"
title: "Hooks"
heading: "Handlers"
group: "Use"
order: 236
url: "/docs/hooks#handlers"
anchor: "handlers"
terms: ["handlers","built","steer","delivers","steering","messages","feed","spools","slack","peers","inbox","injects","prompt","submit","custom","hook","name","executables","path","receiving","event","json","stdin","vars","printing","nothing","continue","decision","block","halt","reason","shown","model","context","injected","system","message","person","force","push"]
hash: "d374978275f785bea92b481941b919bf48a0df73451dd6a7a167350fdf1e5d6a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"{\"decision\": \"continue|block|halt\", \"reason\": \"shown to the model\", \"context\": \"injected\", \"system_message\": \"shown to the person\"}","chunkId":"hooks#handlers"},{"kind":"code","literal":"#!/bin/sh\n# hev-hook-no-force-push\ncase \"$(cat)\" in\n  *'\"kind\":\"mutate\"'*'push --force'*) echo '{\"decision\":\"block\",\"reason\":\"No force pushes here.\"}' ;;\nesac","chunkId":"hooks#handlers"},{"kind":"code","literal":"steer","chunkId":"hooks#handlers"},{"kind":"code","literal":"feed","chunkId":"hooks#handlers"},{"kind":"code","literal":"[feed]","chunkId":"hooks#handlers"},{"kind":"code","literal":"peers-inbox","chunkId":"hooks#handlers"},{"kind":"code","literal":"prompt-submit","chunkId":"hooks#handlers"},{"kind":"code","literal":"hev-hook-NAME","chunkId":"hooks#handlers"},{"kind":"code","literal":".exe","chunkId":"hooks#handlers"},{"kind":"code","literal":".cmd","chunkId":"hooks#handlers"},{"kind":"code","literal":"HEV_HOOK_EVENT","chunkId":"hooks#handlers"},{"kind":"code","literal":"HEV_HOOK_HARNESS","chunkId":"hooks#handlers"},{"kind":"code","literal":"/v1/hooks","chunkId":"hooks#handlers"},{"kind":"code","literal":"~/.hev/hooks/events.jsonl","chunkId":"hooks#handlers"}]
sources: [{"chunkId":"hooks#handlers","url":"/docs/hooks#handlers","anchor":"handlers"}]
---

Built-in handlers are steer (delivers steering messages), feed (spools for Slack feed) and peers-inbox (injects the peers inbox on prompt submit). Custom handlers are hev-hook-NAME executables on PATH receiving event name, JSON on stdin and env vars, printing nothing to continue.
