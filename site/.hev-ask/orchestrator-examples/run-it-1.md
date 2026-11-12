---
id: "orchestrator-examples#run-it-1"
title: "Orchestrator examples"
heading: "Run it"
group: "Use"
order: 175
url: "/docs/orchestrator-examples#run-it-1"
anchor: "run-it-1"
terms: ["release","coordinator","host","train","runs","evidence","local","continues","while","laptop","sleeps","check","checkpoint","round","logs","message","facts","start","early","after","current","finishes","orchestrate","apply","show","blocking","context","waits","drop","buildbox","rounds","continue","example","result","confirm","tell","fact","given","docs","https"]
hash: "4d4202d52fd205613bd9b8da918a5992b589bb1ad99eadba932153f118d714c7"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev orchestrate apply -f release-0-7.md --dry-run\nhev orchestrate apply -f release-0-7.md\nhev orchestrate show release-0-7          # checkpoint, with \"Blocking the release\" on top\nhev orchestrate logs release-0-7 --round 6\nhev orchestrate say release-0-7 \"The context PR waits for 0.8; drop it from the train\"","chunkId":"orchestrator-examples#run-it-1"},{"kind":"code","literal":"buildbox","chunkId":"orchestrator-examples#run-it-1"},{"kind":"code","literal":"say","chunkId":"orchestrator-examples#run-it-1"}]
sources: [{"chunkId":"orchestrator-examples#run-it-1","url":"/docs/orchestrator-examples#run-it-1","anchor":"run-it-1"}]
---

Run the release coordinator on the host with the train's runs so evidence is local and it continues while the laptop sleeps; check its checkpoint and round logs, and message it facts, which start a round early after any current round finishes.
