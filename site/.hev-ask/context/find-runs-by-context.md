---
id: "context#find-runs-by-context"
title: "Composable context"
heading: "Find runs by context"
group: "Use"
order: 79
url: "/docs/context#find-runs-by-context"
anchor: "find-runs-by-context"
terms: ["find","runs","context","digest","handle","finding","given","same","take","details","filter","trace","listing","search","combined","order","time","other","scopes","works","without","layer","traces","tests","fail","dispatch","show","since","were","only","want","recent","sessions","list","quote","best","match","needs","help","fails"]
hash: "9221193f3b587116ffd2f1a04cb6cd3f969e8e9a2ccb908b900ddb7ff11d2055"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev traces ls --context DIGEST\nhev traces search \"why did the tests fail\" --context DIGEST","chunkId":"context#find-runs-by-context"},{"kind":"code","literal":"hev dispatch show","chunkId":"context#find-runs-by-context"},{"kind":"code","literal":"--context","chunkId":"context#find-runs-by-context"},{"kind":"code","literal":"--order","chunkId":"context#find-runs-by-context"},{"kind":"code","literal":"--since","chunkId":"context#find-runs-by-context"},{"kind":"code","literal":"traces ls --context","chunkId":"context#find-runs-by-context"}]
sources: [{"chunkId":"context#find-runs-by-context","url":"/docs/context#find-runs-by-context","anchor":"find-runs-by-context"}]
---

The digest is the handle for finding runs given the same context: take it from a run's details and filter trace listing or search by it, combined with order, time and other scopes. It works with or without hev layer.
