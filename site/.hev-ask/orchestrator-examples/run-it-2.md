---
id: "orchestrator-examples#run-it-2"
title: "Orchestrator examples"
heading: "Run it"
group: "Use"
order: 181
url: "/docs/orchestrator-examples#run-it-2"
anchor: "run-it-2"
terms: ["dispatch","order","builder","host","apply","gauntlet","orchestrator","there","track","progress","through","list","validation","column","verdict","file","item","evidence","show","output","empty","state","page","buildbox","orchestrate","attempt","json","pass","fail","runs","start","loop","instead","result","check","lists","tell","acceptance","docs","https"]
hash: "de313a58f35f109e06aa76cc8145e1d26c724f3967dee8b9b0a9c12518682182"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch empty-state-page.md --host buildbox\nhev orchestrate apply -f gauntlet.md --dry-run\nhev orchestrate apply -f gauntlet.md\nhev dispatch ls                                  # VALIDATION column per attempt\nhev dispatch show empty-state-page               # verdict.json: each item, pass or fail, with evidence\nhev orchestrate show gauntlet-empty-state","chunkId":"orchestrator-examples#run-it-2"}]
sources: [{"chunkId":"orchestrator-examples#run-it-2","url":"/docs/orchestrator-examples#run-it-2","anchor":"run-it-2"}]
---

Dispatch the bar order to the builder host, then apply the Gauntlet orchestrator there. Track progress through the run list's validation column, the verdict file per item with evidence, and the orchestrator's show output.
