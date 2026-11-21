---
id: "traces#list-and-read-traces"
title: "Traces"
heading: "List and read traces"
group: "Use"
order: 427
url: "/docs/traces#list-and-read-traces"
anchor: "list-and-read-traces"
terms: ["list","read","traces","listing","time","window","order","showing","trace","session","prefix","including","json","output","normalized","turns","sessions","last","since","2026","show","3f2a","recent","coding","agent","name","dispatched","open","care","about","summarize","asked","ended","docs","https","hevkit","pages","llms","browse","orderid"]
hash: "22addf19e4becac20fb5d87ef5261e8a2fb85dbb3bf0bd92f0d4b2079a4705cd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev traces                       # sessions from the last day\nhev traces list --since 5d\nhev traces list 2026-10-30       # one day\nhev traces list --order ORDER_ID\nhev traces show 3f2a             # one trace, by session ID prefix\nhev traces show --json 3f2a      # normalized turns as JSON","chunkId":"traces#list-and-read-traces"},{"kind":"code","literal":"List my recent coding-agent sessions with `hev traces list --since 5d` (or `--order ORDER_NAME` for one dispatched order). Then open the one I care about, SESSION_ID_PREFIX, with `hev traces show SESSION_ID_PREFIX` and summarize what the session asked for, what it did and how it ended.\nDocs: https://hevkit.com/docs/traces#list-and-read-traces (all pages: https://hevkit.com/llms.txt)","chunkId":"traces#list-and-read-traces"},{"kind":"code","literal":"hev browse","chunkId":"traces#list-and-read-traces"}]
sources: [{"chunkId":"traces#list-and-read-traces","url":"/docs/traces#list-and-read-traces","anchor":"list-and-read-traces"}]
---

CLI for listing traces by time window, day or order and showing one trace by session-ID prefix, including JSON output of normalized turns.
