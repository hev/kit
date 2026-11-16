---
id: "config-reference#top-level"
title: "Config reference"
heading: "Top level"
group: "Reference"
order: 122
url: "/docs/config-reference#top-level"
anchor: "top-level"
terms: ["level","reference","dispatch","config","fields","target","capacity","default","sender","cannot","raise","workspace","relative","repo","values","resolve","against","host","inline","prefers","notify","command","without","shell","validation","finishes","placeholders","name","state","summary","reply","failing","recorded","result","does","change","buildbox","send","hosts","error"]
hash: "edd41d67754755546e087859414c050aef161b7634c9b09f26ca01019e38f1f0"
mode: "source-primary"
facts: [{"kind":"code","literal":"cap = 2\nworkspace = \"~/workspace\"\ndefault_host = \"buildbox\"\nnotify = [\"notify-send\", \"dispatch {name}: {state} {summary}\"]","chunkId":"config-reference#top-level"},{"kind":"code","literal":"cap","chunkId":"config-reference#top-level"},{"kind":"code","literal":"workspace","chunkId":"config-reference#top-level"},{"kind":"code","literal":"\"~/workspace\"","chunkId":"config-reference#top-level"},{"kind":"code","literal":"repo","chunkId":"config-reference#top-level"},{"kind":"code","literal":"[hosts.NAME].workspace","chunkId":"config-reference#top-level"},{"kind":"code","literal":"default_host","chunkId":"config-reference#top-level"},{"kind":"code","literal":"hev dispatch run","chunkId":"config-reference#top-level"},{"kind":"code","literal":"notify","chunkId":"config-reference#top-level"},{"kind":"code","literal":"{name}","chunkId":"config-reference#top-level"},{"kind":"code","literal":"{state}","chunkId":"config-reference#top-level"},{"kind":"code","literal":"{summary}","chunkId":"config-reference#top-level"},{"kind":"code","literal":"{reply}","chunkId":"config-reference#top-level"},{"kind":"code","literal":"reply","chunkId":"config-reference#top-level"},{"kind":"code","literal":"notify_error","chunkId":"config-reference#top-level"},{"kind":"code","literal":"result.json","chunkId":"config-reference#top-level"}]
sources: [{"chunkId":"config-reference#top-level","url":"/docs/config-reference#top-level","anchor":"top-level"}]
---

Reference for top-level dispatch config fields: the target's capacity (default 2, which a sender cannot raise), the workspace that relative repo values resolve against, the default host that inline dispatch prefers, and a notify command run without a shell when a run or validation finishes, with placeholders for name, state, summary and reply. A failing notify command is recorded in the result and does not change the run's state.
