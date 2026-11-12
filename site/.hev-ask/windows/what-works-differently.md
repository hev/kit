---
id: "windows#what-works-differently"
title: "Windows"
heading: "What works differently"
group: "Reference"
order: 307
url: "/docs/windows#what-works-differently"
anchor: "what-works-differently"
terms: ["works","differently","windows","attaching","just","prints","path","suggests","following","logs","host","cannot","browser","validation","crash","during","admission","leave","lock","directory","makes","admissions","fail","after","seconds","until","verify","nothing","running","remove","plugins","dispatch","attach","name","lockdir","busy","inspect","there","session","crashes"]
hash: "f1c0586db83c3403bff14a3b5991edb6d77196402d6402e9e90c480e995a4c70"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev dispatch attach NAME","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev dispatch logs NAME -f","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev","chunkId":"windows#what-works-differently"},{"kind":"code","literal":".lockdir","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"admission lock busy; inspect PATH","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev-NAME.exe","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev-NAME.cmd","chunkId":"windows#what-works-differently"}]
sources: [{"chunkId":"windows#what-works-differently","url":"/docs/windows#what-works-differently","anchor":"what-works-differently"}]
---

On Windows, attaching to a run just prints the log path and suggests following logs, a Windows host cannot be the browser validation host, and a crash during admission can leave a lock directory that makes new admissions fail after 30 seconds until you verify nothing is running and remove it. Plugins may be .exe or .cmd.
