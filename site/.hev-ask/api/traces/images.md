---
id: "api/traces#images"
title: "Traces API"
heading: "Images"
group: "API"
order: 64
url: "/docs/api/traces#images"
anchor: "images"
terms: ["images","transcript","upload","their","hash","content","addressed","write","once","existing","returns","bytes","traces","sha256","etag","skipped","yaml","uploads","image","server","checks","answers","already","stored","type","jpeg","webp","size","dimensions","blocks","point","through","says","capturing","host"]
hash: "75dae2c0a14966a33b48cb3e04d448572ef67c5d5843d789aee17f42f8ef1acc"
mode: "source-primary"
facts: [{"kind":"code","literal":"PUT /v1/traces/images/{sha256}","chunkId":"api/traces#images"},{"kind":"code","literal":"sha256","chunkId":"api/traces#images"},{"kind":"code","literal":"201","chunkId":"api/traces#images"},{"kind":"code","literal":"200","chunkId":"api/traces#images"},{"kind":"code","literal":"GET","chunkId":"api/traces#images"},{"kind":"code","literal":"ETag","chunkId":"api/traces#images"},{"kind":"code","literal":"images[].sha256","chunkId":"api/traces#images"},{"kind":"code","literal":"skipped","chunkId":"api/traces#images"},{"kind":"value","literal":"kit-v1.yaml","chunkId":"api/traces#images"}]
sources: [{"chunkId":"api/traces#images","url":"/docs/api/traces#images","anchor":"images"}]
---

Transcript images upload by their SHA-256 hash, content-addressed and write-once (201 new, 200 existing); GET returns the bytes.
