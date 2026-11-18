---
id: "concepts#traces"
title: "Concepts"
heading: "Traces"
group: "Use"
order: 116
url: "/docs/concepts#traces"
anchor: "traces"
terms: ["traces","every","trace","daemon","indexes","harness","session","sessions","dispatched","tagged","order","name","listed","searched","validator","included","starter","search","matches","words","shared","archive","also","meaning","work","grouped","under","started","shown","waterfall","dashboard","list","improve","help","test","fail","find","tell","came","back"]
hash: "05b4e28d9c31080e70b92a3344577a17f7067f2307fb569fb5f55f8a8d7cea38"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev traces list --order improve-help\nhev traces search \"why did the test fail\" --order improve-help","chunkId":"concepts#traces"},{"kind":"code","literal":"Find the traces of the dispatched run improve-help: run `hev traces list --order improve-help`, then `hev traces search \"why did the test fail\" --order improve-help`. Tell me which sessions came back and quote the best matching passage. If nothing comes back, run `hev index` and try again.\nDocs: https://hevkit.com/docs/concepts#traces (all pages: https://hevkit.com/llms.txt)","chunkId":"concepts#traces"},{"kind":"code","literal":"~/dispatch/\u003cname\u003e/","chunkId":"concepts#traces"},{"kind":"code","literal":"order=\u003cname\u003e","chunkId":"concepts#traces"}]
sources: [{"chunkId":"concepts#traces","url":"/docs/concepts#traces","anchor":"traces"}]
---

Every run is a trace: the daemon indexes each harness session, and sessions from a dispatched run are tagged with the order name so they can be listed and searched by order, validator sessions included. Starter Kit search matches words and a shared archive also matches meaning. Dispatched work is also grouped under the ask that started it, shown as one waterfall in the dashboard.
