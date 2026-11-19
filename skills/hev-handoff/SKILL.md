---
name: hev-handoff
description: Hand off dispatch results with evidence. Use when completing or resuming a worker run, reporting acceptance results, failures, blockers, and remaining work.
metadata:
  audience: all
---

Keep status short and current. In ../report.md record the revision, PR URL, every acceptance result, exact failing or flaky test names, blockers and remaining work. Include reproduction steps and evidence paths. Write ../review-notes.json beside the report (see hev-dispatch); `hev dispatch show` reports whether it is valid. Never claim unrun checks passed. Inspect the durable order before continuing another worker’s result. A rework or inbox message that starts with "Review findings for NAME" carries recorded review findings (path:line, fail or note, confidence): address every fail finding, update ../review-notes.json for the new head, and say in ../report.md what you did with each; `hev review NAME --json` shows them with earlier reviews. For a run on a host you joined as a member (`hev-node-*`), pass `--host` to show and logs; validation is the host owner’s to run.

```sh
hev dispatch show example-order
hev dispatch logs example-order
hev review example-order --json
hev validate example-order
```
