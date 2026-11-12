---
id: "traces#what-a-trace-holds"
title: "Traces"
heading: "What a trace holds"
group: "Use"
order: 289
url: "/docs/traces#what-a-trace-holds"
anchor: "what-a-trace-holds"
terms: ["trace","holds","consists","asks","root","follow","like","rework","notes","amendments","says","validation","requests","continuing","orders","attempts","launch","reworks","retries","adding","more","runs","verdicts","worker","sessions","dispatch","records","requester","details","metadata","workers","context","their","environment","anything","joins","same","meta","json","traceparent"]
hash: "a03a794a1f52d6bc9efc941b36686fccd5ca94cb1fbc45ee71b689c0c4f3140c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"say","chunkId":"traces#what-a-trace-holds"},{"kind":"code","literal":"hev dispatch","chunkId":"traces#what-a-trace-holds"},{"kind":"code","literal":"meta.json","chunkId":"traces#what-a-trace-holds"},{"kind":"code","literal":"TRACEPARENT","chunkId":"traces#what-a-trace-holds"},{"kind":"code","literal":"HEV_TRACE_ID","chunkId":"traces#what-a-trace-holds"}]
sources: [{"chunkId":"traces#what-a-trace-holds","url":"/docs/traces#what-a-trace-holds","anchor":"what-a-trace-holds"}]
---

A trace consists of asks (the root and follow-ups like rework notes, amendments, says, validation requests or continuing orders), orders, attempts (each launch, with reworks and retries adding more), validation runs with verdicts, and worker sessions. Dispatch records the trace ID, root ask and requester details in the run metadata, and workers get trace context in their environment so anything they dispatch joins the same trace.
