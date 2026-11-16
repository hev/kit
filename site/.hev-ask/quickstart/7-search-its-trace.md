---
id: "quickstart#7-search-its-trace"
title: "Quickstart"
heading: "7. Search its trace"
group: "Start"
order: 241
url: "/docs/quickstart#7-search-its-trace"
anchor: "7-search-its-trace"
terms: ["search","trace","daemon","scans","every","five","minutes","default","index","indexes","immediately","list","traces","filtered","order","name","sessions","whose","working","directory","under","carry","whole","worker","session","found","first","tests","find","tell","came","back","quote","best","matching","passage","docs","https","hevkit","quickstart"]
hash: "d390e44f18bf9bd54d7faa6d8a0d85e1d7a725fc2daa8f2af5bbf4101bf0a520"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev index\nhev traces list --order first-run\nhev traces search \"how to run the tests\" --order first-run","chunkId":"quickstart#7-search-its-trace"},{"kind":"code","literal":"Find the trace of the run first-run: run `hev index` to index now, then `hev traces list --order first-run` and `hev traces search \"how to run the tests\" --order first-run`. Tell me which sessions came back and quote the best matching passage.\nDocs: https://hevkit.com/docs/quickstart#7-search-its-trace (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#7-search-its-trace"},{"kind":"code","literal":"hev index","chunkId":"quickstart#7-search-its-trace"},{"kind":"code","literal":"~/dispatch/first-run/","chunkId":"quickstart#7-search-its-trace"},{"kind":"code","literal":"order=first-run","chunkId":"quickstart#7-search-its-trace"}]
sources: [{"chunkId":"quickstart#7-search-its-trace","url":"/docs/quickstart#7-search-its-trace","anchor":"7-search-its-trace"}]
---

The daemon scans every five minutes by default; hev index indexes immediately. Then list or search traces filtered by the order name. Sessions whose working directory is under the run's directory carry the order name, so the whole worker session is found.
