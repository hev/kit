---
id: "windows#what-works-differently"
title: "Windows"
heading: "What works differently"
group: "Reference"
order: 416
url: "/docs/windows#what-works-differently"
anchor: "what-works-differently"
terms: ["works","differently","differences","windows","hosts","there","session","attach","attaching","just","points","follow","host","cannot","browser","validation","crash","during","admission","leave","lock","directory","behind","making","admissions","wait","seconds","fail","busy","until","confirm","running","remove","plugins","executables","command","scripts","dispatch","name","logs"]
hash: "f1c0586db83c3403bff14a3b5991edb6d77196402d6402e9e90c480e995a4c70"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev dispatch attach NAME","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev dispatch logs NAME -f","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev","chunkId":"windows#what-works-differently"},{"kind":"code","literal":".lockdir","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"admission lock busy; inspect PATH","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev-NAME.exe","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev-NAME.cmd","chunkId":"windows#what-works-differently"}]
sources: [{"chunkId":"windows#what-works-differently","url":"/docs/windows#what-works-differently","anchor":"what-works-differently"}]
---

Differences on Windows hosts: there is no session to attach to, so attaching just points you at the run's log to follow; a Windows host cannot be the browser validation host; and a crash during admission can leave the lock directory behind, making new admissions wait 30 seconds and then fail as busy until you confirm no admission is running and remove it. Plugins on Windows may be executables or command scripts.
