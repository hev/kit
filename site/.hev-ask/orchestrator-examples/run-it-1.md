---
id: "orchestrator-examples#run-it-1"
title: "Orchestrator examples"
heading: "Run it"
group: "Use"
order: 225
url: "/docs/orchestrator-examples#run-it-1"
anchor: "run-it-1"
terms: ["release","coordinator","host","train","runs","live","evidence","local","rounds","continue","while","laptop","sleeps","apply","view","checkpoint","blockers","read","round","logs","pass","facts","message","lands","inbox","triggers","early","after","current","finishes","orchestrate","show","blocking","context","waits","drop","buildbox","example","start","result"]
hash: "4d4202d52fd205613bd9b8da918a5992b589bb1ad99eadba932153f118d714c7"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev orchestrate apply -f release-0-7.md --dry-run\nhev orchestrate apply -f release-0-7.md\nhev orchestrate show release-0-7          # checkpoint, with \"Blocking the release\" on top\nhev orchestrate logs release-0-7 --round 6\nhev orchestrate say release-0-7 \"The context PR waits for 0.8; drop it from the train\"","chunkId":"orchestrator-examples#run-it-1"},{"kind":"code","literal":"buildbox","chunkId":"orchestrator-examples#run-it-1"},{"kind":"code","literal":"say","chunkId":"orchestrator-examples#run-it-1"}]
sources: [{"chunkId":"orchestrator-examples#run-it-1","url":"/docs/orchestrator-examples#run-it-1","anchor":"run-it-1"}]
---

Run the release coordinator on the host where the train's runs live so evidence is local and rounds continue while your laptop sleeps. Dry-run then apply, view its checkpoint with blockers on top, read a round's logs, and pass it facts; a message lands in its inbox and triggers an early round after any current one finishes.
