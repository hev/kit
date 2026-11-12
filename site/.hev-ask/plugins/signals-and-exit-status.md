---
id: "plugins#signals-and-exit-status"
title: "Plugins"
heading: "Signals and exit status"
group: "Operate"
order: 225
url: "/docs/plugins#signals-and-exit-status"
anchor: "signals-and-exit-status"
terms: ["signals","exit","status","macos","linux","replaces","itself","plugin","terminal","like","ctrl","reach","directly","becomes","command","windows","launches","waits","console","interrupts","still","plugins","files","name","other","straight","starts"]
hash: "65ff0c40f823c53acf756c9159c611fbec97feea5d5262c2a019abba468bc05a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev","chunkId":"plugins#signals-and-exit-status"},{"kind":"code","literal":"hev-NAME.exe","chunkId":"plugins#signals-and-exit-status"},{"kind":"code","literal":"hev-NAME.cmd","chunkId":"plugins#signals-and-exit-status"}]
sources: [{"chunkId":"plugins#signals-and-exit-status","url":"/docs/plugins#signals-and-exit-status","anchor":"signals-and-exit-status"}]
---

On macOS and Linux hev replaces itself with the plugin, so terminal signals like Ctrl-C reach it directly and its exit status becomes the command's. On Windows hev launches the plugin and waits, console interrupts still reach it, and plugins may be .exe or .cmd files.
