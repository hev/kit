---
id: "plugins#batch-plugins-on-windows"
title: "Plugins"
heading: "Batch plugins on Windows"
group: "Operate"
order: 229
url: "/docs/plugins#batch-plugins-on-windows"
anchor: "batch-plugins-on-windows"
terms: ["batch","plugins","windows","plugin","runs","through","reparses","command","line","refuses","path","argument","contains","certain","shell","special","characters","newline","spaces","paths","fine","pass","arbitrary","arguments","keep","safe","write","need"]
hash: "99a88e40edb1ac14d91967db0146ecec043d34b41b7e93c2cf20455b6c7ecc81"
mode: "agent-primary"
facts: [{"kind":"code","literal":".cmd","chunkId":"plugins#batch-plugins-on-windows"},{"kind":"code","literal":"cmd.exe /d /s /c","chunkId":"plugins#batch-plugins-on-windows"},{"kind":"code","literal":".exe","chunkId":"plugins#batch-plugins-on-windows"}]
sources: [{"chunkId":"plugins#batch-plugins-on-windows","url":"/docs/plugins#batch-plugins-on-windows","anchor":"batch-plugins-on-windows"}]
---

On Windows a .cmd plugin runs through cmd.exe, which reparses the command line, so hev refuses it if the path or any argument contains certain shell-special characters or a newline. Spaces in paths are fine. Use an .exe plugin to pass arbitrary arguments.
