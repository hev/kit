---
id: "dispatch#send-an-order"
title: "Dispatch"
heading: "Send an order"
group: "Use"
order: 155
url: "/docs/dispatch#send-an-order"
anchor: "send-an-order"
terms: ["send","order","sending","host","field","override","picks","machine","must","dispatch","config","admission","checks","free","slot","refuses","name","already","record","repo","against","policy","fetches","origin","creates","branch","worktree","starts","supervisor","harness","tmux","unix","detached","process","windows","worker","gets","fixed","prompt","read"]
hash: "7a3606ee25bfc63d9ebca821613709d5f9d82f450a6126871f9d9d3b6b678faf"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ORDER.md\nhev dispatch ORDER.md --host buildbox","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"host","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"--host","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"cap","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"origin","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"d/\u003cname\u003e","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"origin/\u003cbase\u003e","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"~/dispatch/\u003cname\u003e/wt","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"d-\u003cname\u003e","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"../order.md","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"../inbox.md","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"../status.md","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"../report.md","chunkId":"dispatch#send-an-order"}]
sources: [{"chunkId":"dispatch#send-an-order","url":"/docs/dispatch#send-an-order","anchor":"send-an-order"}]
---

Sending an order: the order's host field or an override picks the machine, which must be in your dispatch config. On admission the host checks for a free slot, refuses a name that already has a record, checks the repo against admission policy, fetches origin and creates a branch in a new worktree, then starts the supervisor and harness (in tmux on Unix, a detached process on Windows). The worker gets a fixed prompt to read the order, check the inbox, keep status current, commit, push, open a draft PR, write a report and meet every Acceptance item, which the order can override.
