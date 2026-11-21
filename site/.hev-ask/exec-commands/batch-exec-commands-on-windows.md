---
id: "exec-commands#batch-exec-commands-on-windows"
title: "Exec commands"
heading: "Batch exec commands on Windows"
group: "Operate"
order: 212
url: "/docs/exec-commands#batch-exec-commands-on-windows"
anchor: "batch-exec-commands-on-windows"
terms: ["batch","exec","commands","windows","through","refuses","paths","arguments","containing","metacharacters","quotes","caret","ampersand","percent","arbitrary","command","runs","reparses","line","keep","safe","path","argument","contains","newline","spaces","fine","write","need","pass"]
hash: "b37e08a1bf5ff36089cb6ad088a6edef5d647fbe534cc9572d8642a13ff66d95"
mode: "agent-primary"
facts: [{"kind":"code","literal":".cmd","chunkId":"exec-commands#batch-exec-commands-on-windows"},{"kind":"code","literal":"cmd.exe /d /s /c","chunkId":"exec-commands#batch-exec-commands-on-windows"},{"kind":"code","literal":".exe","chunkId":"exec-commands#batch-exec-commands-on-windows"}]
sources: [{"chunkId":"exec-commands#batch-exec-commands-on-windows","url":"/docs/exec-commands#batch-exec-commands-on-windows","anchor":"batch-exec-commands-on-windows"}]
---

On Windows, .cmd exec commands run through cmd.exe and hev refuses paths or arguments containing cmd metacharacters (quotes, caret, ampersand, percent, etc.). Use an .exe for arbitrary arguments.
