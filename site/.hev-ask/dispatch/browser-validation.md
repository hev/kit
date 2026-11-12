---
id: "dispatch#browser-validation"
title: "Dispatch"
heading: "Browser validation"
group: "Use"
order: 84
url: "/docs/dispatch#browser-validation"
anchor: "browser-validation"
terms: ["browser","validation","orders","opting","independent","validator","separate","host","different","harness","model","checks","acceptance","item","pushed","head","real","saves","screenshots","writes","verdict","without","editing","code","started","hand","full","leaves","pending","rework","failed","relaunches","implementer","same","failing","twice","marks","needs","review","dispatch"]
hash: "0a5b83e772484dc8a9f725e6b144c6e005e7049c64e6e7e11388e02416ead339"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch validate api-rate-limit-headers\nhev dispatch validate api-rate-limit-headers --host qa\nhev dispatch rework api-rate-limit-headers --from-validation","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"validation: browser","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"done","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"[validation]","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"../evidence/","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"preview","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"{port}","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"validate NAME","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"--host","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"--allow-same-model","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"pending","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"validate","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"rework --from-validation","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"needs-review","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"--from-validation","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"rework","chunkId":"dispatch#browser-validation"},{"kind":"code","literal":"\u003cname\u003e-v\u003cN\u003e","chunkId":"dispatch#browser-validation"}]
sources: [{"chunkId":"dispatch#browser-validation","url":"/docs/dispatch#browser-validation","anchor":"browser-validation"}]
---

Orders opting into browser validation get an independent validator on a separate host with a different harness and model, which checks each Acceptance item at the pushed head in a real browser, saves screenshots and writes a verdict without editing code. Validation can be started by hand, a full validation host leaves it pending, rework from a failed verdict relaunches the implementer, and the same item failing twice marks the run needs-review.
