---
id: "orchestrator-examples#the-source"
title: "Orchestrator examples"
heading: "The source"
group: "Use"
order: 303
url: "/docs/orchestrator-examples#the-source"
anchor: "the-source"
terms: ["source","section","shows","sample","linear","implementing","three","calls","lists","matching","poll","ignores","cursor","relies","receipts","drop","already","seen","issues","previous","arrives","path","reads","call","environment","only","exec","secrets","automation","args","team","describe","true","events","kind","fields","status","answers","runs","directly"]
hash: "a6f68be42a5c116708579076a642aae20d12bd54a85dc0ca0dbf4e8126926eff"
mode: "agent-primary"
facts: [{"kind":"code","literal":"#!/bin/sh\n# On PATH as hev-linear. Reads the Linear key at each call, into the\n# environment of linear-source only.\nexec hev secrets run --ref LINEAR_API_KEY=op://automation/linear/api-key -- \\\n  linear-source \"$@\"","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"source.args","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"--team","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"source.team","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"hev-linear --hev-describe ...","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"{\"source\": true}","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"hev-linear events ...","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"{\"cursor\": \"...\"}","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"id","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"kind","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"url","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"fields","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"fields.status","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"hev-linear ack ...","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"HEV_SOURCE_CURSOR","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"linear-source","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"PATH","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"LINEAR_API_KEY","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"hev-linear","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"hev secrets run","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"source.env: [LINEAR_API_KEY]","chunkId":"orchestrator-examples#the-source"}]
sources: [{"chunkId":"orchestrator-examples#the-source","url":"/docs/orchestrator-examples#the-source","anchor":"the-source"}]
---

The source section shows a sample source (for Linear) implementing the three calls; it lists the matching set each poll, ignores the cursor, and relies on kit's receipts to drop already-seen issues. The previous cursor arrives via an env var.
