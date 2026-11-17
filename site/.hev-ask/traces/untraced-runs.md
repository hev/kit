---
id: "traces#untraced-runs"
title: "Traces"
heading: "Untraced runs"
group: "Use"
order: 382
url: "/docs/traces#untraced-runs"
anchor: "untraced-runs"
terms: ["untraced","runs","trace","appears","ways","before","tracing","historical","dispatched","traces","existed","labelled","neutrally","deliberately","recorded","such","orchestrator","swarm","launches","always","reason","shown","hidden","default","count","control","reveal","while","listed","stays","through","reworks","retries","pretrace","record","fields","history","without","rooted","fault"]
hash: "9d15ab7ea4e164caeb1b58d5d4a75bcd59ada3c63a994ded2580da21812ae867"
mode: "agent-primary"
facts: [{"kind":"code","literal":"ask_pretrace=1","chunkId":"traces#untraced-runs"}]
sources: [{"chunkId":"traces#untraced-runs","url":"/docs/traces#untraced-runs","anchor":"untraced-runs"}]
---

A run with no trace appears one of two ways: 'before tracing' for historical runs dispatched before traces existed, labelled neutrally, or 'untraced' for runs deliberately recorded that way (such as orchestrator or swarm launches), always with the reason shown. Pre-tracing runs are hidden by default with a count and a control to reveal them, while untraced runs are always listed. An untraced run stays untraced through its reworks and retries.
