---
id: "windows#what-works-differently"
title: "Windows"
heading: "What works differently"
group: "Reference"
order: 321
url: "/docs/windows#what-works-differently"
anchor: "what-works-differently"
terms: ["works","differently","windows","attach","prints","path","instead","attaching","host","cannot","browser","validation","crashes","admission","lock","directory","remain","admissions","wait","seconds","fail","busy","error","check","nothing","still","admitting","before","removing","plugins","dispatch","name","logs","lockdir","inspect","suggests","there","session","during","left"]
hash: "f1c0586db83c3403bff14a3b5991edb6d77196402d6402e9e90c480e995a4c70"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev dispatch attach NAME","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev dispatch logs NAME -f","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev","chunkId":"windows#what-works-differently"},{"kind":"code","literal":".lockdir","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"admission lock busy; inspect PATH","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev-NAME.exe","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev-NAME.cmd","chunkId":"windows#what-works-differently"}]
sources: [{"chunkId":"windows#what-works-differently","url":"/docs/windows#what-works-differently","anchor":"what-works-differently"}]
---

On Windows, attach prints the log path instead of attaching and a Windows host cannot be the browser validation host. If hev crashes in admission the lock directory can remain; new admissions wait 30 seconds then fail with an admission lock busy error, so check nothing is still admitting before removing it. Plugins can be .exe or .cmd.
