---
id: "orchestrator-examples#what-a-round-does-1"
title: "Orchestrator examples"
heading: "What a round does"
group: "Use"
order: 233
url: "/docs/orchestrator-examples#what-a-round-does-1"
anchor: "what-a-round-does-1"
terms: ["round","does","release","coordinator","code","gathers","every","train","state","report","verdict","nothing","changed","model","called","otherwise","fresh","session","reads","packet","last","checkpoint","acts","within","authority","writes","next","blockers","first","reply","need","know","something","issue","matching","validation","since","ends","there","through"]
hash: "2c880136adc65ef13d2bacccec675e87184916a3f7b9a56606d8c409655aaf65"
mode: "agent-primary"
facts: [{"kind":"code","literal":"issue","chunkId":"orchestrator-examples#what-a-round-does-1"},{"kind":"code","literal":"train-0.7-*","chunkId":"orchestrator-examples#what-a-round-does-1"},{"kind":"code","literal":"reply.md","chunkId":"orchestrator-examples#what-a-round-does-1"}]
sources: [{"chunkId":"orchestrator-examples#what-a-round-does-1","url":"/docs/orchestrator-examples#what-a-round-does-1","anchor":"what-a-round-does-1"}]
---

Each release coordinator round, code gathers every train run's state, report, PR, CI and verdict; if nothing changed, no model is called. Otherwise a fresh session reads the packet and last checkpoint, acts within its authority, writes the next checkpoint with blockers first, and writes a reply when you need to know something.
