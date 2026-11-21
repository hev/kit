---
id: "hooks#the-event-log"
title: "Hooks"
heading: "The event log"
group: "Use"
order: 245
url: "/docs/hooks#the-event-log"
anchor: "the-event-log"
terms: ["event","records","redacted","line","harness","peer","message","board","post","evidence","sizes","only","prompts","tool","data","decisions","latency","rotating","past","steering","blocks","read","hooks","events","jsonl","attention","notices","join","normalized","without","prompt","text","input","command","output","their","plus","decision","injected","bytes"]
hash: "76c514060db56213b226de0bf5a90741dcb85f51c6eb06e67742440a82fddeef"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/.hev/hooks/events.jsonl","chunkId":"hooks#the-event-log"},{"kind":"code","literal":"peer-message","chunkId":"hooks#the-event-log"},{"kind":"code","literal":"board-post","chunkId":"hooks#the-event-log"},{"kind":"code","literal":"evidence-*","chunkId":"hooks#the-event-log"},{"kind":"code","literal":"events.jsonl.1","chunkId":"hooks#the-event-log"},{"kind":"value","literal":"0.7","chunkId":"hooks#the-event-log"}]
sources: [{"chunkId":"hooks#the-event-log","url":"/docs/hooks#the-event-log","anchor":"the-event-log"}]
---

The event log records one redacted line per harness, peer-message, board-post and evidence event (sizes only for prompts and tool data) with decisions and latency, rotating past 32 MiB; steering latency and blocks can be read from it.
