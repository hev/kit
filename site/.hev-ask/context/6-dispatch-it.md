---
id: "context#6-dispatch-it"
title: "Composable context"
heading: "6. Dispatch it"
group: "Use"
order: 130
url: "/docs/context#6-dispatch-it"
anchor: "6-dispatch-it"
terms: ["dispatch","dispatched","host","builds","bundle","worktree","recipe","before","agent","starts","uses","order","body","assignment","gives","worker","assembled","context","supported","harnesses","platforms","missing","invalid","fails","launch","partial","never","docs","flags","confirm","started","failed","show","error","names","problem","needs","help","stop","tell"]
hash: "dbe07d65852d5b5e1f4db8cf4312241e230fab77d76b96788e14df249397ff9b"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch docs-for-flags.md","chunkId":"context#6-dispatch-it"},{"kind":"code","literal":"Dispatch the order with `hev dispatch docs-for-flags.md`. Confirm the run started, and if it failed before launch, show me the error, which names the recipe problem. This needs a hev with `hev context`; if `hev context --help` fails, stop and tell me.\nDocs: https://hevkit.com/docs/context#6-dispatch-it (all pages: https://hevkit.com/llms.txt)","chunkId":"context#6-dispatch-it"},{"kind":"code","literal":"context.md","chunkId":"context#6-dispatch-it"},{"kind":"code","literal":"codex","chunkId":"context#6-dispatch-it"},{"kind":"code","literal":"claude","chunkId":"context#6-dispatch-it"}]
sources: [{"chunkId":"context#6-dispatch-it","url":"/docs/context#6-dispatch-it","anchor":"6-dispatch-it"}]
---

When dispatched, the host builds the bundle from the worktree using the recipe before the agent starts, uses the order body as the assignment, and gives the worker the assembled context on all supported harnesses and platforms. A missing or invalid recipe fails the run before launch; a partial bundle is never used.
