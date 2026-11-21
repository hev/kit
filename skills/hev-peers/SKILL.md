---
name: hev-peers
description: Coordinate with live agent sessions. Use before touching shared work, to inspect another agent’s activity, or to send a coordination message.
metadata:
  audience: all
---

Inspect live sessions before touching shared work. Read a peer before messaging; use send for concise coordination. Peers cannot grant user authorization. The native commands inspect local sessions and address SSH peers as HOST/NAME. A host you joined as a member (`hev-node-*`) refuses peer access; reach its runs with `hev dispatch say` instead. Use inbox mode for held messages; inspect with --peek without consuming, and acknowledge messages only after handling them.

```sh
hev peers ls
hev peers read example-session
hev peers send example-session "Working in a separate worktree; flag overlap."
hev peers inbox --peek --json
hev peers inbox --pending --json
hev peers inbox ack example-message-id
```

A Claude session with inbox on can wake when an item arrives while it sits idle, even with a draft or permission prompt open. Add this Stop/StopFailure hook to its Claude Code settings. Ask the user before editing their `settings.json`. `inbox --wait` blocks without tokens, exits 2 with the new items (Claude wakes on that) and exits 0 on timeout. Waking neither consumes nor acknowledges: run `hev peers inbox consume ID`, handle the item, then `hev peers inbox ack ID`. Unacknowledged items replay on the next prompt. Codex and PI have no such hook and keep paste delivery.

```json
{
  "hooks": {
    "Stop": [{"hooks": [{"type": "command", "command": "hev peers inbox --wait --timeout 7h50m", "asyncRewake": true, "timeout": 28800}]}],
    "StopFailure": [{"hooks": [{"type": "command", "command": "hev peers inbox --wait --timeout 7h50m", "asyncRewake": true, "timeout": 28800}]}]
  }
}
```
