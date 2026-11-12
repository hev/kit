---
id: "traces#across-hosts"
title: "Traces"
heading: "Across hosts"
group: "Use"
order: 271
url: "/docs/traces#across-hosts"
anchor: "across-hosts"
terms: ["across","hosts","without","layer","asks","view","reads","only","records","host","serving","indexer","publishes","trace","facts","runs","shared","spans","namespace","dashboard","shows","attempts","other","marking","traces","whose","root","recorded","elsewhere","continued","published","data","covers","names","times","text","attempt","details","scrubbed","index"]
hash: "353db878424e98d6928d6a6236d20183fcae2913c16788610b3efa2c2914116b"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev index","chunkId":"traces#across-hosts"},{"kind":"code","literal":"NAMESPACE-spans","chunkId":"traces#across-hosts"},{"kind":"code","literal":"requested_by","chunkId":"traces#across-hosts"}]
sources: [{"chunkId":"traces#across-hosts","url":"/docs/traces#across-hosts","anchor":"across-hosts"}]
---

Without a layer key, the Asks view reads only the run records on the host serving it. With a key, each host's indexer publishes trace facts of its own runs to a shared spans namespace so one dashboard shows asks and attempts from other hosts, marking traces whose root was recorded elsewhere as continued. Published data covers IDs, run names, hosts, times, ask text and attempt details, with text scrubbed.
