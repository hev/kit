---
id: "orchestrator-examples#when-it-stops"
title: "Orchestrator examples"
heading: "When it stops"
group: "Use"
order: 188
url: "/docs/orchestrator-examples#when-it-stops"
anchor: "when-it-stops"
terms: ["stops","intake","loop","never","meets","goal","polls","until","stopped","most","days","apply","after","reapplying","starts","lifetime","same","receipts","reply","maxlifetime","stop","more","runs","told","applying","spec","again"]
hash: "e0a75c3ad7329e35a77ec06cdba2ce859690256b8dec7895d13cd135e2d6349e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"until: never","chunkId":"orchestrator-examples#when-it-stops"},{"kind":"code","literal":"max_lifetime: 30d","chunkId":"orchestrator-examples#when-it-stops"},{"kind":"code","literal":"reply","chunkId":"orchestrator-examples#when-it-stops"}]
sources: [{"chunkId":"orchestrator-examples#when-it-stops","url":"/docs/orchestrator-examples#when-it-stops","anchor":"when-it-stops"}]
---

The intake loop never meets a goal; it polls until stopped or for at most 30 days per apply, after which reapplying starts a new lifetime with the same receipts.
