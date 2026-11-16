---
id: "dispatch#browser-validation"
title: "Dispatch"
heading: "Browser validation"
group: "Use"
order: 146
url: "/docs/dispatch#browser-validation"
anchor: "browser-validation"
terms: ["browser","validation","order","opts","gets","independent","check","after","finishes","done","validator","configured","host","works","implementer","pushed","head","checks","acceptance","item","real","saves","screenshots","writes","verdict","without","editing","code","must","different","harness","model","pair","also","hand","rework","failed","relaunches","repeats","same"]
hash: "0a5b83e772484dc8a9f725e6b144c6e005e7049c64e6e7e11388e02416ead339"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch validate api-rate-limit-headers\nhev dispatch validate api-rate-limit-headers --host qa\nhev dispatch rework api-rate-limit-headers --from-validation","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"validation: browser","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"done","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"[validation]","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"../evidence/","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"preview","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"{port}","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"validate NAME","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"--host","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"--allow-same-model","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"pending","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"validate","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"rework --from-validation","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"needs-review","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"--from-validation","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"rework","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"\u003cname\u003e-v\u003cN\u003e","chunkId":"dispatch#browser-validation"}]
sources: [{"chunkId":"dispatch#browser-validation","url":"/docs/dispatch#browser-validation","anchor":"browser-validation"}]
---

An order that opts into browser validation gets an independent check after it finishes done: a validator on the configured validation host works at the implementer's pushed head, checks each Acceptance item in a real browser, saves screenshots and writes a verdict without editing code. The validator must be on a different host and use a different harness and model pair, and validation can also be run by hand. Rework from a failed verdict relaunches the implementer and validation repeats; the same item failing twice puts the run in needs-review.
