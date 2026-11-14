---
id: "orchestrator-examples#when-it-stops"
title: "Orchestrator examples"
heading: "When it stops"
group: "Use"
order: 238
url: "/docs/orchestrator-examples#when-it-stops"
anchor: "when-it-stops"
terms: ["stops","intake","loop","never","meets","goal","polls","until","stopped","most","lifetime","apply","ends","told","reapplying","starts","keeping","same","receipts","reply","maxlifetime","stop","more","days","runs","applying","spec","again"]
hash: "e0a75c3ad7329e35a77ec06cdba2ce859690256b8dec7895d13cd135e2d6349e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"until: never","chunkId":"orchestrator-examples#when-it-stops"},{"kind":"code","literal":"max_lifetime: 30d","chunkId":"orchestrator-examples#when-it-stops"},{"kind":"code","literal":"reply","chunkId":"orchestrator-examples#when-it-stops"}]
sources: [{"chunkId":"orchestrator-examples#when-it-stops","url":"/docs/orchestrator-examples#when-it-stops","anchor":"when-it-stops"}]
---

The intake loop never meets a goal; it polls until stopped, for at most its max lifetime per apply. When the lifetime ends you are told, and reapplying starts a new lifetime keeping the same receipts.
