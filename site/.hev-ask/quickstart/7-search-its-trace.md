---
id: "quickstart#7-search-its-trace"
title: "Quickstart"
heading: "7. Search its trace"
group: "Start"
order: 235
url: "/docs/quickstart#7-search-its-trace"
anchor: "7-search-its-trace"
terms: ["search","trace","daemon","indexes","every","five","minutes","default","index","demand","list","traces","filtered","order","name","since","sessions","under","directory","tagged","local","matches","words","plus","layer","also","meaning","across","machine","sharing","archive","first","tests","find","tell","came","back","quote","best","matching"]
hash: "ab694d76fa2750345ae941501e6275f9b0db8044b17291dd77bde0d3b12b034f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev index\nhev traces list --order first-run\nhev traces search \"how to run the tests\" --order first-run","chunkId":"quickstart#7-search-its-trace"},{"kind":"code","literal":"Find the trace of the run first-run: run `hev index` to index now, then `hev traces list --order first-run` and `hev traces search \"how to run the tests\" --order first-run`. Tell me which sessions came back and quote the best matching passage.\nDocs: https://hevkit.com/docs/quickstart#7-search-its-trace (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#7-search-its-trace"},{"kind":"code","literal":"hev index","chunkId":"quickstart#7-search-its-trace"},{"kind":"code","literal":"~/dispatch/first-run/","chunkId":"quickstart#7-search-its-trace"},{"kind":"code","literal":"order=first-run","chunkId":"quickstart#7-search-its-trace"}]
sources: [{"chunkId":"quickstart#7-search-its-trace","url":"/docs/quickstart#7-search-its-trace","anchor":"7-search-its-trace"}]
---

The daemon indexes every five minutes by default, or you can index on demand, then list and search traces filtered by order name since sessions under the run's directory are tagged with that order. Local search matches words; with kit plus layer it also matches meaning across every machine sharing the archive.
