---
id: "traces#what-a-trace-holds"
title: "Traces"
heading: "What a trace holds"
group: "Use"
order: 345
url: "/docs/traces#what-a-trace-holds"
anchor: "what-a-trace-holds"
terms: ["trace","holds","consists","asks","root","plus","follow","such","rework","notes","amendments","says","validation","requests","continuation","orders","attempts","launch","validations","verdicts","worker","sessions","dispatch","records","requester","details","metadata","workers","inherit","context","their","environment","anything","joins","same","meta","json","traceparent","part","something"]
hash: "a03a794a1f52d6bc9efc941b36686fccd5ca94cb1fbc45ee71b689c0c4f3140c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"say","chunkId":"traces#what-a-trace-holds"},{"kind":"code","literal":"hev dispatch","chunkId":"traces#what-a-trace-holds"},{"kind":"code","literal":"meta.json","chunkId":"traces#what-a-trace-holds"},{"kind":"code","literal":"TRACEPARENT","chunkId":"traces#what-a-trace-holds"},{"kind":"code","literal":"HEV_TRACE_ID","chunkId":"traces#what-a-trace-holds"}]
sources: [{"chunkId":"traces#what-a-trace-holds","url":"/docs/traces#what-a-trace-holds","anchor":"what-a-trace-holds"}]
---

A trace consists of asks (the root plus follow-ups such as rework notes, amendments, says, validation requests or continuation orders), orders, attempts (one per launch), validations with verdicts, and worker sessions. Dispatch records the trace ID and requester details in the run metadata, and workers inherit trace context in their environment so anything they dispatch joins the same trace.
