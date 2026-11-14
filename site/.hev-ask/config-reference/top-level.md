---
id: "config-reference#top-level"
title: "Config reference"
heading: "Top level"
group: "Reference"
order: 121
url: "/docs/config-reference#top-level"
anchor: "top-level"
terms: ["level","dispatch","config","fields","target","capacity","defaulting","raisable","senders","workspace","relative","repos","preferred","default","host","inline","runs","notification","command","without","shell","name","state","summary","reply","placeholders","failures","recorded","change","buildbox","notify","send","repo","hosts","error","result","json","field","type","read"]
hash: "edd41d67754755546e087859414c050aef161b7634c9b09f26ca01019e38f1f0"
mode: "source-primary"
facts: [{"kind":"code","literal":"cap = 2\nworkspace = \"~/workspace\"\ndefault_host = \"buildbox\"\nnotify = [\"notify-send\", \"dispatch {name}: {state} {summary}\"]","chunkId":"config-reference#top-level"},{"kind":"code","literal":"cap","chunkId":"config-reference#top-level"},{"kind":"code","literal":"workspace","chunkId":"config-reference#top-level"},{"kind":"code","literal":"\"~/workspace\"","chunkId":"config-reference#top-level"},{"kind":"code","literal":"repo","chunkId":"config-reference#top-level"},{"kind":"code","literal":"[hosts.NAME].workspace","chunkId":"config-reference#top-level"},{"kind":"code","literal":"default_host","chunkId":"config-reference#top-level"},{"kind":"code","literal":"hev dispatch run","chunkId":"config-reference#top-level"},{"kind":"code","literal":"notify","chunkId":"config-reference#top-level"},{"kind":"code","literal":"{name}","chunkId":"config-reference#top-level"},{"kind":"code","literal":"{state}","chunkId":"config-reference#top-level"},{"kind":"code","literal":"{summary}","chunkId":"config-reference#top-level"},{"kind":"code","literal":"{reply}","chunkId":"config-reference#top-level"},{"kind":"code","literal":"reply","chunkId":"config-reference#top-level"},{"kind":"code","literal":"notify_error","chunkId":"config-reference#top-level"},{"kind":"code","literal":"result.json","chunkId":"config-reference#top-level"}]
sources: [{"chunkId":"config-reference#top-level","url":"/docs/config-reference#top-level","anchor":"top-level"}]
---

Top-level dispatch config fields: target capacity (defaulting to 2, not raisable by senders), the workspace for relative repos, a preferred default host for inline runs, and a notification command run without a shell with name, state, summary and reply placeholders. Notification failures are recorded but do not change the run's state.
