---
id: "context#6-dispatch-it"
title: "Composable context"
heading: "6. Dispatch it"
group: "Use"
order: 131
url: "/docs/context#6-dispatch-it"
anchor: "6-dispatch-it"
terms: ["dispatch","step","after","worktree","exists","before","agent","starts","host","builds","bundle","recipe","order","body","assignment","worker","receives","assembled","context","standard","input","alongside","usual","prompt","both","harnesses","platforms","harness","wrapper","must","pass","through","missing","invalid","fails","anything","launches","never","part","docs"]
hash: "ffee65741badb90bf7337fb3f11d0da1cc4900885dd5d9e1ea9b2f7eeb705fa6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch docs-for-flags.md","chunkId":"context#6-dispatch-it"},{"kind":"code","literal":"Dispatch the order with `hev dispatch docs-for-flags.md`. Confirm the run started, and if it failed before launch, show me the error, which names the recipe problem. This needs a hev with `hev context`; if `hev context --help` fails, stop and tell me.\nDocs: https://hevkit.com/docs/context#6-dispatch-it (all pages: https://hevkit.com/llms.txt)","chunkId":"context#6-dispatch-it"},{"kind":"code","literal":"task","chunkId":"context#6-dispatch-it"},{"kind":"code","literal":"context.md","chunkId":"context#6-dispatch-it"},{"kind":"code","literal":"codex","chunkId":"context#6-dispatch-it"},{"kind":"code","literal":"claude","chunkId":"context#6-dispatch-it"},{"kind":"code","literal":"harness_wrapper","chunkId":"context#6-dispatch-it"},{"kind":"code","literal":"exec \"$@\"","chunkId":"context#6-dispatch-it"}]
sources: [{"chunkId":"context#6-dispatch-it","url":"/docs/context#6-dispatch-it","anchor":"6-dispatch-it"}]
---

Step six: on dispatch, after the worktree exists and before the agent starts, the host builds the bundle from the worktree using the recipe, with the order's body as the assignment. The worker receives the assembled context on standard input alongside its usual prompt for both harnesses on all platforms, so any harness wrapper must pass standard input through. A missing or invalid recipe fails the run before anything launches; a worker never starts with part of a bundle.
