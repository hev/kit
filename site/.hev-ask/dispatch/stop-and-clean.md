---
id: "dispatch#stop-and-clean"
title: "Dispatch"
heading: "Stop and clean"
group: "Use"
order: 157
url: "/docs/dispatch#stop-and-clean"
anchor: "stop-and-clean"
terms: ["stop","clean","stopping","asks","supervisor","kill","harness","descendants","records","failed","already","dead","process","group","killed","directly","cleaning","refuses","running","order","admitted","removes","only","worktree","normal","checks","while","keeping","record","branch","admission","whole","dispatch","rate","limit","headers","replace","name","remove","confirm"]
hash: "ec3eeea62837b2b1b7ed6eb21d44dc7b51f2a673e02adc8f7148f37d525a3175"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch stop api-rate-limit-headers\nhev dispatch clean api-rate-limit-headers","chunkId":"dispatch#stop-and-clean"},{"kind":"code","literal":"stop","chunkId":"dispatch#stop-and-clean"},{"kind":"code","literal":"failed","chunkId":"dispatch#stop-and-clean"},{"kind":"code","literal":"clean","chunkId":"dispatch#stop-and-clean"}]
sources: [{"chunkId":"dispatch#stop-and-clean","url":"/docs/dispatch#stop-and-clean","anchor":"stop-and-clean"}]
---

Stopping asks the supervisor to kill the harness and its descendants and records the run as failed; if the supervisor is already dead the harness process group is killed directly. Cleaning refuses a running order, and for an admitted run removes only the worktree using Git's normal checks while keeping the record, branch and PR. For a failed admission, cleaning removes the whole record.
