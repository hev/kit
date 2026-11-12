---
id: "tradeoffs#workers-run-without-approval-prompts"
title: "Tradeoffs"
heading: "Workers run without approval prompts"
group: "Reference"
order: 299
url: "/docs/tradeoffs#workers-run-without-approval-prompts"
anchor: "workers-run-without-approval-prompts"
terms: ["workers","without","approval","prompts","unattended","codex","claude","sandbox","checks","disabled","anything","host","user","isolation","dedicated","machine","account","minimal","credentials","admission","limits","since","worktrees","branches","security","boundary","dangerously","bypass","approvals","skip","permissions","harness","starts","worker","command","cost","give","only","work","needs"]
hash: "6aad049417cbbcc634a43fbdb8d8efcdab6d83f848778fc26b84768a4e2c440e"
mode: "source-primary"
facts: [{"kind":"code","literal":"--dangerously-bypass-approvals-and-sandbox","chunkId":"tradeoffs#workers-run-without-approval-prompts"},{"kind":"code","literal":"--dangerously-skip-permissions","chunkId":"tradeoffs#workers-run-without-approval-prompts"}]
sources: [{"chunkId":"tradeoffs#workers-run-without-approval-prompts","url":"/docs/tradeoffs#workers-run-without-approval-prompts","anchor":"workers-run-without-approval-prompts"}]
---

Workers run unattended with Codex and Claude approval and sandbox checks disabled, so they can run anything the host user can. Isolation is the host's job: use a dedicated machine or account with minimal credentials and admission limits, since per-run worktrees and branches are not a security boundary.
