---
id: "dispatch#send-an-order"
title: "Dispatch"
heading: "Send an order"
group: "Use"
order: 148
url: "/docs/dispatch#send-an-order"
anchor: "send-an-order"
terms: ["send","order","host","override","picks","machine","checks","free","slot","refuses","duplicate","names","admission","fetches","origin","creates","worktree","branch","starts","supervisor","tmux","unix","detached","process","windows","worker","gets","fixed","prompt","read","check","inbox","keep","status","current","commit","push","open","draft","write"]
hash: "7a3606ee25bfc63d9ebca821613709d5f9d82f450a6126871f9d9d3b6b678faf"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ORDER.md\nhev dispatch ORDER.md --host buildbox","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"host","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"--host","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"cap","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"origin","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"d/\u003cname\u003e","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"origin/\u003cbase\u003e","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"~/dispatch/\u003cname\u003e/wt","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"d-\u003cname\u003e","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"../order.md","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"../inbox.md","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"../status.md","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"../report.md","chunkId":"dispatch#send-an-order"}]
sources: [{"chunkId":"dispatch#send-an-order","url":"/docs/dispatch#send-an-order","anchor":"send-an-order"}]
---

The order's host (or an override) picks the machine. The host checks for a free slot, refuses duplicate names, checks admission, fetches origin and creates a worktree branch, then starts a supervisor (tmux on Unix, a detached process on Windows). The worker gets a fixed prompt to read the order, check its inbox, keep status current, commit, push, open a draft PR, write a report and meet every acceptance item.
