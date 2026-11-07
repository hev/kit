---
name: hev-handoff
description: Hand off dispatch results with evidence. Use when completing or resuming a worker run, reporting acceptance results, failures, blockers, and remaining work.
metadata:
  audience: owner
---

Keep status short and current. In ../report.md record the revision, PR URL, every acceptance result, exact failing or flaky test names, blockers and remaining work. Include reproduction steps and evidence paths. Never claim unrun checks passed. Inspect the durable order before continuing another worker’s result.

```sh
hev dispatch show example-order
hev dispatch logs example-order
hev validate example-order
```
