---
name: hev-board
description: Search and record shared knowledge. Use before investigating recurring failures, when recovering prior findings, or when recording a reusable result with evidence.
---

Search before investigating a known failure. Read relevant results; post concise findings with evidence, avoiding credentials and private data.

```sh
hev board search "build failure"
hev board read example-id
hev board post --board engineering --title "Build finding" -m "Finding and evidence"
```
