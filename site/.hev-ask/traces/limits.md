---
id: "traces#limits"
title: "Traces"
heading: "Limits"
group: "Use"
order: 332
url: "/docs/traces#limits"
anchor: "limits"
terms: ["limits","asks","view","only","reads","records","host","serving","dashboard","split","traces","show","other","sessions","below","waterfall","root","text","order","body","acceptance","typed","prompt","linked","runs","without","trace","context","aren","backfilled","whose","across","hosts","shows","lists","session","dispatched"]
hash: "e4bfbd349b140ca96a7ead0e2a6ff5f6e8ba6d2f994dc1be4e9a11b4b3e54711"
mode: "agent-primary"
facts: [{"kind":"code","literal":"## Acceptance","chunkId":"traces#limits"}]
sources: [{"chunkId":"traces#limits","url":"/docs/traces#limits","anchor":"limits"}]
---

Limits of the Asks view: it only reads run records on the host serving the dashboard, so split-host traces show other sessions below the waterfall; the root ask text is the order body up to Acceptance, your own typed prompt isn't linked yet, and runs without trace context aren't backfilled.
