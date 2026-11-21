---
id: "exec-commands#signals-and-exit-status"
title: "Exec commands"
heading: "Signals and exit status"
group: "Operate"
order: 214
url: "/docs/exec-commands#signals-and-exit-status"
anchor: "signals-and-exit-status"
terms: ["signals","exit","status","macos","linux","replaces","itself","exec","command","windows","launches","waits","commands","name","ctrl","other","terminal","straight","starts","console","interrupts","reach"]
hash: "5e5f24662357cdd74533a2f614173a3df80fd9fb7478a277cf96d7338f446acf"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev","chunkId":"exec-commands#signals-and-exit-status"},{"kind":"code","literal":"hev-NAME.exe","chunkId":"exec-commands#signals-and-exit-status"},{"kind":"code","literal":"hev-NAME.cmd","chunkId":"exec-commands#signals-and-exit-status"}]
sources: [{"chunkId":"exec-commands#signals-and-exit-status","url":"/docs/exec-commands#signals-and-exit-status","anchor":"signals-and-exit-status"}]
---

On macOS and Linux hev replaces itself with the exec command so signals and exit status are the command's; on Windows hev launches and waits. Exec commands can be .exe or .cmd.
