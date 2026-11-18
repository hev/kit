---
id: "traces#sensitive-data"
title: "Traces"
heading: "Sensitive data"
group: "Use"
order: 372
url: "/docs/traces#sensitive-data"
anchor: "sensitive-data"
terms: ["sensitive","data","sessions","contain","whatever","agent","including","pasted","keys","secrets","command","lines","file","contents","customer","scrubs","before","writing","default","standard","secret","detection","rule","plus","rules","tokens","private","jwts","credentials","high","entropy","assignments","replacing","matches","fingerprint","cannot","catch","everything","source","transcripts"]
hash: "95b8b26a20d4b8b54b3c72a70e4647fa765ed2886ffc5b64adc379b27b193b7c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"redact = false","chunkId":"traces#sensitive-data"},{"kind":"code","literal":"[capture]","chunkId":"traces#sensitive-data"},{"kind":"code","literal":"hev serve","chunkId":"traces#sensitive-data"},{"kind":"value","literal":"config.toml","chunkId":"traces#sensitive-data"}]
sources: [{"chunkId":"traces#sensitive-data","url":"/docs/traces#sensitive-data","anchor":"sensitive-data"}]
---

Sessions contain whatever the agent saw, including pasted keys, secrets on command lines, file contents and customer data. kit scrubs secrets before writing, on by default, using a standard secret-detection rule set plus rules for API tokens, private keys, JWTs, URL credentials and high-entropy assignments, replacing matches with a fingerprint. It cannot catch everything, the source transcripts on disk stay raw, and screenshots are not scrubbed, so only capture on machines whose sessions you are comfortable storing and protect the archive, its key and any dashboard address like the transcripts themselves.
