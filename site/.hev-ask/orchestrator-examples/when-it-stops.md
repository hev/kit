---
id: "orchestrator-examples#when-it-stops"
title: "Orchestrator examples"
heading: "When it stops"
group: "Use"
order: 311
url: "/docs/orchestrator-examples#when-it-stops"
anchor: "when-it-stops"
terms: ["stops","intake","loop","polls","until","stopped","days","apply","lifetime","ends","reply","told","applying","starts","same","receipts","never","maxlifetime","stop","more","runs","spec","again"]
hash: "e0a75c3ad7329e35a77ec06cdba2ce859690256b8dec7895d13cd135e2d6349e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"until: never","chunkId":"orchestrator-examples#when-it-stops"},{"kind":"code","literal":"max_lifetime: 30d","chunkId":"orchestrator-examples#when-it-stops"},{"kind":"code","literal":"reply","chunkId":"orchestrator-examples#when-it-stops"}]
sources: [{"chunkId":"orchestrator-examples#when-it-stops","url":"/docs/orchestrator-examples#when-it-stops","anchor":"when-it-stops"}]
---

The intake loop polls until stopped or 30 days per apply; when lifetime ends reply is told and re-applying starts a new lifetime with the same receipts.
