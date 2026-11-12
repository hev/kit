---
id: "orchestrator-examples#the-loop-an-orchestrator"
title: "Orchestrator examples"
heading: "The loop: an orchestrator"
group: "Use"
order: 178
url: "/docs/orchestrator-examples#the-loop-an-orchestrator"
anchor: "the-loop-an-orchestrator"
terms: ["loop","orchestrator","gauntlet","spec","loops","every","minutes","until","goal","scoped","rework","validate","message","notify","authority","tight","budget","brief","never","judges","work","pending","failed","verdict","report","full","pass","head","stop","needs","review","alter","acceptance","save","kind","name","empty","state","page","passes"]
hash: "57050a10838b945f28301806903a88a5468dc55a7eccc82150d46c301cc21043"
mode: "agent-primary"
facts: [{"kind":"code","literal":"gauntlet.md","chunkId":"orchestrator-examples#the-loop-an-orchestrator"}]
sources: [{"chunkId":"orchestrator-examples#the-loop-an-orchestrator","url":"/docs/orchestrator-examples#the-loop-an-orchestrator","anchor":"the-loop-an-orchestrator"}]
---

The Gauntlet orchestrator spec loops every 15 minutes until the goal, scoped to one run with rework, validate, message and notify authority and a tight budget. Its brief never judges the work: validate when pending, rework from a failed verdict, report a full pass with the head SHA, stop on needs-review, and never alter Acceptance.
