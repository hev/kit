---
id: "dispatch#find-a-runs-traces"
title: "Dispatch"
heading: "Find a run's traces"
group: "Use"
order: 87
url: "/docs/dispatch#find-a-runs-traces"
anchor: "find-a-runs-traces"
terms: ["find","traces","sessions","inside","directory","tagged","order","name","including","validator","list","search","belongs","rooted","trace","follow","commands","extend","runs","context","recipe","also","digest","rate","limit","headers","test","fail","dispatch","rework","retry","amend","validate","whose","working","under","indexing","time","carry","parent"]
hash: "fa74eda19a278b2bbf9c76dd8e399ebef9f527732eb007157c4f827e9c45048a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev traces ls --order api-rate-limit-headers\nhev traces search 'why did the 429 test fail' --order api-rate-limit-headers","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"~/dispatch/\u003cname\u003e/","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"order=\u003cname\u003e","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"hev dispatch ORDER.md","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"--trace ID","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"rework","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"retry","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"say","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"amend","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"validate","chunkId":"dispatch#find-a-runs-traces"},{"kind":"code","literal":"hev traces ls --context DIGEST","chunkId":"dispatch#find-a-runs-traces"}]
sources: [{"chunkId":"dispatch#find-a-runs-traces","url":"/docs/dispatch#find-a-runs-traces","anchor":"find-a-runs-traces"}]
---

Sessions inside a run directory are tagged with the order name, including validator sessions, so you can list and search a run's traces. Each run belongs to an ask-rooted trace that follow-up commands extend, and runs with a context recipe are also tagged by digest.
