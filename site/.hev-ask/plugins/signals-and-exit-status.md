---
id: "plugins#signals-and-exit-status"
title: "Plugins"
heading: "Signals and exit status"
group: "Operate"
order: 231
url: "/docs/plugins#signals-and-exit-status"
anchor: "signals-and-exit-status"
terms: ["signals","exit","status","macos","linux","replaces","itself","plugin","directly","windows","starts","waits","console","interrupts","reach","plugins","files","name","ctrl","other","terminal","straight","command"]
hash: "65ff0c40f823c53acf756c9159c611fbec97feea5d5262c2a019abba468bc05a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev","chunkId":"plugins#signals-and-exit-status"},{"kind":"code","literal":"hev-NAME.exe","chunkId":"plugins#signals-and-exit-status"},{"kind":"code","literal":"hev-NAME.cmd","chunkId":"plugins#signals-and-exit-status"}]
sources: [{"chunkId":"plugins#signals-and-exit-status","url":"/docs/plugins#signals-and-exit-status","anchor":"signals-and-exit-status"}]
---

On macOS and Linux hev replaces itself with the plugin, so signals and exit status go directly to it. On Windows hev starts the plugin and waits, and console interrupts reach it. Plugins can be .exe or .cmd files.
