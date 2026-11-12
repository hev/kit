---
id: "traces#sensitive-data"
title: "Traces"
heading: "Sensitive data"
group: "Use"
order: 282
url: "/docs/traces#sensitive-data"
anchor: "sensitive-data"
terms: ["sensitive","data","agent","sessions","contain","pasted","keys","command","line","secrets","file","contents","customer","scrubs","before","writing","gitleaks","rules","plus","patterns","tokens","private","jwts","credentials","high","entropy","values","replacing","matches","fingerprints","cannot","catch","everything","source","transcripts","stay","treat","archive","namespace","served"]
hash: "65ae9ffa3d3b58959cb97420464a16d1bf8fc0fad2e239331158841e3db6fafa"
mode: "agent-primary"
facts: [{"kind":"code","literal":"redact = false","chunkId":"traces#sensitive-data"},{"kind":"code","literal":"[capture]","chunkId":"traces#sensitive-data"},{"kind":"code","literal":"hev serve","chunkId":"traces#sensitive-data"},{"kind":"value","literal":"config.toml","chunkId":"traces#sensitive-data"}]
sources: [{"chunkId":"traces#sensitive-data","url":"/docs/traces#sensitive-data","anchor":"sensitive-data"}]
---

Agent sessions can contain pasted keys, command-line secrets, file contents and customer data. kit scrubs secrets before writing using gitleaks rules plus patterns for tokens, private keys, JWTs, URL credentials and high-entropy values, replacing matches with fingerprints, but it cannot catch everything and source transcripts stay raw, so treat the archive, namespace, key and any served dashboard with care. Scrubbing can be disabled in the capture config.
