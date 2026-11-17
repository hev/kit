---
id: "dispatch#find-a-runs-traces"
title: "Dispatch"
heading: "Find a run's traces"
group: "Use"
order: 93
url: "/docs/dispatch#find-a-runs-traces"
anchor: "find-a-runs-traces"
terms: ["find","traces","sessions","under","directory","tagged","order","name","indexing","validator","carry","parent","listed","searched","rate","limit","headers","search","test","fail","dispatch","trace","rework","retry","amend","validate","show","meta","json","context","digest","whose","working","time","replace","question","list","including","quote","best"]
hash: "1a3d23b60d8ade6a23c91683c2fddd5e63f90ca9920159754a50b70c809ab79f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev traces ls --order api-rate-limit-headers\nhev traces search 'why did the 429 test fail' --order api-rate-limit-headers","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"~/dispatch/\u003cname\u003e/","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"order=\u003cname\u003e","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"hev dispatch ORDER.md","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"--trace ID","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"rework","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"retry","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"say","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"amend","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"validate","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"show","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"trace_id","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"meta.json","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"hev traces ls --context DIGEST","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"hev traces search --context DIGEST","chunkId":"dispatch#find-a-runs-traces"}]
sources: [{"chunkId":"dispatch#find-a-runs-traces","url":"/docs/dispatch#find-a-runs-traces","anchor":"find-a-runs-traces"}]
---

Sessions under a run's directory are tagged with its order name at indexing, and validator sessions carry the parent's name. Traces can be listed and searched by order.
