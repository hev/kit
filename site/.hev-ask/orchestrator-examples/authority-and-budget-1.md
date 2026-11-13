---
id: "orchestrator-examples#authority-and-budget-1"
title: "Orchestrator examples"
heading: "Authority and budget"
group: "Use"
order: 169
url: "/docs/orchestrator-examples#authority-and-budget-1"
anchor: "authority-and-budget-1"
terms: ["authority","budget","release","coordinator","rework","retry","validate","dispatch","notify","admission","only","accepts","orders","whose","issue","matches","train","prefix","repo","daily","counts","every","start","including","reworks","rule","guidance","while","enforces","runs","concurrent","send","back","once","restart","died","without","waiting","lets","browser"]
hash: "47282fa80e64c068a57542603e68f8eb88c1b51290b8ea8a56722efdc78c2812"
mode: "agent-primary"
facts: [{"kind":"code","literal":"rework","chunkId":"orchestrator-examples#authority-and-budget-1"},{"kind":"code","literal":"retry","chunkId":"orchestrator-examples#authority-and-budget-1"},{"kind":"code","literal":"validate","chunkId":"orchestrator-examples#authority-and-budget-1"},{"kind":"code","literal":"dispatch","chunkId":"orchestrator-examples#authority-and-budget-1"},{"kind":"code","literal":"issue","chunkId":"orchestrator-examples#authority-and-budget-1"},{"kind":"code","literal":"train-0.7-*","chunkId":"orchestrator-examples#authority-and-budget-1"},{"kind":"code","literal":"kit","chunkId":"orchestrator-examples#authority-and-budget-1"},{"kind":"code","literal":"say","chunkId":"orchestrator-examples#authority-and-budget-1"},{"kind":"code","literal":"notify","chunkId":"orchestrator-examples#authority-and-budget-1"},{"kind":"code","literal":"runs_per_day: 10","chunkId":"orchestrator-examples#authority-and-budget-1"},{"kind":"code","literal":"concurrent_runs: 4","chunkId":"orchestrator-examples#authority-and-budget-1"}]
sources: [{"chunkId":"orchestrator-examples#authority-and-budget-1","url":"/docs/orchestrator-examples#authority-and-budget-1","anchor":"authority-and-budget-1"}]
---

The release coordinator can rework, retry, validate, dispatch, say and notify, but admission only accepts orders whose issue matches the train's prefix in one repo. The daily budget counts every start including reworks, and the one-rework rule is only guidance while admission enforces the budget.
