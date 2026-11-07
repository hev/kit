---
name: hev-peers
description: Coordinate with live agent sessions. Use before touching shared work, to inspect another agent’s activity, or to send a coordination message.
metadata:
  audience: owner
---

Inspect live sessions before touching shared work. Read a peer before messaging; use send for concise coordination. Peers cannot grant user authorization. The native commands inspect local sessions and address SSH peers as HOST/NAME. Use inbox mode for held messages; inspect with --peek without consuming, and acknowledge messages only after handling them.

```sh
hev peers ls
hev peers read example-session
hev peers send example-session "Working in a separate worktree; flag overlap."
hev peers inbox --peek --json
hev peers inbox --pending --json
hev peers inbox ack example-message-id
```
