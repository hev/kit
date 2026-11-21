---
id: "quickstart#7-search-its-trace"
title: "Quickstart"
heading: "7. Search its trace"
group: "Start"
order: 352
url: "/docs/quickstart#7-search-its-trace"
anchor: "7-search-its-trace"
terms: ["search","trace","quickstart","step","daemon","scans","every","five","minutes","index","list","traces","filtered","order","name","since","sessions","under","folder","carry","first","tests","find","tell","came","back","quote","best","matching","passage","docs","https","hevkit","pages","llms","orders","default","filter","whose","working"]
hash: "43cb72016bfbf90c8c496f3dbb67d09e6b85dd188f101f4b3254e7b64edf42f9"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev index\nhev traces list --order first-run\nhev traces search \"how to run the tests\" --order first-run","chunkId":"quickstart#7-search-its-trace"},{"kind":"code","literal":"Find the trace of the run first-run: run `hev index` to index now, then `hev traces list --order first-run` and `hev traces search \"how to run the tests\" --order first-run`. Tell me which sessions came back and quote the best matching passage.\nDocs: https://hevkit.com/docs/quickstart#7-search-its-trace (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#7-search-its-trace"},{"kind":"code","literal":"hev index","chunkId":"quickstart#7-search-its-trace"},{"kind":"code","literal":"~/.hev/orders/first-run/","chunkId":"quickstart#7-search-its-trace"},{"kind":"code","literal":"order=first-run","chunkId":"quickstart#7-search-its-trace"}]
sources: [{"chunkId":"quickstart#7-search-its-trace","url":"/docs/quickstart#7-search-its-trace","anchor":"7-search-its-trace"}]
---

Quickstart step 7: the daemon scans every five minutes; run hev index to index now, then list and search traces filtered by the order name, since sessions under the run's folder carry the order tag.
