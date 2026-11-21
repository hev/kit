---
id: "orchestrator-examples#run-it-2"
title: "Orchestrator examples"
heading: "Run it"
group: "Use"
order: 299
url: "/docs/orchestrator-examples#run-it-2"
anchor: "run-it-2"
terms: ["gauntlet","loop","host","running","builder","dispatch","order","apply","orchestrator","watch","validation","column","verdict","show","state","empty","page","buildbox","orchestrate","attempt","json","item","pass","fail","evidence","runs","start","instead","result","check","progress","lists","tell","acceptance","docs","https","hevkit","examples","pages","llms"]
hash: "de313a58f35f109e06aa76cc8145e1d26c724f3967dee8b9b0a9c12518682182"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch empty-state-page.md --host buildbox\nhev orchestrate apply -f gauntlet.md --dry-run\nhev orchestrate apply -f gauntlet.md\nhev dispatch ls                                  # VALIDATION column per attempt\nhev dispatch show empty-state-page               # verdict.json: each item, pass or fail, with evidence\nhev orchestrate show gauntlet-empty-state","chunkId":"orchestrator-examples#run-it-2"}]
sources: [{"chunkId":"orchestrator-examples#run-it-2","url":"/docs/orchestrator-examples#run-it-2","anchor":"run-it-2"}]
---

Run the Gauntlet loop on the host running the builder: dispatch the bar order, dry-run and apply the orchestrator, then watch the validation column in ls, the verdict from show, and the orchestrator state.
