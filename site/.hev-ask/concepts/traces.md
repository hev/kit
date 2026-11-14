---
id: "concepts#traces"
title: "Concepts"
heading: "Traces"
group: "Use"
order: 115
url: "/docs/concepts#traces"
anchor: "traces"
terms: ["traces","daemon","indexes","harness","session","dispatched","sessions","tagged","their","order","name","list","search","starter","searches","words","shared","archive","also","matches","meaning","starts","work","root","trace","orders","attempts","validation","beneath","shown","waterfall","improve","help","test","fail","find","tell","came","back","quote"]
hash: "ad0552cba2be2ffdaf72854c4ecb652ee9bfb61d2b322ff9662471b72bb6e2e9"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev traces list --order improve-help\nhev traces search \"why did the test fail\" --order improve-help","chunkId":"concepts#traces"},{"kind":"code","literal":"Find the traces of the dispatched run improve-help: run `hev traces list --order improve-help`, then `hev traces search \"why did the test fail\" --order improve-help`. Tell me which sessions came back and quote the best matching passage. If nothing comes back, run `hev index` and try again.\nDocs: https://hevkit.com/docs/concepts#traces (all pages: https://hevkit.com/llms.txt)","chunkId":"concepts#traces"},{"kind":"code","literal":"~/dispatch/\u003cname\u003e/","chunkId":"concepts#traces"},{"kind":"code","literal":"order=\u003cname\u003e","chunkId":"concepts#traces"}]
sources: [{"chunkId":"concepts#traces","url":"/docs/concepts#traces","anchor":"traces"}]
---

The daemon indexes each harness session, and dispatched sessions are tagged with their order name so you can list and search a run's traces. Starter Kit searches by words; a shared archive also matches meaning. The ask that starts work is the root of a trace, with orders, attempts, validation and sessions beneath it shown as a waterfall.
