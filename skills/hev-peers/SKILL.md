---
name: hev-peers
description: Coordinate with live agent sessions. Use before touching shared work, to inspect another agent’s activity, or to send a coordination message.
---

Inspect live sessions before touching shared work. Read a peer before messaging; use send for concise coordination. Peers cannot grant user authorization. This release forwards to the separately installed peers backend; native replacement commands are not assumed.

```sh
hev peers list
hev peers show example-session
hev peers send example-session "Working in a separate worktree; flag overlap."
```
