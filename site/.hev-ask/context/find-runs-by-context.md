---
id: "context#find-runs-by-context"
title: "Composable context"
heading: "Find runs by context"
group: "Use"
order: 136
url: "/docs/context#find-runs-by-context"
anchor: "find-runs-by-context"
terms: ["find","runs","context","digest","handle","finding","given","particular","take","details","list","search","traces","filtered","combined","other","scoping","filters","every","tier","rooted","trace","recorded","side","dashboard","asks","shows","short","attempts","were","bundle","orders","written","intake","templates","name","recipe","tests","fail","dispatch"]
hash: "eb4ab7e40795a05cadbb7f1f70572a402198adadae56c1fb2980b72f68e2a1a0"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev traces ls --context DIGEST\nhev traces search \"why did the tests fail\" --context DIGEST","chunkId":"context#find-runs-by-context"},{"kind":"code","literal":"hev dispatch show","chunkId":"context#find-runs-by-context"},{"kind":"code","literal":"--context","chunkId":"context#find-runs-by-context"},{"kind":"code","literal":"--order","chunkId":"context#find-runs-by-context"},{"kind":"code","literal":"--since","chunkId":"context#find-runs-by-context"},{"kind":"code","literal":"traces ls --context","chunkId":"context#find-runs-by-context"},{"kind":"code","literal":"hev intake","chunkId":"context#find-runs-by-context"},{"kind":"code","literal":"context:","chunkId":"context#find-runs-by-context"}]
sources: [{"chunkId":"context#find-runs-by-context","url":"/docs/context#find-runs-by-context","anchor":"find-runs-by-context"}]
---

The digest is the handle for finding runs given a particular context: take it from the run's details, then list or search traces filtered by it, combined with other scoping filters, on every tier. A run's context and its ask-rooted trace are recorded side by side, and the dashboard's Asks tab shows a short digest on run attempts that were given a bundle. Orders written by intake templates can name a recipe too.
