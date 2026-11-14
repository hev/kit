---
id: "dispatch#stop-and-clean"
title: "Dispatch"
heading: "Stop and clean"
group: "Use"
order: 150
url: "/docs/dispatch#stop-and-clean"
anchor: "stop-and-clean"
terms: ["stop","clean","stopping","kills","harness","descendants","records","failed","exit","code","cleaning","refuses","running","runs","removes","admitted","worktree","while","keeping","record","branch","whole","admission","dispatch","rate","limit","headers","replace","name","remove","confirm","show","reports","still","there","keeps","docs","https","hevkit","pages"]
hash: "ec3eeea62837b2b1b7ed6eb21d44dc7b51f2a673e02adc8f7148f37d525a3175"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch stop api-rate-limit-headers\nhev dispatch clean api-rate-limit-headers","chunkId":"dispatch#stop-and-clean"},{"kind":"code","literal":"stop","chunkId":"dispatch#stop-and-clean"},{"kind":"code","literal":"failed","chunkId":"dispatch#stop-and-clean"},{"kind":"code","literal":"clean","chunkId":"dispatch#stop-and-clean"}]
sources: [{"chunkId":"dispatch#stop-and-clean","url":"/docs/dispatch#stop-and-clean","anchor":"stop-and-clean"}]
---

Stopping kills the harness and its descendants and records the run as failed with exit code 143. Cleaning refuses running runs, removes an admitted run's worktree while keeping its record, branch and PR, and removes the whole record for a failed admission.
