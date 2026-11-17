---
id: "traces#what-a-trace-holds"
title: "Traces"
heading: "What a trace holds"
group: "Use"
order: 383
url: "/docs/traces#what-a-trace-holds"
anchor: "what-a-trace-holds"
terms: ["trace","holds","rooted","made","asks","root","plus","follow","orders","attempts","browser","validations","verdicts","worker","sessions","their","turns","tool","calls","dispatch","records","metadata","text","learn","about","caller","such","terminal","session","harness","host","time","attempt","gets","span","started","context","anything","dispatches","joins"]
hash: "275b4948765a3e5073c6fe782216248b2de4221894cbf1c208051b6d97bb0a49"
mode: "agent-primary"
facts: [{"kind":"code","literal":"say","chunkId":"traces#what-a-trace-holds"},{"kind":"code","literal":"hev dispatch","chunkId":"traces#what-a-trace-holds"},{"kind":"code","literal":"meta.json","chunkId":"traces#what-a-trace-holds"},{"kind":"code","literal":"trace_id","chunkId":"traces#what-a-trace-holds"},{"kind":"code","literal":"HEV_CALLER_SESSION","chunkId":"traces#what-a-trace-holds"},{"kind":"code","literal":"## Acceptance","chunkId":"traces#what-a-trace-holds"},{"kind":"code","literal":"TRACEPARENT","chunkId":"traces#what-a-trace-holds"},{"kind":"code","literal":"HEV_TRACE_ID","chunkId":"traces#what-a-trace-holds"},{"kind":"code","literal":"HEV_SPAN_ID","chunkId":"traces#what-a-trace-holds"}]
sources: [{"chunkId":"traces#what-a-trace-holds","url":"/docs/traces#what-a-trace-holds","anchor":"what-a-trace-holds"}]
---

An ask-rooted trace is made of asks (the root plus follow-ups), orders, attempts, browser validations with verdicts, and worker sessions with their turns and tool calls. Dispatch records the trace in the run's metadata: a W3C trace ID, the root ask's text, and what it can learn about the caller, such as terminal session, harness, host and time. Each attempt gets its own span and the worker is started with that trace context, so anything it dispatches joins the same trace and its indexed session carries the trace ID.
