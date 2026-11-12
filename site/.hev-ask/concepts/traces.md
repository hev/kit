---
id: "concepts#traces"
title: "Concepts"
heading: "Traces"
group: "Use"
order: 60
url: "/docs/concepts#traces"
anchor: "traces"
terms: ["traces","every","trace","sessions","under","directory","tagged","order","name","listed","searched","page","gives","prompt","finding","list","improve","help","search","test","fail","find","dispatched","tell","came","back","quote","best","matching","passage","nothing","comes","index","again","docs","https","hevkit","concepts","pages","llms"]
hash: "a9affb84edd38a5b5ce165cd5378974e50b11cef399c7c3efb9d12e8ce10abd7"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev traces list --order improve-help\nhev traces search \"why did the test fail\" --order improve-help","chunkId":"concepts#traces"},{"kind":"code","literal":"Find the traces of the dispatched run improve-help: run `hev traces list --order improve-help`, then `hev traces search \"why did the test fail\" --order improve-help`. Tell me which sessions came back and quote the best matching passage. If nothing comes back, run `hev index` and try again.\nDocs: https://hevkit.com/docs/concepts#traces (all pages: https://hevkit.com/llms.txt)","chunkId":"concepts#traces"},{"kind":"code","literal":"~/dispatch/\u003cname\u003e/","chunkId":"concepts#traces"},{"kind":"code","literal":"order=\u003cname\u003e","chunkId":"concepts#traces"}]
sources: [{"chunkId":"concepts#traces","url":"/docs/concepts#traces","anchor":"traces"}]
---

Every run is a trace. Sessions under a run's directory are tagged with the order name, so traces can be listed and searched by order, and the page gives a prompt for finding them.
