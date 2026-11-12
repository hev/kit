---
id: "config-reference#top-level"
title: "Config reference"
heading: "Top level"
group: "Reference"
order: 66
url: "/docs/config-reference#top-level"
anchor: "top-level"
terms: ["level","fields","target","capacity","senders","cannot","raise","workspace","relative","repo","paths","preferred","default","host","inline","runs","notify","command","argument","list","name","state","summary","reply","placeholders","failing","recorded","result","does","change","buildbox","send","dispatch","hosts","error","json","field","type","read","meaning"]
hash: "edd41d67754755546e087859414c050aef161b7634c9b09f26ca01019e38f1f0"
mode: "source-primary"
facts: [{"kind":"code","literal":"cap = 2\nworkspace = \"~/workspace\"\ndefault_host = \"buildbox\"\nnotify = [\"notify-send\", \"dispatch {name}: {state} {summary}\"]","chunkId":"config-reference#top-level"},{"kind":"code","literal":"cap","chunkId":"config-reference#top-level"},{"kind":"code","literal":"workspace","chunkId":"config-reference#top-level"},{"kind":"code","literal":"\"~/workspace\"","chunkId":"config-reference#top-level"},{"kind":"code","literal":"repo","chunkId":"config-reference#top-level"},{"kind":"code","literal":"[hosts.NAME].workspace","chunkId":"config-reference#top-level"},{"kind":"code","literal":"default_host","chunkId":"config-reference#top-level"},{"kind":"code","literal":"hev dispatch run","chunkId":"config-reference#top-level"},{"kind":"code","literal":"notify","chunkId":"config-reference#top-level"},{"kind":"code","literal":"{name}","chunkId":"config-reference#top-level"},{"kind":"code","literal":"{state}","chunkId":"config-reference#top-level"},{"kind":"code","literal":"{summary}","chunkId":"config-reference#top-level"},{"kind":"code","literal":"{reply}","chunkId":"config-reference#top-level"},{"kind":"code","literal":"reply","chunkId":"config-reference#top-level"},{"kind":"code","literal":"notify_error","chunkId":"config-reference#top-level"},{"kind":"code","literal":"result.json","chunkId":"config-reference#top-level"}]
sources: [{"chunkId":"config-reference#top-level","url":"/docs/config-reference#top-level","anchor":"top-level"}]
---

Top-level fields set target capacity (which senders cannot raise), the workspace for relative repo paths, a preferred default host for inline runs, and a notify command as an argument list with name, state, summary and reply placeholders. A failing notify is recorded in the result and does not change the run state.
