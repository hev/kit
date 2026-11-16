---
id: "concepts#validation"
title: "Concepts"
heading: "Validation"
group: "Use"
order: 117
url: "/docs/concepts#validation"
anchor: "validation"
terms: ["validation","second","agent","checking","first","work","different","machine","opted","order","after","successful","supervisor","launches","validator","harness","model","pair","checks","acceptance","item","real","browser","saves","screenshots","writes","verdict","without","editing","code","failed","feed","rework","opts","host","named","never","edits","dispatch"]
hash: "dcdf99242435081070c6e80cbc9e4d89a1b0f351f02ea8c0b32937b126dbfe0a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"validation: browser","chunkId":"concepts#validation"},{"kind":"code","literal":"[validation]","chunkId":"concepts#validation"},{"kind":"code","literal":"rework --from-validation","chunkId":"concepts#validation"}]
sources: [{"chunkId":"concepts#validation","url":"/docs/concepts#validation","anchor":"validation"}]
---

Validation is a second agent checking the first one's work on a different machine, opted into per order. After a successful run the supervisor launches a validator with a different harness and model pair that checks each Acceptance item in a real browser, saves screenshots and writes a verdict without editing code; a failed verdict can feed rework.
