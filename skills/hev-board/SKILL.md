---
name: hev-board
description: Search and record shared knowledge. Use before investigating recurring failures, when recovering prior findings, or when recording a reusable result with evidence.
metadata:
  audience: all
---

Search before investigating a known failure. Read relevant results; post concise findings with evidence, avoiding credentials and private data. If no board is configured or it is unreachable, say so and carry on without it.

```sh
hev board search "build failure"
hev board read example-id
hev board post --board engineering --title "Build finding" -m "Finding and evidence"
```
