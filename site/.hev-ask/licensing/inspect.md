---
id: "licensing#inspect"
title: "Licensing"
heading: "Inspect"
group: "Operate"
order: 269
url: "/docs/licensing#inspect"
anchor: "inspect"
terms: ["inspect","license","command","refreshes","shows","host","status","line","state","daemon","background","dispatch","decides","eligibility","local","refresh","show","json","capture","receiving","whether","offer","features","without","reading","vault","path"]
hash: "1454eda903bdc2d366e50796102a90721b3cbcf62415a972a7cfe63335f31cb2"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev license          # refresh and show this host's Kit license\nhev license --json\nhev s                # one-line license state in the host status","chunkId":"licensing#inspect"}]
sources: [{"chunkId":"licensing#inspect","url":"/docs/licensing#inspect","anchor":"inspect"}]
---

The license command refreshes and shows the host's license; the status command shows a one-line state. The daemon refreshes state in the background so dispatch decides Pro eligibility from local state.
