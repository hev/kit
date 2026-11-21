---
id: "dispatch#find-a-runs-traces"
title: "Dispatch"
heading: "Find a run's traces"
group: "Use"
order: 191
url: "/docs/dispatch#find-a-runs-traces"
anchor: "find-a-runs-traces"
terms: ["find","traces","sessions","whose","working","directory","under","folder","tagged","order","name","indexed","validator","parent","list","search","filtered","something","failed","rate","limit","headers","test","fail","orders","dispatch","trace","rework","retry","amend","validate","show","meta","json","context","digest","indexing","time","carry","replace"]
hash: "571269003fd3c65c055141d213d2b877949582dff81c0fa694aa1ad36403bae7"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev traces ls --order api-rate-limit-headers\nhev traces search 'why did the 429 test fail' --order api-rate-limit-headers","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"~/.hev/orders/\u003cname\u003e/","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"order=\u003cname\u003e","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"hev dispatch ORDER.md","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"--trace ID","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"rework","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"retry","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"say","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"amend","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"validate","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"show","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"trace_id","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"meta.json","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"hev traces ls --order NAME","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"hev traces ls --context DIGEST","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"hev traces search --context DIGEST","chunkId":"dispatch#find-a-runs-traces"}]
sources: [{"chunkId":"dispatch#find-a-runs-traces","url":"/docs/dispatch#find-a-runs-traces","anchor":"find-a-runs-traces"}]
---

Sessions whose working directory is under a run's folder are tagged with the order name when indexed, validator sessions with the parent's name. You can list and search traces filtered by order to find why something failed.
