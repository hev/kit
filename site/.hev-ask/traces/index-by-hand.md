---
id: "traces#index-by-hand"
title: "Traces"
heading: "Index by hand"
group: "Use"
order: 308
url: "/docs/traces#index-by-hand"
anchor: "index-by-hand"
terms: ["index","hand","hevd","indexes","runs","pass","demand","useful","first","backfill","limit","tier","option","also","includes","tool","results","most","bytes","unchanged","transcripts","skipped","chunk","content","addressed","reruns","cheap","safe","text","calls","parse","without","writing","machine","agent","instead","waiting","tell","would","write"]
hash: "2158f68cdb138e38fbc39ad30bda44d94e958a4cf6c57b72b8e036d11485fbbd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev index                        # text and tool calls\nhev index --tier all             # also tool results\nhev index --limit 20 --dry-run   # parse and chunk without writing","chunkId":"traces#index-by-hand"},{"kind":"code","literal":"Index this machine's agent transcripts now instead of waiting for hevd. First run `hev index --limit 20 --dry-run` and tell me what it would write. Then run `hev index` for text and tool calls (add `--tier all` only if I ask for tool results too). Report how many units were indexed.\nDocs: https://hevkit.com/docs/traces#index-by-hand (all pages: https://hevkit.com/llms.txt)","chunkId":"traces#index-by-hand"},{"kind":"code","literal":"hev index","chunkId":"traces#index-by-hand"},{"kind":"code","literal":"--tier tool_result","chunkId":"traces#index-by-hand"},{"kind":"code","literal":"--tier all","chunkId":"traces#index-by-hand"}]
sources: [{"chunkId":"traces#index-by-hand","url":"/docs/traces#index-by-hand","anchor":"index-by-hand"}]
---

hevd indexes on its own, but hev index runs a pass on demand, useful for a first backfill or a dry run with a limit. A tier option also includes tool results, which are most of the bytes. Unchanged transcripts are skipped and chunk IDs are content addressed, so reruns are cheap and safe.
