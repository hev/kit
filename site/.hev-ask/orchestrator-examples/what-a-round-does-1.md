---
id: "orchestrator-examples#what-a-round-does-1"
title: "Orchestrator examples"
heading: "What a round does"
group: "Use"
order: 189
url: "/docs/orchestrator-examples#what-a-round-does-1"
anchor: "what-a-round-does-1"
terms: ["round","does","code","gathers","every","train","state","report","validation","verdict","nothing","changed","since","last","checkpoint","model","called","otherwise","fresh","session","acts","through","authority","writes","blockers","reply","person","must","hear","something","issue","matching","ends","there","reads","packet","next","need"]
hash: "2c880136adc65ef13d2bacccec675e87184916a3f7b9a56606d8c409655aaf65"
mode: "agent-primary"
facts: [{"kind":"code","literal":"issue","chunkId":"orchestrator-examples#what-a-round-does-1"},{"kind":"code","literal":"train-0.7-*","chunkId":"orchestrator-examples#what-a-round-does-1"},{"kind":"code","literal":"reply.md","chunkId":"orchestrator-examples#what-a-round-does-1"}]
sources: [{"chunkId":"orchestrator-examples#what-a-round-does-1","url":"/docs/orchestrator-examples#what-a-round-does-1","anchor":"what-a-round-does-1"}]
---

Code gathers every train run with its state, report, PR, CI and validation verdict. If nothing changed since the last checkpoint no model is called. Otherwise a fresh session acts through its authority, writes the checkpoint with blockers on top, and writes a reply when a person must hear something.
