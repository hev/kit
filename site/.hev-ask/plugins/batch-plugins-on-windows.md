---
id: "plugins#batch-plugins-on-windows"
title: "Plugins"
heading: "Batch plugins on Windows"
group: "Operate"
order: 273
url: "/docs/plugins#batch-plugins-on-windows"
anchor: "batch-plugins-on-windows"
terms: ["batch","plugins","windows","file","through","reparses","arguments","refuses","path","contain","shell","metacharacters","spaces","fine","plugin","needed","arbitrary","runs","command","line","keep","safe","argument","contains","newline","paths","write","need","pass"]
hash: "99a88e40edb1ac14d91967db0146ecec043d34b41b7e93c2cf20455b6c7ecc81"
mode: "agent-primary"
facts: [{"kind":"code","literal":".cmd","chunkId":"plugins#batch-plugins-on-windows"},{"kind":"code","literal":"cmd.exe /d /s /c","chunkId":"plugins#batch-plugins-on-windows"},{"kind":"code","literal":".exe","chunkId":"plugins#batch-plugins-on-windows"}]
sources: [{"chunkId":"plugins#batch-plugins-on-windows","url":"/docs/plugins#batch-plugins-on-windows","anchor":"batch-plugins-on-windows"}]
---

Windows batch-file plugins run through cmd.exe, which reparses arguments, so hev refuses them when the path or arguments contain shell metacharacters; spaces are fine, and an exe plugin is needed for arbitrary arguments.
