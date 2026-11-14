---
id: "context#3-build-it-and-read-it"
title: "Composable context"
heading: "3. Build it and read it"
group: "Use"
order: 127
url: "/docs/context#3-build-it-and-read-it"
anchor: "3-build-it-and-read-it"
terms: ["build","read","building","recipe","produces","directory","assembled","context","text","agent","reads","selected","files","byte","manifest","hashes","digest","offline","html","page","review","serves","loopback","only","builds","reproducible","never","overwrite","bundles","compared","side","root","docs","json","bundle","diff","before","after","materialized","index"]
hash: "04d7aca64e6bd7adf3716ad0c453b9a5630b25f9bb15a7e67ef4d62fac06c8b3"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev context build --root . --recipe .hev/context/docs.json --out /tmp/docs-context\nhev context review --bundle /tmp/docs-context","chunkId":"context#3-build-it-and-read-it"},{"kind":"code","literal":"hev context diff --before /tmp/docs-context --after /tmp/docs-context-2 --out /tmp/docs-diff\nhev context review --bundle /tmp/docs-diff","chunkId":"context#3-build-it-and-read-it"},{"kind":"code","literal":"build","chunkId":"context#3-build-it-and-read-it"},{"kind":"code","literal":"context.md","chunkId":"context#3-build-it-and-read-it"},{"kind":"code","literal":"materialized/","chunkId":"context#3-build-it-and-read-it"},{"kind":"code","literal":"bundle.json","chunkId":"context#3-build-it-and-read-it"},{"kind":"code","literal":"index.html","chunkId":"context#3-build-it-and-read-it"},{"kind":"code","literal":"review","chunkId":"context#3-build-it-and-read-it"},{"kind":"code","literal":"diff","chunkId":"context#3-build-it-and-read-it"}]
sources: [{"chunkId":"context#3-build-it-and-read-it","url":"/docs/context#3-build-it-and-read-it","anchor":"3-build-it-and-read-it"}]
---

Building a recipe produces a new directory with the assembled context text the agent reads, the selected files byte-for-byte, a manifest with hashes and the digest, and an offline HTML page; review serves that page on loopback only. Builds are reproducible and never overwrite, and two bundles can be compared side by side.
