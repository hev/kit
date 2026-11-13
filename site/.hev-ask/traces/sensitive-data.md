---
id: "traces#sensitive-data"
title: "Traces"
heading: "Sensitive data"
group: "Use"
order: 314
url: "/docs/traces#sensitive-data"
anchor: "sensitive-data"
terms: ["sensitive","data","sessions","hold","whatever","agent","including","secrets","customer","scrubs","before","writing","default","unless","redact","disabled","capture","config","gitleaks","rules","plus","common","tokens","keys","jwts","credentials","urls","replacing","matches","fingerprint","cannot","catch","everything","source","transcripts","stay","screenshots","scrubbed","image","turned"]
hash: "95b8b26a20d4b8b54b3c72a70e4647fa765ed2886ffc5b64adc379b27b193b7c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"redact = false","chunkId":"traces#sensitive-data"},{"kind":"code","literal":"[capture]","chunkId":"traces#sensitive-data"},{"kind":"code","literal":"hev serve","chunkId":"traces#sensitive-data"},{"kind":"value","literal":"config.toml","chunkId":"traces#sensitive-data"}]
sources: [{"chunkId":"traces#sensitive-data","url":"/docs/traces#sensitive-data","anchor":"sensitive-data"}]
---

Sessions hold whatever the agent saw, including secrets and customer data. kit scrubs secrets before writing, on by default unless redact is disabled in capture config, using gitleaks rules plus rules for common tokens, keys, JWTs and credentials in URLs, replacing matches with a fingerprint. It cannot catch everything, source transcripts stay raw, and screenshots are not scrubbed, so image capture can be turned off.
