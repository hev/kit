---
id: "orchestrator-examples#run-it-2"
title: "Orchestrator examples"
heading: "Run it"
group: "Use"
order: 235
url: "/docs/orchestrator-examples#run-it-2"
anchor: "run-it-2"
terms: ["gauntlet","loop","dispatch","order","builder","host","validate","apply","orchestrator","spec","progress","shows","validation","column","attempt","verdict","file","listing","acceptance","item","pass","fail","evidence","empty","state","page","buildbox","orchestrate","show","json","runs","start","instead","result","check","lists","tell","docs","https","hevkit"]
hash: "de313a58f35f109e06aa76cc8145e1d26c724f3967dee8b9b0a9c12518682182"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch empty-state-page.md --host buildbox\nhev orchestrate apply -f gauntlet.md --dry-run\nhev orchestrate apply -f gauntlet.md\nhev dispatch ls                                  # VALIDATION column per attempt\nhev dispatch show empty-state-page               # verdict.json: each item, pass or fail, with evidence\nhev orchestrate show gauntlet-empty-state","chunkId":"orchestrator-examples#run-it-2"}]
sources: [{"chunkId":"orchestrator-examples#run-it-2","url":"/docs/orchestrator-examples#run-it-2","anchor":"run-it-2"}]
---

To run the Gauntlet loop, dispatch the bar order on the builder's host, then validate and apply the orchestrator spec. Progress shows in a validation column per attempt and in a verdict file listing each acceptance item as pass or fail with evidence.
