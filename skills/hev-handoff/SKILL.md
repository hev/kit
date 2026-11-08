---
name: hev-handoff
description: Hand off dispatch results with evidence. Use when completing or resuming a worker run, reporting acceptance results, failures, blockers, and remaining work.
metadata:
  audience: all
---

Keep status short and current. In ../report.md record the revision, PR URL, every acceptance result, exact failing or flaky test names, blockers and remaining work. Include reproduction steps and evidence paths. Never claim unrun checks passed. Inspect the durable order before continuing another worker’s result. For a run on a host you joined as a member (`hev-node-*`), pass `--host` to show and logs; validation is the host owner’s to run.

```sh
hev dispatch show example-order
hev dispatch logs example-order
hev validate example-order
```
