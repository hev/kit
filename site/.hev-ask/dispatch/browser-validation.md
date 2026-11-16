---
id: "dispatch#browser-validation"
title: "Dispatch"
heading: "Browser validation"
group: "Use"
order: 90
url: "/docs/dispatch#browser-validation"
anchor: "browser-validation"
terms: ["browser","validation","order","gets","independent","check","once","done","validator","configured","host","harness","model","works","pushed","head","checks","acceptance","item","real","saves","screenshots","evidence","dispatch","validate","rate","limit","headers","rework","preview","port","name","allow","same","pending","needs","review","after","finishes","supervisor"]
hash: "0a5b83e772484dc8a9f725e6b144c6e005e7049c64e6e7e11388e02416ead339"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch validate api-rate-limit-headers\nhev dispatch validate api-rate-limit-headers --host qa\nhev dispatch rework api-rate-limit-headers --from-validation","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"validation: browser","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"done","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"[validation]","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"../evidence/","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"preview","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"{port}","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"validate NAME","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"--host","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"--allow-same-model","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"pending","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"validate","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"rework --from-validation","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"needs-review","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"--from-validation","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"rework","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"\u003cname\u003e-v\u003cN\u003e","chunkId":"dispatch#browser-validation"}]
sources: [{"chunkId":"dispatch#browser-validation","url":"/docs/dispatch#browser-validation","anchor":"browser-validation"}]
---

An order with browser validation gets an independent check once the run is done. A validator on the configured validation host, with its own harness and model, works at the pushed head, checks each Acceptance item in a real browser and saves screenshots as evidence.
