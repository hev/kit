---
id: "concepts#traces"
title: "Concepts"
heading: "Traces"
group: "Use"
order: 60
url: "/docs/concepts#traces"
anchor: "traces"
terms: ["traces","sessions","running","inside","dispatch","directory","tagged","order","name","list","search","including","validator","matches","words","alone","also","meaning","layer","starts","work","roots","trace","shown","waterfall","improve","help","test","fail","find","dispatched","tell","came","back","quote","best","matching","passage","nothing","comes"]
hash: "502e37ae3a3c2d01f72f03fe265bd2ba9d7013bd27a3b894c550db4e4d2a839e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev traces list --order improve-help\nhev traces search \"why did the test fail\" --order improve-help","chunkId":"concepts#traces"},{"kind":"code","literal":"Find the traces of the dispatched run improve-help: run `hev traces list --order improve-help`, then `hev traces search \"why did the test fail\" --order improve-help`. Tell me which sessions came back and quote the best matching passage. If nothing comes back, run `hev index` and try again.\nDocs: https://hevkit.com/docs/concepts#traces (all pages: https://hevkit.com/llms.txt)","chunkId":"concepts#traces"},{"kind":"code","literal":"~/dispatch/\u003cname\u003e/","chunkId":"concepts#traces"},{"kind":"code","literal":"order=\u003cname\u003e","chunkId":"concepts#traces"}]
sources: [{"chunkId":"concepts#traces","url":"/docs/concepts#traces","anchor":"traces"}]
---

Sessions running inside a dispatch run directory are tagged with the order name so you can list and search a run's traces, including validator sessions. Search matches words alone or also meaning with kit + layer, and the ask that starts work roots a trace shown as one waterfall.
