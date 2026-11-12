---
id: "dispatch#send-an-order"
title: "Dispatch"
heading: "Send an order"
group: "Use"
order: 93
url: "/docs/dispatch#send-an-order"
anchor: "send-an-order"
terms: ["send","order","sending","uses","host","override","checks","free","slot","refuses","name","admission","fetches","origin","creates","worktree","dedicated","branch","starts","supervisor","tmux","unix","detached","process","windows","worker","gets","default","prompt","read","check","inbox","keep","status","commit","push","open","draft","report","dispatch"]
hash: "7a3606ee25bfc63d9ebca821613709d5f9d82f450a6126871f9d9d3b6b678faf"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ORDER.md\nhev dispatch ORDER.md --host buildbox","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"host","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"--host","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"cap","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"origin","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"d/\u003cname\u003e","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"origin/\u003cbase\u003e","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"~/dispatch/\u003cname\u003e/wt","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"d-\u003cname\u003e","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"../order.md","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"../inbox.md","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"../status.md","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"../report.md","chunkId":"dispatch#send-an-order"}]
sources: [{"chunkId":"dispatch#send-an-order","url":"/docs/dispatch#send-an-order","anchor":"send-an-order"}]
---

Sending an order uses its host or an override. The host checks for a free slot, refuses a used name, checks admission, fetches origin and creates a worktree on a dedicated branch, then starts the supervisor in tmux on Unix or a detached process on Windows. The worker gets a default prompt to read the order, check its inbox, keep status, commit, push, open a draft PR and report, which the order can override.
