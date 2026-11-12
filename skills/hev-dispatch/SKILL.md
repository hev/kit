---
name: hev-dispatch
description: Write and follow dispatch orders. Use when routing implementation work, inspecting an order, or working as a dispatched worker with acceptance requirements.
metadata:
  audience: owner
---

Write an order with YAML frontmatter naming name, host, repo, base, harness, model, validation and reply. Add concrete Acceptance items. Use configured host names; never guess routing. Add `context: PATH` to name a context recipe in the repository; dispatch builds its bundle at launch and gives the worker the assembled context on standard input (`hev context --help`, `hev context review --run NAME`).

```sh
hev dispatch --help
hev dispatch hosts
hev dispatch order.md
hev dispatch show example-order
hev dispatch ls
hev dispatch say example-order --resolve db 'SQLite'
```

Workers read the full ../order.md, check ../inbox.md between steps, keep ../status.md current, and write ../report.md with acceptance evidence and named failures. Follow explicit order instructions about commits and publishing. Never rerun a failed test just to obtain green.

Workers end by making the last line of ../status.md exactly one marker: `done`, `blocked: <reason>`, or `needs-decision <KEY>: <question>`. Ask for a decision only when the order leaves a real choice open, and stop after writing it. A run that needs a decision shows as `needs-review`, and `ls` shows its key and question in the DECISION column. Answer it with `say NAME --resolve KEY ANSWER`, which records the answer and reworks the same branch and PR. Use plain `say` for other notes. A blocked run shows as `blocked` with its reason.
