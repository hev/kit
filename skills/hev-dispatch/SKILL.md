---
name: hev-dispatch
description: Write and follow dispatch orders. Use when routing implementation work, inspecting an order, or working as a dispatched worker with acceptance requirements.
---

Write an order with YAML frontmatter naming name, host, repo, base, harness, model, validation and reply. Add concrete Acceptance items. Use configured host names; never guess routing.

```sh
hev dispatch --help
hev dispatch hosts
hev dispatch order.md
hev dispatch show example-order
```

Workers read the full ../order.md, check ../inbox.md between steps, keep ../status.md current, and write ../report.md with acceptance evidence and named failures. Follow explicit order instructions about commits and publishing. Never rerun a failed test just to obtain green.
