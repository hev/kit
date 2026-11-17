---
id: "tradeoffs#workers-run-without-approval-prompts"
title: "Tradeoffs"
heading: "Workers run without approval prompts"
group: "Reference"
order: 394
url: "/docs/tradeoffs#workers-run-without-approval-prompts"
anchor: "workers-run-without-approval-prompts"
terms: ["workers","without","approval","prompts","unattended","harnesses","sandbox","bypassed","worker","command","host","user","isolation","therefore","dedicated","machine","account","give","only","credentials","work","needs","admission","limit","repositories","gets","worktree","branch","prevents","runs","editing","other","files","security","boundary","dangerously","bypass","approvals","skip","permissions"]
hash: "6aad049417cbbcc634a43fbdb8d8efcdab6d83f848778fc26b84768a4e2c440e"
mode: "source-primary"
facts: [{"kind":"code","literal":"--dangerously-bypass-approvals-and-sandbox","chunkId":"tradeoffs#workers-run-without-approval-prompts"},{"kind":"code","literal":"--dangerously-skip-permissions","chunkId":"tradeoffs#workers-run-without-approval-prompts"}]
sources: [{"chunkId":"tradeoffs#workers-run-without-approval-prompts","url":"/docs/tradeoffs#workers-run-without-approval-prompts","anchor":"workers-run-without-approval-prompts"}]
---

Workers run unattended with the harnesses' approval and sandbox prompts bypassed, so a worker can run any command the host's user can. Isolation is therefore the host's job: use a dedicated machine or account, give it only the credentials the work needs, and use admission to limit repositories. Each run gets its own Git worktree and branch, which prevents runs editing each other's files but is not a security boundary.
