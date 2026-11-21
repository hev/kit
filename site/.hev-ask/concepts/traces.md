---
id: "concepts#traces"
title: "Concepts"
heading: "Traces"
group: "Use"
order: 153
url: "/docs/concepts#traces"
anchor: "traces"
terms: ["traces","every","trace","sessions","whose","working","directory","under","folder","carry","order","name","listed","searched","list","improve","help","search","test","fail","find","dispatched","tell","came","back","quote","best","matching","passage","nothing","comes","index","again","docs","https","hevkit","concepts","pages","llms","orders"]
hash: "71c9503c4b9de272b96c597453d6903991be9c3598d0a7dc7f218eedbddf808e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev traces list --order improve-help\nhev traces search \"why did the test fail\" --order improve-help","chunkId":"concepts#traces"},{"kind":"code","literal":"Find the traces of the dispatched run improve-help: run `hev traces list --order improve-help`, then `hev traces search \"why did the test fail\" --order improve-help`. Tell me which sessions came back and quote the best matching passage. If nothing comes back, run `hev index` and try again.\nDocs: https://hevkit.com/docs/concepts#traces (all pages: https://hevkit.com/llms.txt)","chunkId":"concepts#traces"},{"kind":"code","literal":"~/.hev/orders/\u003cname\u003e/","chunkId":"concepts#traces"},{"kind":"code","literal":"order=\u003cname\u003e","chunkId":"concepts#traces"},{"kind":"code","literal":"~/.hev/traces/hev-traces/","chunkId":"concepts#traces"},{"kind":"code","literal":"hev serve","chunkId":"concepts#traces"},{"kind":"code","literal":"meta.json","chunkId":"concepts#traces"}]
sources: [{"chunkId":"concepts#traces","url":"/docs/concepts#traces","anchor":"traces"}]
---

Every run is a trace: sessions whose working directory is under the run's folder carry the order name and can be listed and searched by order.
