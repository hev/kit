---
id: "context#what-is-recorded"
title: "Composable context"
heading: "What is recorded"
group: "Use"
order: 144
url: "/docs/context#what-is-recorded"
anchor: "what-is-recorded"
terms: ["recorded","every","launch","builds","fresh","bundle","rework","retries","restarting","amends","digest","while","earlier","attempts","keep","theirs","amend","changes","context","live","restarts","bundles","record","rather","worktree","never","committed","recipe","details","metadata","result","history","worker","session","rows","hold","full","text","source","permissions"]
hash: "f85b680de65d5fcd4f875de77e8609bd5be9d69efc57254af993ce2d40f66128"
mode: "agent-primary"
facts: [{"kind":"code","literal":"say","chunkId":"context#what-is-recorded"},{"kind":"code","literal":"base","chunkId":"context#what-is-recorded"},{"kind":"code","literal":"context","chunkId":"context#what-is-recorded"},{"kind":"code","literal":"~/dispatch/NAME/contexts/N/","chunkId":"context#what-is-recorded"},{"kind":"code","literal":"context.md","chunkId":"context#what-is-recorded"},{"kind":"code","literal":"materialized/","chunkId":"context#what-is-recorded"},{"kind":"code","literal":"bundle.json","chunkId":"context#what-is-recorded"},{"kind":"code","literal":"index.html","chunkId":"context#what-is-recorded"},{"kind":"code","literal":"meta.json","chunkId":"context#what-is-recorded"},{"kind":"code","literal":"context-recipe.json","chunkId":"context#what-is-recorded"},{"kind":"code","literal":"result.json","chunkId":"context#what-is-recorded"},{"kind":"code","literal":"history.json","chunkId":"context#what-is-recorded"},{"kind":"code","literal":"context_digest","chunkId":"context#what-is-recorded"},{"kind":"code","literal":"hev dispatch show NAME --json","chunkId":"context#what-is-recorded"},{"kind":"code","literal":"hev dispatch ls --json","chunkId":"context#what-is-recorded"},{"kind":"code","literal":"context_bundle","chunkId":"context#what-is-recorded"}]
sources: [{"chunkId":"context#what-is-recorded","url":"/docs/context#what-is-recorded","anchor":"what-is-recorded"}]
---

Every launch builds a fresh bundle, so rework, retries and restarting amends get a new bundle and digest while earlier attempts keep theirs; an amend that changes context on a live run restarts it. Bundles live in the run record rather than the worktree, so they are never committed, and the digest and recipe details are recorded in the run's metadata, result, history and worker session rows. Bundles hold the full text of every source with the run record's permissions and are not sent anywhere new.
