---
id: "traces#across-hosts"
title: "Traces"
heading: "Across hosts"
group: "Use"
order: 364
url: "/docs/traces#across-hosts"
anchor: "across-hosts"
terms: ["across","hosts","without","layer","asks","view","only","shows","runs","host","serving","dashboard","listing","sessions","carry","trace","match","visible","below","waterfall","publishes","facts","about","names","times","text","attempt","metadata","elsewhere","show","whose","root","recorded","marked","continued","titles","scrubbed","first","while","order"]
hash: "353db878424e98d6928d6a6236d20183fcae2913c16788610b3efa2c2914116b"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev index","chunkId":"traces#across-hosts"},{"kind":"code","literal":"NAMESPACE-spans","chunkId":"traces#across-hosts"},{"kind":"code","literal":"requested_by","chunkId":"traces#across-hosts"}]
sources: [{"chunkId":"traces#across-hosts","url":"/docs/traces#across-hosts","anchor":"across-hosts"}]
---

Without a hev layer key the Asks view only shows runs from the host serving the dashboard, listing sessions that carry the trace ID but match no visible run below the waterfall. With a key each host publishes facts about its own runs (IDs, names, hosts, times, ask text, attempt metadata) so a dashboard elsewhere can show them, and a trace whose root was recorded elsewhere is marked continued. Ask text and titles are scrubbed first, while order bodies, acceptance criteria, reports and verdicts stay on the host that ran the order.
