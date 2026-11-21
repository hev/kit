---
id: "dispatch#stop-and-clean"
title: "Dispatch"
heading: "Stop and clean"
group: "Use"
order: 199
url: "/docs/dispatch#stop-and-clean"
anchor: "stop-and-clean"
terms: ["stop","clean","asks","supervisor","kill","harness","descendants","recording","failed","exit","removes","worktree","keeps","record","branch","dispatch","rate","limit","headers","replace","name","remove","confirm","show","reports","code","still","there","docs","https","hevkit","pages","llms","records","already","died","kills","recorded","process","group"]
hash: "ec3eeea62837b2b1b7ed6eb21d44dc7b51f2a673e02adc8f7148f37d525a3175"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch stop api-rate-limit-headers\nhev dispatch clean api-rate-limit-headers","chunkId":"dispatch#stop-and-clean"},{"kind":"code","literal":"stop","chunkId":"dispatch#stop-and-clean"},{"kind":"code","literal":"failed","chunkId":"dispatch#stop-and-clean"},{"kind":"code","literal":"clean","chunkId":"dispatch#stop-and-clean"}]
sources: [{"chunkId":"dispatch#stop-and-clean","url":"/docs/dispatch#stop-and-clean","anchor":"stop-and-clean"}]
---

stop asks the supervisor to kill the harness and descendants, recording failed with exit 143. clean removes the worktree but keeps the record, branch and PR.
