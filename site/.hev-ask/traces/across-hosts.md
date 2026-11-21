---
id: "traces#across-hosts"
title: "Traces"
heading: "Across hosts"
group: "Use"
order: 417
url: "/docs/traces#across-hosts"
anchor: "across-hosts"
terms: ["across","hosts","trace","facts","start","host","layer","indexer","publishes","shared","spans","namespace","seen","index","requested","records","hevd","cycle","also","runs","archive","read","published","span","names","times","action","text","caller","attempt","order","title","repo","branch","head","harness","model","state","session","requester"]
hash: "89a6e5a1d293070b0e88995e17c72e4f481e7450a3682a9fe375678f7832becf"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev index","chunkId":"traces#across-hosts"},{"kind":"code","literal":"NAMESPACE-spans","chunkId":"traces#across-hosts"},{"kind":"code","literal":"requested_by","chunkId":"traces#across-hosts"}]
sources: [{"chunkId":"traces#across-hosts","url":"/docs/traces#across-hosts","anchor":"across-hosts"}]
---

Each run's trace facts start on the host that ran it; with a hev layer key each host's indexer publishes them to a shared spans namespace so they can be seen across hosts.
