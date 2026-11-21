---
id: "traces#sensitive-data"
title: "Traces"
heading: "Sensitive data"
group: "Use"
order: 431
url: "/docs/traces#sensitive-data"
anchor: "sensitive-data"
terms: ["sensitive","data","sessions","hold","anything","agent","including","keys","customer","hevd","indexes","every","session","machine","scrubs","secrets","before","indexing","publishing","redact","false","capture","serve","config","toml","coding","whatever","pasted","prompt","command","line","file","contents","sources","above","runs","written","scrubbing","unless","under"]
hash: "95b8b26a20d4b8b54b3c72a70e4647fa765ed2886ffc5b64adc379b27b193b7c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"redact = false","chunkId":"traces#sensitive-data"},{"kind":"code","literal":"[capture]","chunkId":"traces#sensitive-data"},{"kind":"code","literal":"hev serve","chunkId":"traces#sensitive-data"},{"kind":"value","literal":"config.toml","chunkId":"traces#sensitive-data"}]
sources: [{"chunkId":"traces#sensitive-data","url":"/docs/traces#sensitive-data","anchor":"sensitive-data"}]
---

Sessions hold anything the agent saw, including keys and customer data; hevd indexes every session on each machine and scrubs secrets before indexing or publishing.
