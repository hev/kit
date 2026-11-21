---
id: "dispatch#send-an-order"
title: "Dispatch"
heading: "Send an order"
group: "Use"
order: 197
url: "/docs/dispatch#send-an-order"
anchor: "send-an-order"
terms: ["send","order","sending","host","picks","machine","overrides","must","name","dispatch","config","admission","checks","repo","reserves","slot","creates","worktree","branch","before","launching","worker","buildbox","origin","base","orders","inbox","status","report","file","replace","path","want","another","toml","confirm","started","show","refused","line"]
hash: "9877f596a5663ae36a93135f5d7313d6220139e484518ce06eff9eec387e416a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch ORDER.md\nhev dispatch ORDER.md --host buildbox","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"host","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"--host","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"cap","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"origin","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"d/\u003cname\u003e","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"origin/\u003cbase\u003e","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"~/.hev/orders/\u003cname\u003e/wt","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"d-\u003cname\u003e","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"../order.md","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"../inbox.md","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"../status.md","chunkId":"dispatch#send-an-order"},{"kind":"code","literal":"../report.md","chunkId":"dispatch#send-an-order"}]
sources: [{"chunkId":"dispatch#send-an-order","url":"/docs/dispatch#send-an-order","anchor":"send-an-order"}]
---

Sending an order: the order's host picks the machine and --host overrides it; one must name a host in your dispatch config. On admission the host checks the repo, reserves a slot, and creates the worktree and branch before launching the worker.
