---
id: "plugins#batch-plugins-on-windows"
title: "Plugins"
heading: "Batch plugins on Windows"
group: "Operate"
order: 223
url: "/docs/plugins#batch-plugins-on-windows"
anchor: "batch-plugins-on-windows"
terms: ["batch","plugins","windows","plugin","runs","through","reparses","command","line","refuses","path","argument","contains","shell","metacharacters","such","quotes","ampersands","carets","redirection","parentheses","percent","exclamation","marks","newlines","spaces","paths","fine","write","pass","arbitrary","arguments","keep","safe","newline","need"]
hash: "99a88e40edb1ac14d91967db0146ecec043d34b41b7e93c2cf20455b6c7ecc81"
mode: "agent-primary"
facts: [{"kind":"code","literal":".cmd","chunkId":"plugins#batch-plugins-on-windows"},{"kind":"code","literal":"cmd.exe /d /s /c","chunkId":"plugins#batch-plugins-on-windows"},{"kind":"code","literal":".exe","chunkId":"plugins#batch-plugins-on-windows"}]
sources: [{"chunkId":"plugins#batch-plugins-on-windows","url":"/docs/plugins#batch-plugins-on-windows","anchor":"batch-plugins-on-windows"}]
---

A batch (.cmd) plugin on Windows runs through cmd.exe, which reparses its command line, so hev refuses it when the path or any argument contains shell metacharacters such as quotes, ampersands, carets, redirection, parentheses, percent or exclamation marks, or newlines. Spaces in paths are fine; write an .exe plugin to pass arbitrary arguments.
