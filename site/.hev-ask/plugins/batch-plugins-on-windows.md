---
id: "plugins#batch-plugins-on-windows"
title: "Plugins"
heading: "Batch plugins on Windows"
group: "Operate"
order: 283
url: "/docs/plugins#batch-plugins-on-windows"
anchor: "batch-plugins-on-windows"
terms: ["batch","plugins","windows","file","plugin","runs","through","command","interpreter","reparses","line","refuses","path","argument","contains","shell","metacharacters","newline","paths","spaces","fine","write","executable","need","arbitrary","arguments","keep","safe","pass"]
hash: "99a88e40edb1ac14d91967db0146ecec043d34b41b7e93c2cf20455b6c7ecc81"
mode: "agent-primary"
facts: [{"kind":"code","literal":".cmd","chunkId":"plugins#batch-plugins-on-windows"},{"kind":"code","literal":"cmd.exe /d /s /c","chunkId":"plugins#batch-plugins-on-windows"},{"kind":"code","literal":".exe","chunkId":"plugins#batch-plugins-on-windows"}]
sources: [{"chunkId":"plugins#batch-plugins-on-windows","url":"/docs/plugins#batch-plugins-on-windows","anchor":"batch-plugins-on-windows"}]
---

On Windows a batch-file plugin runs through the command interpreter, which reparses its command line, so hev refuses it when the path or any argument contains shell metacharacters or a newline. Paths with spaces are fine; write an executable plugin if you need arbitrary arguments.
