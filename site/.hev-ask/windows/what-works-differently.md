---
id: "windows#what-works-differently"
title: "Windows"
heading: "What works differently"
group: "Reference"
order: 466
url: "/docs/windows#what-works-differently"
anchor: "what-works-differently"
terms: ["works","differently","windows","differences","attach","just","prints","path","session","host","cannot","browser","validation","stale","admission","lock","directory","block","admissions","seconds","fail","exec","commands","dispatch","name","logs","lockdir","busy","inspect","suggests","there","crashes","during","left","behind","wait","check","still","running","before"]
hash: "45459a3b9410894030dfcc3e0592c49d8eeb2c4da40c57db22ace9cf73ff99bd"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev dispatch attach NAME","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev dispatch logs NAME -f","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev","chunkId":"windows#what-works-differently"},{"kind":"code","literal":".lockdir","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"admission lock busy; inspect PATH","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev-NAME.exe","chunkId":"windows#what-works-differently"},{"kind":"code","literal":"hev-NAME.cmd","chunkId":"windows#what-works-differently"}]
sources: [{"chunkId":"windows#what-works-differently","url":"/docs/windows#what-works-differently","anchor":"what-works-differently"}]
---

Windows differences: attach just prints the log path (no session), a Windows host cannot be the browser validation host, a stale admission lock directory can block admissions for 30 seconds then fail, and exec commands may be .exe or .cmd.
