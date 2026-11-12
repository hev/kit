---
id: "orchestrator-examples#the-source"
title: "Orchestrator examples"
heading: "The source"
group: "Use"
order: 180
url: "/docs/orchestrator-examples#the-source"
anchor: "the-source"
terms: ["source","plugin","answers","describe","call","events","printing","json","final","cursor","optional","example","python","linear","queries","open","labelled","issues","shell","wrapper","reading","1password","secrets","runner","alternatively","spec","name","environment","variable","passed","only","path","reads","exec","automation","args","team","true","kind","fields"]
hash: "a6f68be42a5c116708579076a642aae20d12bd54a85dc0ca0dbf4e8126926eff"
mode: "agent-primary"
facts: [{"kind":"code","literal":"#!/bin/sh\n# On PATH as hev-linear. Reads the Linear key at each call, into the\n# environment of linear-source only.\nexec hev secrets run --ref LINEAR_API_KEY=op://automation/linear/api-key -- \\\n  linear-source \"$@\"","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"source.args","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"--team","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"source.team","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"hev-linear --hev-describe ...","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"{\"source\": true}","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"hev-linear events ...","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"{\"cursor\": \"...\"}","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"id","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"kind","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"url","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"fields","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"fields.status","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"hev-linear ack ...","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"HEV_SOURCE_CURSOR","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"linear-source","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"PATH","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"LINEAR_API_KEY","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"hev-linear","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"hev secrets run","chunkId":"orchestrator-examples#the-source"},{"kind":"code","literal":"source.env: [LINEAR_API_KEY]","chunkId":"orchestrator-examples#the-source"}]
sources: [{"chunkId":"orchestrator-examples#the-source","url":"/docs/orchestrator-examples#the-source","anchor":"the-source"}]
---

A source plugin answers a describe call, an events call printing JSON events and a final cursor, and an optional ack. The example Python Linear source queries open labelled issues, with a shell wrapper reading the API key from 1Password per call via the secrets runner; alternatively the spec can name an environment variable passed only to the source.
