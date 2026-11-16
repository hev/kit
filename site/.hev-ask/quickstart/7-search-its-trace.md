---
id: "quickstart#7-search-its-trace"
title: "Quickstart"
heading: "7. Search its trace"
group: "Start"
order: 295
url: "/docs/quickstart#7-search-its-trace"
anchor: "7-search-its-trace"
terms: ["search","trace","daemon","scans","every","five","minutes","default","index","immediately","list","traces","filtered","order","name","since","sessions","directory","carry","local","matches","words","shared","archive","also","meaning","across","machines","first","tests","find","tell","came","back","quote","best","matching","passage","docs","https"]
hash: "0d5d2bea6bef890f15a56a8b9ee9e789b29194e7cad0dd5e73cad85fa7c44e2b"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev index\nhev traces list --order first-run\nhev traces search \"how to run the tests\" --order first-run","chunkId":"quickstart#7-search-its-trace"},{"kind":"code","literal":"Find the trace of the run first-run: run `hev index` to index now, then `hev traces list --order first-run` and `hev traces search \"how to run the tests\" --order first-run`. Tell me which sessions came back and quote the best matching passage.\nDocs: https://hevkit.com/docs/quickstart#7-search-its-trace (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#7-search-its-trace"},{"kind":"code","literal":"hev index","chunkId":"quickstart#7-search-its-trace"},{"kind":"code","literal":"~/dispatch/first-run/","chunkId":"quickstart#7-search-its-trace"},{"kind":"code","literal":"order=first-run","chunkId":"quickstart#7-search-its-trace"}]
sources: [{"chunkId":"quickstart#7-search-its-trace","url":"/docs/quickstart#7-search-its-trace","anchor":"7-search-its-trace"}]
---

The daemon scans every five minutes by default, or you can index immediately, then list and search traces filtered by the order name since sessions in the run directory carry that tag. Local search matches words; a shared archive also matches meaning across machines.
