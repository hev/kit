---
id: "context#3-build-it-and-read-it"
title: "Composable context"
heading: "3. Build it and read it"
group: "Use"
order: 169
url: "/docs/context#3-build-it-and-read-it"
anchor: "3-build-it-and-read-it"
terms: ["build","read","step","bundle","recipe","review","exactly","worker","would","receive","context","root","docs","json","diff","before","after","materialized","index","html","output","directory","must","confirm","wrote","tell","digest","give","loopback","prints","open","browser","needs","help","fails","stop","https","hevkit","pages","llms"]
hash: "04d7aca64e6bd7adf3716ad0c453b9a5630b25f9bb15a7e67ef4d62fac06c8b3"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev context build --root . --recipe .hev/context/docs.json --out /tmp/docs-context\nhev context review --bundle /tmp/docs-context","chunkId":"context#3-build-it-and-read-it"},{"kind":"code","literal":"hev context diff --before /tmp/docs-context --after /tmp/docs-context-2 --out /tmp/docs-diff\nhev context review --bundle /tmp/docs-diff","chunkId":"context#3-build-it-and-read-it"},{"kind":"code","literal":"build","chunkId":"context#3-build-it-and-read-it"},{"kind":"code","literal":"context.md","chunkId":"context#3-build-it-and-read-it"},{"kind":"code","literal":"materialized/","chunkId":"context#3-build-it-and-read-it"},{"kind":"code","literal":"bundle.json","chunkId":"context#3-build-it-and-read-it"},{"kind":"code","literal":"index.html","chunkId":"context#3-build-it-and-read-it"},{"kind":"code","literal":"review","chunkId":"context#3-build-it-and-read-it"},{"kind":"code","literal":"diff","chunkId":"context#3-build-it-and-read-it"}]
sources: [{"chunkId":"context#3-build-it-and-read-it","url":"/docs/context#3-build-it-and-read-it","anchor":"3-build-it-and-read-it"}]
---

Step 3: build the bundle from the recipe and review it to see exactly what a worker would receive.
