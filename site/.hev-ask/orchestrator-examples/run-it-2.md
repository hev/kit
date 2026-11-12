---
id: "orchestrator-examples#run-it-2"
title: "Orchestrator examples"
heading: "Run it"
group: "Use"
order: 176
url: "/docs/orchestrator-examples#run-it-2"
anchor: "run-it-2"
terms: ["start","gauntlet","loop","dispatching","order","builder","host","applying","orchestrator","spec","track","validation","column","item","verdict","evidence","dispatch","empty","state","page","buildbox","orchestrate","apply","attempt","show","json","pass","fail","runs","instead","result","check","progress","lists","tell","acceptance","docs","https","hevkit","examples"]
hash: "de313a58f35f109e06aa76cc8145e1d26c724f3967dee8b9b0a9c12518682182"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch empty-state-page.md --host buildbox\nhev orchestrate apply -f gauntlet.md --dry-run\nhev orchestrate apply -f gauntlet.md\nhev dispatch ls                                  # VALIDATION column per attempt\nhev dispatch show empty-state-page               # verdict.json: each item, pass or fail, with evidence\nhev orchestrate show gauntlet-empty-state","chunkId":"orchestrator-examples#run-it-2"}]
sources: [{"chunkId":"orchestrator-examples#run-it-2","url":"/docs/orchestrator-examples#run-it-2","anchor":"run-it-2"}]
---

Start the Gauntlet loop by dispatching the bar order to the builder host, then applying the orchestrator spec, and track the validation column and per-item verdict with evidence.
