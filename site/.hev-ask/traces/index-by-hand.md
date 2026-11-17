---
id: "traces#index-by-hand"
title: "Traces"
heading: "Index by hand"
group: "Use"
order: 370
url: "/docs/traces#index-by-hand"
anchor: "index-by-hand"
terms: ["index","hand","daemon","indexes","automatically","same","pass","demand","first","backfill","preview","would","written","unchanged","transcripts","skipped","chunk","content","addressed","running","cheap","safe","tool","results","most","bytes","rarely","searched","indexing","text","calls","tier","also","limit","parse","without","writing","machine","agent","instead"]
hash: "2158f68cdb138e38fbc39ad30bda44d94e958a4cf6c57b72b8e036d11485fbbd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev index                        # text and tool calls\nhev index --tier all             # also tool results\nhev index --limit 20 --dry-run   # parse and chunk without writing","chunkId":"traces#index-by-hand"},{"kind":"code","literal":"Index this machine's agent transcripts now instead of waiting for hevd. First run `hev index --limit 20 --dry-run` and tell me what it would write. Then run `hev index` for text and tool calls (add `--tier all` only if I ask for tool results too). Report how many units were indexed.\nDocs: https://hevkit.com/docs/traces#index-by-hand (all pages: https://hevkit.com/llms.txt)","chunkId":"traces#index-by-hand"},{"kind":"code","literal":"hev index","chunkId":"traces#index-by-hand"},{"kind":"code","literal":"--tier tool_result","chunkId":"traces#index-by-hand"},{"kind":"code","literal":"--tier all","chunkId":"traces#index-by-hand"}]
sources: [{"chunkId":"traces#index-by-hand","url":"/docs/traces#index-by-hand","anchor":"index-by-hand"}]
---

The daemon indexes automatically, but you can run the same pass on demand for a first backfill or a dry run to preview what would be written. Unchanged transcripts are skipped and chunk IDs are content addressed, so re-running is cheap and safe. Tool results are most of the bytes and rarely searched, so indexing them is opt-in.
