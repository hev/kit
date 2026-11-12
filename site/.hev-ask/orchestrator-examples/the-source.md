---
id: "orchestrator-examples#the-source"
title: "Orchestrator examples"
heading: "The source"
group: "Use"
order: 186
url: "/docs/orchestrator-examples#the-source"
anchor: "the-source"
terms: ["source","answers","three","calls","describe","events","final","cursor","line","optional","runs","directly","without","shell","sketch","linear","script","provided","starting","point","supported","plugin","ignores","because","receipts","drop","seen","issues","path","reads","call","environment","only","exec","secrets","automation","args","team","true","kind"]
hash: "a6f68be42a5c116708579076a642aae20d12bd54a85dc0ca0dbf4e8126926eff"
mode: "agent-primary"
facts: [{"kind":"code","literal":"#!/bin/sh\n# On PATH as hev-linear. Reads the Linear key at each call, into the\n# environment of linear-source only.\nexec hev secrets run --ref LINEAR_API_KEY=op://automation/linear/api-key -- \\\n  linear-source \"$@\"","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"source.args","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"--team","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"source.team","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"hev-linear --hev-describe ...","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"{\"source\": true}","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"hev-linear events ...","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"{\"cursor\": \"...\"}","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"id","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"kind","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"url","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"fields","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"fields.status","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"hev-linear ack ...","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"HEV_SOURCE_CURSOR","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"linear-source","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"PATH","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"LINEAR_API_KEY","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"hev-linear","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"hev secrets run","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"source.env: [LINEAR_API_KEY]","chunkId":"orchestrator-examples#the-source"}]
sources: [{"chunkId":"orchestrator-examples#the-source","url":"/docs/orchestrator-examples#the-source","anchor":"the-source"}]
---

A source answers three calls: describe, events with a final cursor line, and an optional ack. Kit runs it directly without a shell. A sketch Linear source script is provided as a starting point, not a supported plugin, and it ignores the cursor because receipts drop seen issues.
