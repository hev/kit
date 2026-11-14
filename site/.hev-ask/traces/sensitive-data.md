---
id: "traces#sensitive-data"
title: "Traces"
heading: "Sensitive data"
group: "Use"
order: 338
url: "/docs/traces#sensitive-data"
anchor: "sensitive-data"
terms: ["sensitive","data","sessions","contain","whatever","agents","including","secrets","customer","scrubbing","default","gitleaks","rules","plus","token","credential","high","entropy","replacing","matches","fingerprints","cannot","catch","everything","source","transcripts","stay","daemon","only","comfortable","archiving","protect","archive","namespace","served","address","redact","false","capture","serve"]
hash: "65ae9ffa3d3b58959cb97420464a16d1bf8fc0fad2e239331158841e3db6fafa"
mode: "agent-primary"
facts: [{"kind":"code","literal":"redact = false","chunkId":"traces#sensitive-data"},{"kind":"code","literal":"[capture]","chunkId":"traces#sensitive-data"},{"kind":"code","literal":"hev serve","chunkId":"traces#sensitive-data"},{"kind":"value","literal":"config.toml","chunkId":"traces#sensitive-data"}]
sources: [{"chunkId":"traces#sensitive-data","url":"/docs/traces#sensitive-data","anchor":"sensitive-data"}]
---

Sessions contain whatever agents saw, including secrets and customer data. Scrubbing is on by default using gitleaks rules plus token, key, JWT, URL credential and high-entropy rules, replacing matches with fingerprints, but it cannot catch everything and source transcripts stay raw; run the daemon only where you are comfortable archiving sessions and protect the archive, namespace, key and any served address.
