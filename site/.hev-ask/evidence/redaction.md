---
id: "evidence#redaction"
title: "Evidence"
heading: "Redaction"
group: "Use"
order: 208
url: "/docs/evidence#redaction"
anchor: "redaction"
terms: ["redaction","text","manifest","strings","pass","through","secret","scrubber","replacing","secrets","marker","items","flagged","redacted","screenshots","video","cannot","avoid","capturing","credentials","client","data","rule","fingerprint","true","sha256","every","item","string","person","agent","writes","title","summary","provenance","names","captions","urls","metadata","claims"]
hash: "721283bd2557d5be4eb3138aac2c35e0e1d8b96e4c312995bae247d015d26164"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[REDACTED:rule#fingerprint]","chunkId":"evidence#redaction"},{"kind":"code","literal":"redacted: true","chunkId":"evidence#redaction"},{"kind":"code","literal":"sha256","chunkId":"evidence#redaction"}]
sources: [{"chunkId":"evidence#redaction","url":"/docs/evidence#redaction","anchor":"redaction"}]
---

All text and manifest strings pass through kit's secret scrubber, replacing secrets with a redaction marker; text items are flagged redacted. Screenshots and video cannot be redacted, so avoid capturing credentials or client data.
