---
id: "traces#start-capturing"
title: "Traces"
heading: "Start capturing"
group: "Use"
order: 284
url: "/docs/traces#start-capturing"
anchor: "start-capturing"
terms: ["start","capturing","sets","capture","machine","writes","config","installs","hevd","under","launchd","systemd","traces","skill","harnesses","layer","stay","files","archive","directory","docker","verify","exists","export","hevlayer","backfill","init","stop","down","daemon","status","claude","code","codex","opencode","whichever","trace","make","sure","hevlayerapikey"]
hash: "37edeb36ad6bfe314c87043c0f1a12aa3744b64d7af907f159b3a3e993822cb2"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev up","chunkId":"traces#start-capturing"},{"kind":"code","literal":"export HEVLAYER_API_KEY=...\nhev up","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev-traces","chunkId":"traces#start-capturing"},{"kind":"code","literal":"~/.hev/archive/hev-traces/","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev backfill","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev init","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev d","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev s","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev stop","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev down","chunkId":"traces#start-capturing"},{"kind":"code","literal":"hev daemon status","chunkId":"traces#start-capturing"},{"kind":"value","literal":"hevlayer.com","chunkId":"traces#start-capturing"}]
sources: [{"chunkId":"traces#start-capturing","url":"/docs/traces#start-capturing","anchor":"start-capturing"}]
---

hev up sets up capture on one machine: writes config, installs hevd under launchd or systemd, and installs the traces skill for the harnesses you have. With no layer key traces stay as files in the archive directory with no Docker. Verify with hev s and that the archive directory exists.
