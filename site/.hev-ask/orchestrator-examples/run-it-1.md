---
id: "orchestrator-examples#run-it-1"
title: "Orchestrator examples"
heading: "Run it"
group: "Use"
order: 234
url: "/docs/orchestrator-examples#run-it-1"
anchor: "run-it-1"
terms: ["release","coordinator","host","train","runs","live","evidence","local","rounds","continue","while","laptop","sleeps","view","checkpoint","round","logs","messages","send","land","inbox","trigger","early","after","progress","finishes","orchestrate","apply","show","blocking","context","waits","drop","buildbox","example","start","result","confirm","tell","fact"]
hash: "4d4202d52fd205613bd9b8da918a5992b589bb1ad99eadba932153f118d714c7"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev orchestrate apply -f release-0-7.md --dry-run\nhev orchestrate apply -f release-0-7.md\nhev orchestrate show release-0-7          # checkpoint, with \"Blocking the release\" on top\nhev orchestrate logs release-0-7 --round 6\nhev orchestrate say release-0-7 \"The context PR waits for 0.8; drop it from the train\"","chunkId":"orchestrator-examples#run-it-1"},{"kind":"code","literal":"buildbox","chunkId":"orchestrator-examples#run-it-1"},{"kind":"code","literal":"say","chunkId":"orchestrator-examples#run-it-1"}]
sources: [{"chunkId":"orchestrator-examples#run-it-1","url":"/docs/orchestrator-examples#run-it-1","anchor":"run-it-1"}]
---

Run the release coordinator on the host where the train's runs live so evidence is local and rounds continue while your laptop sleeps. You can view its checkpoint and round logs, and messages you send land in its inbox and trigger an early round after any in-progress round finishes.
