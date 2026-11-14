---
id: "windows#what-works-differently"
title: "Windows"
heading: "What works differently"
group: "Reference"
order: 369
url: "/docs/windows#what-works-differently"
anchor: "what-works-differently"
terms: ["works","differently","windows","differences","attach","just","points","since","there","session","host","browser","validation","crash","during","admission","leave","stale","lock","makes","admissions","fail","after","seconds","check","nothing","running","before","removing","plugins","files","dispatch","name","logs","lockdir","busy","inspect","path","prints","suggests"]
hash: "f1c0586db83c3403bff14a3b5991edb6d77196402d6402e9e90c480e995a4c70"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev dispatch attach NAME","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev dispatch logs NAME -f","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev","chunkId":"windows#what-works-differently"},{"kind":"code","literal":".lockdir","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"admission lock busy; inspect PATH","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev-NAME.exe","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev-NAME.cmd","chunkId":"windows#what-works-differently"}]
sources: [{"chunkId":"windows#what-works-differently","url":"/docs/windows#what-works-differently","anchor":"what-works-differently"}]
---

Windows differences: attach just points you to the log since there is no session, a Windows host can't be the browser validation host, a crash during admission can leave a stale lock that makes new admissions fail after 30 seconds (check nothing is running before removing it), and plugins can be exe or cmd files.
